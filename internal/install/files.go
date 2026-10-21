package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// checkArtifacts makes sure the three build outputs exist before we change
// anything.
func checkArtifacts(artifacts string) error {
	for _, name := range []string{"runtime", "extensions", saverName} {
		if info, err := os.Stat(filepath.Join(artifacts, name)); err != nil || !info.IsDir() {
			return fmt.Errorf("artifacts folder %s has no %s folder (use --artifacts)", artifacts, name)
		}
	}
	return nil
}

func copyArtifacts(bin, artifacts string, where locations) error {
	if err := copyBinary(bin, where.bin); err != nil {
		return err
	}
	if err := replaceTree(filepath.Join(artifacts, "runtime"), where.paths.RuntimeDir); err != nil {
		return err
	}
	if err := copyBuiltinExtensions(filepath.Join(artifacts, "extensions"), where.paths.ExtensionsDir); err != nil {
		return err
	}
	return replaceTree(filepath.Join(artifacts, saverName), where.saver)
}

// copyBinary skips the copy when the running binary already is the installed one.
func copyBinary(source, destination string) error {
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	if destinationInfo, err := os.Stat(destination); err == nil && os.SameFile(sourceInfo, destinationInfo) {
		return nil
	}
	return copyFile(source, destination, 0o755)
}

// copyBuiltinExtensions replaces our builtin.* folders and leaves every other
// installed extension alone. Old builtin.* folders go first so one that was
// removed from the build does not linger.
func copyBuiltinExtensions(source, destination string) error {
	if err := removeBuiltinExtensions(destination); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read extensions: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), builtinPrefix) {
			continue
		}
		if err := replaceTree(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func removeBuiltinExtensions(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read extensions: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), builtinPrefix) {
			if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
				return fmt.Errorf("remove old extension %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

// removeInstalledFiles deletes what Apply copied and the aerial clones the
// daemon made. Settings, spool, the user's media and other extensions stay. The paths come from the home directory, never from the
// record file, so a damaged or edited record cannot point deletion elsewhere.
func removeInstalledFiles(where locations) error {
	for _, path := range []string{filepath.Dir(where.bin), where.paths.RuntimeDir, where.saver, where.paths.AerialClonesDir} {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return removeBuiltinExtensions(where.paths.ExtensionsDir)
}

// replaceTree makes destination an exact copy of source.
func replaceTree(source, destination string) error {
	if err := os.RemoveAll(destination); err != nil {
		return fmt.Errorf("replace %s: %w", destination, err)
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, 0o755)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

func copyFile(source, destination string, perm fs.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer in.Close()
	return writeStream(destination, in, perm)
}

func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	return writeStream(path, strings.NewReader(string(data)), perm)
}

// writeStream writes through a temp file and a rename, so a crash never leaves
// a half-written file. A symlinked file (dotfile managers do this) is
// replaced at its target, and an existing file keeps its permissions.
func writeStream(path string, content io.Reader, perm fs.FileMode) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	if info, err := os.Stat(target); err == nil {
		perm = info.Mode().Perm()
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(target)+".tmp*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", target, err)
	}
	defer os.Remove(temp.Name()) // no-op after a successful rename
	if _, err := io.Copy(temp, content); err != nil {
		temp.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := temp.Chmod(perm); err != nil {
		temp.Close()
		return fmt.Errorf("set permissions of %s: %w", target, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", target, err)
	}
	if err := os.Rename(temp.Name(), target); err != nil {
		return fmt.Errorf("replace %s: %w", target, err)
	}
	return nil
}
