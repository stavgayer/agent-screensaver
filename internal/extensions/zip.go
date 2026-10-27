package extensions

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	// Zip bomb guards: a small archive must not fill the disk of a Mac that
	// may be locked.
	maxZipEntries = 1000
	maxZipBytes   = 50 << 20

	stagingPrefix = ".install-"
	// staleStaging is the age after which a staging folder is a left over
	// of a crash and not of an install that still runs.
	staleStaging = time.Hour
)

// InstallChecked installs an extension from a folder or a .zip file into
// extDir and returns its manifest. The files are staged in a temp folder
// inside extDir first (same filesystem, so the final rename is cheap),
// validated, and only then moved to extDir/<id>. check runs on the staged
// folder before it replaces anything: callers pass the rules that live in
// other packages, such as an agent's install recipe. check may be nil only
// for extensions that are not agents.
func InstallChecked(src, extDir string, check func(m Manifest, dir string) error) (Manifest, error) {
	return stage(src, extDir, check, true)
}

// Inspect stages src as InstallChecked does and runs inspect on the staged
// folder, then removes it. The installed extensions are never changed, so
// a caller can show what an install would do before it asks.
func Inspect(src, extDir string, inspect func(m Manifest, dir string) error) (Manifest, error) {
	return stage(src, extDir, inspect, false)
}

func stage(src, extDir string, check func(m Manifest, dir string) error, commit bool) (Manifest, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	removeStaleStaging(extDir, time.Now())
	staging, err := os.MkdirTemp(extDir, stagingPrefix)
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	defer os.RemoveAll(staging)

	content := filepath.Join(staging, "new")
	if info.IsDir() {
		err = copyFolder(src, content)
	} else {
		err = unzip(src, content)
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	manifest, err := readManifest(content)
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	if strings.HasPrefix(manifest.ID, builtinPrefix) {
		return Manifest{}, fmt.Errorf("install %q: %w", manifest.ID, ErrBuiltin)
	}
	if err := checkUniqueSource(manifest, extDir); err != nil {
		return Manifest{}, fmt.Errorf("install %q: %w", manifest.ID, err)
	}
	if check != nil {
		if err := check(manifest, content); err != nil {
			return Manifest{}, fmt.Errorf("install %q: %w", manifest.ID, err)
		}
	}
	if !commit {
		return manifest, nil
	}
	if err := replaceFolder(content, filepath.Join(extDir, manifest.ID), staging); err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	return manifest, nil
}

// removeStaleStaging deletes the staging folders older than staleStaging.
// A crash in the middle of an install leaves one; Scan already skips them.
// It is best effort: a folder that cannot be removed is tried next time.
func removeStaleStaging(extDir string, now time.Time) {
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), stagingPrefix) {
			continue
		}
		if info, err := entry.Info(); err == nil && now.Sub(info.ModTime()) > staleStaging {
			_ = os.RemoveAll(filepath.Join(extDir, entry.Name()))
		}
	}
}

// replaceFolder moves content to target. An existing target is parked in
// spare first and put back if the move fails, so a failed upgrade never loses
// the old version. (A directory cannot be renamed over a non-empty one.)
func replaceFolder(content, target, spare string) error {
	parked := filepath.Join(spare, "old")
	hadOld := true
	if err := os.Rename(target, parked); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		hadOld = false
	}
	if err := os.Rename(content, target); err != nil {
		if hadOld {
			_ = os.Rename(parked, target)
		}
		return err
	}
	return nil
}

// copyFolder copies regular files and folders. Symlinks are refused because
// they could point at files outside the extension.
func copyFolder(src, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, relative)
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0o755)
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return copyFile(path, target, info.Mode())
		default:
			return fmt.Errorf("%q is a symlink or special file, which is not allowed", relative)
		}
	})
}

func copyFile(src, dest string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return writeNewFile(dest, in, maxZipBytes, mode)
}

// unzip extracts archive into dest. Every entry is checked before anything
// is written, so a rejected archive leaves no files behind.
func unzip(archive, dest string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	if err := checkZipEntries(reader.File); err != nil {
		return err
	}
	top, err := topFolder(reader.File)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	budget := int64(maxZipBytes)
	for _, file := range reader.File {
		name := zipPath(file.Name)
		if isMacJunk(name) {
			continue
		}
		written, err := extractEntry(file, strings.TrimPrefix(name, top), dest, budget)
		if err != nil {
			return err
		}
		budget -= written
	}
	return nil
}

func checkZipEntries(files []*zip.File) error {
	if len(files) > maxZipEntries {
		return fmt.Errorf("archive has %d entries, the limit is %d", len(files), maxZipEntries)
	}
	var total uint64
	for _, file := range files {
		if err := checkZipName(file.Name); err != nil {
			return err
		}
		mode := file.Mode()
		if mode&fs.ModeSymlink != 0 {
			return fmt.Errorf("entry %q is a symlink, which is not allowed", file.Name)
		}
		if !mode.IsRegular() && !mode.IsDir() {
			return fmt.Errorf("entry %q is a special file, which is not allowed", file.Name)
		}
		total += file.UncompressedSize64
	}
	if total > maxZipBytes {
		return fmt.Errorf("archive is too large uncompressed (%d bytes, limit %d)", total, maxZipBytes)
	}
	return nil
}

