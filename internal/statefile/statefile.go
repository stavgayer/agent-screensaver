// Package statefile writes state.json, the one file the screen saver reads.
package statefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"

	"agent-screensaver/internal/atomicfile"
	"agent-screensaver/internal/nowplaying"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/view"
)

const (
	formatVersion = 1
	fileMode      = 0o644 // the sandboxed saver must be able to read it
)

// StateFile is the JSON document the screen saver loads.
type StateFile struct {
	Version       int                     `json:"version"`
	GeneratedAtMs int64                   `json:"generated_at_ms"`
	ServiceOK     bool                    `json:"service_ok"`
	Cards         []state.Card            `json:"cards"`
	Limits        []state.Limit           `json:"limits"`
	Sources       map[string]state.Source `json:"sources"`
	// SessionNames is the last name of each session, so a restarted
	// service still knows a name its files no longer show. Internal: the
	// saver ignores it. Privacy mode leaves it out.
	SessionNames []state.SessionName `json:"session_names,omitempty"`
	View         view.View           `json:"view"`
	// NowPlaying is the song a music app plays, when the user has now
	// playing on. Privacy mode leaves it out.
	NowPlaying *nowplaying.Track `json:"now_playing,omitempty"`
	// Demo marks a file whose cards and limits are the made-up demo scene,
	// so a restarted service never takes them back as real. Internal: the
	// saver ignores it.
	Demo bool `json:"demo,omitempty"`
}

// Build combines state, view and the playing song (nil for none) into a
// file. With privacy on it clears each card's title, request, conversation
// name and branch: each says what the work is; it also leaves out the
// session names and the song. It copies the cards, so the caller's state is
// untouched, and it never leaves a nil list (which would encode as null).
func Build(st state.State, v view.View, track *nowplaying.Track, nowMs int64) StateFile {
	cards := make([]state.Card, len(st.Cards))
	copy(cards, st.Cards)
	if v.Privacy {
		for i := range cards {
			cards[i].Title = ""
			cards[i].Request = ""
			cards[i].Name = ""
			cards[i].Branch = ""
		}
	}
	limits := make([]state.Limit, len(st.Limits))
	copy(limits, st.Limits)
	var names []state.SessionName
	if !v.Privacy {
		names = st.SessionNames(nowMs)
	}
	var song *nowplaying.Track
	if track != nil && !v.Privacy {
		copied := *track
		song = &copied
	}
	sources := maps.Clone(st.Sources)
	if sources == nil {
		sources = map[string]state.Source{}
	}
	return StateFile{
		Version:       formatVersion,
		GeneratedAtMs: nowMs,
		ServiceOK:     true,
		Cards:         cards,
		Limits:        limits,
		Sources:       sources,
		SessionNames:  names,
		View:          v,
		NowPlaying:    song,
	}
}

// WriteAtomic writes f to path through a temp file in the same folder and a
// rename, so the saver never reads a half-written file.
func WriteAtomic(path string, f StateFile) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode state file: %w", err)
	}
	if err := atomicfile.Write(path, data, fileMode); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

// ReadLimits returns the limits of the state file at path, so a restarted
// service shows the last readings at once. A missing file has none, and so
// does a demo file: its limits are made up.
func ReadLimits(path string) ([]state.Limit, error) {
	file, err := readLast(path)
	if file.Demo {
		return nil, err
	}
	return file.Limits, err
}

// ReadSources returns the last event time of each source in the state file
// at path, so a restart does not lose them. A missing file has none.
func ReadSources(path string) (map[string]state.Source, error) {
	file, err := readLast(path)
	return file.Sources, err
}

// ReadSessionNames returns the session names of the state file at path.
// A missing file has none. A demo file has the real names: the demo
// replaces only the cards and limits.
func ReadSessionNames(path string) ([]state.SessionName, error) {
	file, err := readLast(path)
	return file.SessionNames, err
}

// lastRun is the part of a state file the service reads back.
type lastRun struct {
	Limits       []state.Limit           `json:"limits"`
	Sources      map[string]state.Source `json:"sources"`
	SessionNames []state.SessionName     `json:"session_names"`
	Demo         bool                    `json:"demo"`
}

func readLast(path string) (lastRun, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return lastRun{}, nil
	}
	if err != nil {
		return lastRun{}, fmt.Errorf("read state file: %w", err)
	}
	var file lastRun
	if err := json.Unmarshal(data, &file); err != nil {
		return lastRun{}, fmt.Errorf("decode state file: %w", err)
	}
	return file, nil
}
