// Package statefile writes state.json, the one file the screen saver reads.
package statefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"agent-screensaver/internal/state"
	"agent-screensaver/internal/view"
)

const (
	formatVersion = 1
	dirMode       = 0o755
	fileMode      = 0o644 // the sandboxed saver must be able to read it
)

// StateFile is the JSON document the screen saver loads.
type StateFile struct {
	Version       int           `json:"version"`
	GeneratedAtMs int64         `json:"generated_at_ms"`
	ServiceOK     bool          `json:"service_ok"`
	Cards         []state.Card  `json:"cards"`
	Limits        []state.Limit `json:"limits"`
	View          view.View     `json:"view"`
}

// Build combines state and view into a file. With privacy on it clears each
// card's title and request. It copies the cards, so the caller's state is
// untouched, and it never leaves a nil list (which would encode as null).
func Build(st state.State, v view.View, nowMs int64) StateFile {
	cards := make([]state.Card, len(st.Cards))
	copy(cards, st.Cards)
	if v.Privacy {
		for i := range cards {
			cards[i].Title = ""
			cards[i].Request = ""
		}
	}
	limits := make([]state.Limit, len(st.Limits))
	copy(limits, st.Limits)
	return StateFile{
		Version:       formatVersion,
		GeneratedAtMs: nowMs,
		ServiceOK:     true,
		Cards:         cards,
		Limits:        limits,
		View:          v,
	}
}

// WriteAtomic writes f to path through a temp file in the same folder and a
// rename, so the saver never reads a half-written file.
func WriteAtomic(path string, f StateFile) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp state file: %w", err)
	}
	tempPath := temp.Name()
	_, writeErr := temp.Write(data)
	chmodErr := temp.Chmod(fileMode)
	closeErr := temp.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace state file: %w", err)
	}
	return nil
}
