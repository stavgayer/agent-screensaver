package extensions

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// A pack is a folder or a .zip file with no extension.json at its top (or
// in its only top folder) and two or more subfolders that each hold one.
// It lets an author share a whole theme (theme, layout, background and
// widgets) as one file.

// IsPack reports whether src is a pack. A single extension is not.
func IsPack(src string) (bool, error) {
	names, err := packNames(src)
	if err != nil {
		return false, err
	}
	_, parts := findPack(names)
	return len(parts) > 0, nil
}

// InstallPack installs every part of the pack src into extDir and returns
// their manifests in the order of the part folders. All parts are staged
// and checked first: when one fails, nothing is installed and the error
// names the part. check runs on each staged part, as in InstallChecked.
// Agents are refused, because an agent upgrade needs its own review. The
// zip limits apply to the whole pack.
func InstallPack(src, extDir string, check func(m Manifest, dir string) error) ([]Manifest, error) {
	names, err := packNames(src)
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	base, parts := findPack(names)
	if len(parts) == 0 {
		return nil, fmt.Errorf("install: %s is not a pack: a pack holds two or more folders that each have an %s", src, ManifestFile)
	}
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	removeStaleStaging(extDir, time.Now())
	staging, err := os.MkdirTemp(extDir, stagingPrefix)
	if err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	defer os.RemoveAll(staging)

	content := filepath.Join(staging, "new")
	if err := copyPack(src, base, content); err != nil {
		return nil, fmt.Errorf("install: %w", err)
	}
	manifests, err := checkParts(content, parts, check)
	if err != nil {
		return nil, err
	}
	if err := commitParts(content, parts, manifests, extDir, staging); err != nil {
		return nil, err
	}
	return manifests, nil
}

// packNames lists the names a pack check needs, as slash paths with a "/"
// after a folder: every zip entry, or the first three levels of a folder.
// macOS junk is left out.
func packNames(src string) ([]string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		reader, err := zip.OpenReader(src)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		if err := checkZipEntries(reader.File); err != nil {
			return nil, err
		}
		var names []string
		for _, file := range reader.File {
			if name := zipPath(file.Name); !isMacJunk(strings.TrimSuffix(name, "/")) {
				names = append(names, name)
			}
		}
		return names, nil
	}
	var names []string
	err = filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || path == src {
			return walkErr
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if isMacJunk(name) {
			return nil
		}
		depth := strings.Count(name, "/") + 1
		switch {
		case entry.IsDir() && depth >= 3:
			return filepath.SkipDir // base/part/extension.json is the deepest name that counts
		case entry.IsDir():
			names = append(names, name+"/")
		default:
			names = append(names, name)
		}
		return nil
	})
	return names, err
}

// findPack returns the folder ("" or "name/") that holds the parts and the
// part folder names, sorted. parts is empty when names are not a pack.
func findPack(names []string) (string, []string) {
	files := map[string]bool{}
	tops := map[string]bool{}
	hasTopFile := false
	for _, name := range names {
		first, rest, nested := strings.Cut(name, "/")
		switch {
		case !nested:
			hasTopFile = true
			files[name] = true
		case rest != "" && !strings.HasSuffix(rest, "/"):
			files[name] = true
			tops[first] = true
		default:
			tops[first] = true
		}
	}
	if files[ManifestFile] {
		return "", nil
	}
	base := ""
	if !hasTopFile && len(tops) == 1 {
		for top := range tops {
			base = top + "/"
		}
		if files[base+ManifestFile] {
			return "", nil
		}
	}
	var parts []string
	for name := range files {
		part, file, nested := strings.Cut(strings.TrimPrefix(name, base), "/")
		if strings.HasPrefix(name, base) && nested && file == ManifestFile {
			parts = append(parts, part)
		}
	}
	if len(parts) < 2 {
		return "", nil
	}
	slices.Sort(parts)
	return base, parts
}

// copyPack copies the pack folder base of src into dest.
func copyPack(src, base, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyFolder(filepath.Join(src, filepath.FromSlash(base)), dest)
	}
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := checkZipEntries(reader.File); err != nil {
		return err
	}
	return extractAll(reader.File, base, dest)
}

