// Package atomicfile writes a file so that a reader never sees half of it.
package atomicfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const dirMode = 0o755

// Write writes data to path through a temp file in the same folder and a
// rename. It creates the folder when it is missing. perm is set on the
// temp file before the rename, because CreateTemp makes it private.
func Write(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create folder of %s: %w", filepath.Base(path), err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", filepath.Base(path), err)
	}
	tempPath := temp.Name()
	_, writeErr := temp.Write(data)
	chmodErr := temp.Chmod(perm)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp file for %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}
