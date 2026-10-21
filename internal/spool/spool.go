// Package spool hands events from hooks to the daemon through a folder, one
// file per event. The file contract is public: docs/event-format.md.
package spool

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-screensaver/internal/event"
)

const (
	dirMode  = 0o700
	fileMode = 0o600
	finalExt = ".json"
	tempExt  = ".json.tmp"
)

// Write stores one event in dir. It writes a temp file first and renames it,
// so a reader never sees a half-written event. The temp name is unique and
// created exclusively, so Write never overwrites a file.
func Write(dir string, e event.Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create spool dir: %w", err)
	}
	base := fmt.Sprintf("%d-%d-%d", e.AtMs, os.Getpid(), time.Now().UnixNano())
	tempPath := filepath.Join(dir, base+tempExt)
	if err := writeExclusive(tempPath, data); err != nil {
		return err
	}
	if err := os.Rename(tempPath, filepath.Join(dir, base+finalExt)); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("publish event: %w", err)
	}
	return nil
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return fmt.Errorf("create event file: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("write event file: %w", err)
	}
	return nil
}

// Drain returns every valid event in dir, ordered by file name (which starts
// with the event time), and deletes each file it read. Unreadable, oversized
// and invalid files are deleted too, so one bad file never blocks the rest.
// A missing dir is an empty spool.
func Drain(dir string) []event.Event {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		return nil
	}
	var events []event.Event
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), finalExt) {
			continue // also skips ".json.tmp": the writer is not done yet
		}
		path := filepath.Join(dir, entry.Name())
		data, err := readCapped(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue // another reader took it first
		}
		_ = os.Remove(path)
		if err != nil {
			continue
		}
		if parsed, err := event.Parse(data); err == nil {
			events = append(events, parsed)
		}
	}
	return events
}

// readCapped reads at most one byte more than the event limit, so an
// oversized file is detected without loading all of it.
func readCapped(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, event.MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > event.MaxInputBytes {
		return nil, fmt.Errorf("event file over %d bytes", event.MaxInputBytes)
	}
	return data, nil
}