// checkZipName rejects names that could leave the extension folder. Windows
// style separators are treated like "/" because some tools write them.
func checkZipName(name string) error {
	unified := strings.ReplaceAll(name, `\`, "/")
	unsafe := strings.HasPrefix(unified, "/") || filepath.VolumeName(unified) != ""
	for _, element := range strings.Split(unified, "/") {
		if element == ".." {
			unsafe = true
		}
	}
	if unsafe {
		return fmt.Errorf("entry %q has an unsafe path", name)
	}
	return nil
}

// ZipTopFolder returns the folder prefix ("name/") that an install strips
// from the entries of archive, or "" when extension.json is at its top.
// Callers use it to name an entry inside the zip, for example in a review.
func ZipTopFolder(archive string) (string, error) {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	if err := checkZipEntries(reader.File); err != nil {
		return "", err
	}
	return topFolder(reader.File)
}

// zipPath is an entry name with Windows style separators turned into "/".
func zipPath(name string) string {
	return strings.ReplaceAll(name, `\`, "/")
}

// isMacJunk reports whether a (zipPath) name is one of the entries macOS
// adds to the zips it makes: the __MACOSX folder and .DS_Store files. They
// are never extracted.
func isMacJunk(name string) bool {
	return name == "__MACOSX" || strings.HasPrefix(name, "__MACOSX/") ||
		path.Base(name) == ".DS_Store"
}

// topFolder returns the prefix ("name/") to strip from every entry when the
// manifest sits inside the archive's only top folder, as in a zip made with
// Finder's Compress on a folder. It is "" when extension.json is at the top
// or there is no single top folder. macOS junk entries do not count. Every
// name was already checked by checkZipEntries, so the stripped names are
// as safe as the full ones.
func topFolder(files []*zip.File) (string, error) {
	folders := map[string]bool{}
	hasTopFile := false
	names := map[string]bool{}
	for _, file := range files {
		name := zipPath(file.Name)
		if isMacJunk(name) {
			continue
		}
		names[name] = true
		first, _, nested := strings.Cut(name, "/")
		switch {
		case name == "extension.json":
			return "", nil
		case nested:
			folders[first] = true
		default:
			hasTopFile = true
		}
	}
	if hasTopFile || len(folders) == 0 {
		return "", nil
	}
	if len(folders) > 1 {
		listed := slices.Sorted(maps.Keys(folders))
		return "", fmt.Errorf("archive has no extension.json at the top and %d top folders (%s): zip one folder, or put extension.json at the top of the zip",
			len(folders), strings.Join(listed, ", "))
	}
	for folder := range folders {
		if names[folder+"/extension.json"] {
			return folder + "/", nil
		}
	}
	return "", nil
}

func extractEntry(file *zip.File, name, dest string, budget int64) (int64, error) {
	if name == "" {
		return 0, nil // the top folder itself
	}
	relative := filepath.FromSlash(name)
	if !filepath.IsLocal(relative) {
		return 0, fmt.Errorf("entry %q has an unsafe path", file.Name)
	}
	target := filepath.Join(dest, relative)
	if file.FileInfo().IsDir() {
		return 0, os.MkdirAll(target, 0o755)
	}
	in, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer in.Close()
	return budgetedWrite(target, in, budget, file.Mode())
}

func budgetedWrite(target string, in io.Reader, budget int64, mode fs.FileMode) (int64, error) {
	counter := &countingReader{reader: in}
	if err := writeNewFile(target, counter, budget, mode); err != nil {
		return 0, err
	}
	return counter.count, nil
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.count += int64(n)
	return n, err
}

// writeNewFile creates target (failing if it exists, so duplicate names in an
// archive are caught) and copies at most limit bytes. Archive headers can lie
// about sizes, so the limit is enforced on the bytes actually read. Of the
// source mode only the execute bit is kept, so an agent's converter program
// still runs; no other bit (setuid, group write) is ever copied.
func writeNewFile(target string, in io.Reader, limit int64, source fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if source&0o111 != 0 {
		mode = 0o755
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	copied, copyErr := io.Copy(out, io.LimitReader(in, limit+1))
	closeErr := out.Close()
	switch {
	case copyErr != nil:
		return copyErr
	case copied > limit:
		return errors.New("archive is too large uncompressed")
	}
	return closeErr
}

// KeepExecutable sets every regular file under dir to 0644, except the
// files in programs (full paths), which become 0755. Install copies keep a
// source's execute bit; this narrows it to the programs the manifest names.
func KeepExecutable(dir string, programs []string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || !entry.Type().IsRegular() {
			return walkErr
		}
		mode := fs.FileMode(0o644)
		if slices.Contains(programs, path) {
			mode = 0o755
		}
		return os.Chmod(path, mode)
	})
}
