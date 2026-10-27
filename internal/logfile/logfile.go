// Package logfile writes the service log with a size limit. When a write
// would make the file pass the limit, the file becomes <path>.1 (replacing
// the older one) and the write goes whole to a new, empty file.
package logfile

import (
	"os"
	"sync"
)

// MaxSize is the size the service log may reach before it is rotated.
const MaxSize = 1 << 20

// Rotating is an io.Writer for a log file that keeps one old copy. It is
// safe for use from several goroutines; each Write is one log line.
type Rotating struct {
	mu       sync.Mutex
	path     string
	max      int64
	file     *os.File
	size     int64
	onReopen func(*os.File) error
}

// Open opens path for appending. onReopen, when not nil, runs with each new
// file after a rotation, for example to point stderr at it; its error is
// ignored, because the log itself still works.
func Open(path string, max int64, onReopen func(*os.File) error) (*Rotating, error) {
	file, size, err := openAppend(path)
	if err != nil {
		return nil, err
	}
	return &Rotating{path: path, max: max, file: file, size: size, onReopen: onReopen}, nil
}

func openAppend(path string) (*os.File, int64, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}
	return file, info.Size(), nil
}

// Write appends p. A p that would make a non-empty file pass the limit is
// written to a new file after the rotation, so a line is never split. If
// the rotation fails, p goes to the current file.
func (r *Rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		r.rotate()
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *Rotating) rotate() {
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return
	}
	file, size, err := openAppend(r.path)
	if err != nil {
		// The old file keeps the lines until the next try.
		return
	}
	_ = r.file.Close()
	r.file, r.size = file, size
	if r.onReopen != nil {
		_ = r.onReopen(file)
	}
}

// Close closes the current file.
func (r *Rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.file.Close()
}