// checkParts reads and checks every staged part before any is installed.
func checkParts(content string, parts []string, check func(m Manifest, dir string) error) ([]Manifest, error) {
	manifests := make([]Manifest, 0, len(parts))
	seen := map[string]string{} // id to part folder
	for _, part := range parts {
		dir := filepath.Join(content, filepath.FromSlash(part))
		manifest, err := readManifest(dir)
		switch {
		case err != nil:
			return nil, fmt.Errorf("install pack part %q: %w", part, err)
		case manifest.Type == TypeAgent:
			return nil, fmt.Errorf("install pack part %q: %s is an agent; install an agent alone so its changes can be reviewed", part, manifest.ID)
		case strings.HasPrefix(manifest.ID, builtinPrefix):
			return nil, fmt.Errorf("install pack part %q: %q: %w", part, manifest.ID, ErrBuiltin)
		case seen[manifest.ID] != "":
			return nil, fmt.Errorf("install pack part %q: %s is in the pack twice (also in %q)", part, manifest.ID, seen[manifest.ID])
		}
		seen[manifest.ID] = part
		if check != nil {
			if err := check(manifest, dir); err != nil {
				return nil, fmt.Errorf("install pack part %q (%s): %w", part, manifest.ID, err)
			}
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// commitParts moves every checked part to extDir/<id>. When one move
// fails, the parts moved before it are put back as they were.
func commitParts(content string, parts []string, manifests []Manifest, extDir, staging string) error {
	type moved struct{ target, spare string }
	var done []moved
	for index, manifest := range manifests {
		spare := filepath.Join(staging, "spare-"+strconv.Itoa(index))
		target := filepath.Join(extDir, manifest.ID)
		err := os.Mkdir(spare, 0o755)
		if err == nil {
			err = replaceFolder(filepath.Join(content, filepath.FromSlash(parts[index])), target, spare)
		}
		if err != nil {
			for _, back := range slices.Backward(done) {
				_ = os.RemoveAll(back.target)
				_ = os.Rename(filepath.Join(back.spare, "old"), back.target)
			}
			return fmt.Errorf("install %q: %w", manifest.ID, err)
		}
		done = append(done, moved{target, spare})
	}
	return nil
}

// KeptPart is a part of a theme that stays because another extension
// still uses it.
type KeptPart struct {
	ID     string
	UsedBy string
}

// ThemeRemoval is what removing a theme removes and keeps.
type ThemeRemoval struct {
	Remove []string // the theme first, then its parts
	Kept   []KeptPart
}

// PlanThemeRemoval says what removing id removes. For a theme that is the
// theme, its layout, its background and the widgets of that layout, except
// built-ins, parts that are not installed and parts another installed
// theme or layout still uses. Any other id removes only itself. Nothing is
// changed.
func PlanThemeRemoval(id string, installed []Installed) (ThemeRemoval, error) {
	byID := make(map[string]Manifest, len(installed))
	for _, ext := range installed {
		byID[ext.Manifest.ID] = ext.Manifest
	}
	target, ok := byID[id]
	if !ok {
		return ThemeRemoval{}, fmt.Errorf("extension %q is not installed", id)
	}
	if target.Type != TypeTheme {
		return ThemeRemoval{Remove: []string{id}}, nil
	}
	removing := map[string]bool{id: true}
	var candidates []string
	add := func(ids ...string) {
		for _, part := range ids {
			if _, ok := byID[part]; ok && !removing[part] && !strings.HasPrefix(part, builtinPrefix) {
				removing[part] = true
				candidates = append(candidates, part)
			}
		}
	}
	add(References(target)...)
	if target.Theme != nil {
		add(References(byID[target.Theme.Layout])...)
	}
	// A kept part keeps what it uses, so repeat until nothing changes.
	var kept []KeptPart
	for changed := true; changed; {
		changed = false
		for _, part := range candidates {
			if !removing[part] {
				continue
			}
			if user := usedBy(part, installed, removing); user != "" {
				delete(removing, part)
				kept = append(kept, KeptPart{ID: part, UsedBy: user})
				changed = true
			}
		}
	}
	slices.SortFunc(kept, func(a, b KeptPart) int { return strings.Compare(a.ID, b.ID) })
	plan := ThemeRemoval{Remove: []string{id}, Kept: kept}
	for _, part := range candidates {
		if removing[part] {
			plan.Remove = append(plan.Remove, part)
		}
	}
	return plan, nil
}

// usedBy returns the first id, sorted, of an installed theme or layout
// that is not being removed and uses part, or "".
func usedBy(part string, installed []Installed, removing map[string]bool) string {
	var users []string
	for _, ext := range installed {
		if !removing[ext.Manifest.ID] && slices.Contains(References(ext.Manifest), part) {
			users = append(users, ext.Manifest.ID)
		}
	}
	if len(users) == 0 {
		return ""
	}
	return slices.Min(users)
}

// References returns the ids an extension uses: a theme's layout and
// background, or a layout's widgets (sorted, once each). Other types use
// none.
func References(m Manifest) []string {
	switch {
	case m.Type == TypeTheme && m.Theme != nil:
		return []string{m.Theme.Layout, m.Theme.Background.ID}
	case m.Type == TypeLayout:
		var spec layoutSpec
		if err := json.Unmarshal(m.Layout, &spec); err != nil {
			return nil // Scan only returns valid layouts
		}
		var ids []string
		for _, regions := range spec.Modes {
			for _, refs := range regions {
				for _, ref := range refs {
					ids = append(ids, ref.ID)
				}
			}
		}
		slices.Sort(ids)
		return slices.Compact(ids)
	}
	return nil
}
