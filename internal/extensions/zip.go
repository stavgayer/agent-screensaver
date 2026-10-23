package extensions

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Zip bomb guards: a small archive must not fill the disk of a Mac that
	// may be locked.
	maxZipEntries = 1000
	maxZipBytes   = 50 << 20

	stagingPrefix = ".install-"
)

// InstallFrom installs an extension from a folder or a .zip file into extDir
// and returns its manifest. The files are staged in a temp folder inside
// extDir first (same filesystem, so the final rename is cheap), validated,
// and only then moved to extDir/<id>.
func InstallFrom(src, extDir string) (Manifest, error) {
	return InstallChecked(src, extDir, nil)
}

// InstallChecked is InstallFrom with one more check, run on the staged
// folder before it replaces anything. Callers use it for the rules that
// live in other packages, such as an agent's install recipe. check may be nil.
func InstallChecked(src, extDir string, check func(m Manifest, dir string) error) (Manifest, error) {
	info, err := os.Stat(src)
	if err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
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
	if err := replaceFolder(content, filepath.Join(extDir, manifest.ID), staging); err != nil {
		return Manifest{}, fmt.Errorf("install: %w", err)
	}
	return manifest, nil
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
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	budget := int64(maxZipBytes)
	for _, file := range reader.File {
		written, err := extractEntry(file, dest, budget)
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

func extractEntry(file *zip.File, dest string, budget int64) (int64, error) {
	relative := filepath.FromSlash(strings.ReplaceAll(file.Name, `\`, "/"))
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
