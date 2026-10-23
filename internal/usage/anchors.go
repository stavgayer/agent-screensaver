// Package usage keeps the Claude usage limits live between exact readings.
// Exact readings (anchors) come from the status line and from files Claude
// itself writes; between them the percent is estimated from the tokens in
// the local transcripts. Everything is read from local files: no network,
// no login tokens.
package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"agent-screensaver/internal/event"
)

// Bounds on the files read whole. The desktop history keeps 30 days of
// samples (about 16 KiB on a busy machine); ~/.claude.json holds every
// project's settings and grows with them.
const (
	maxHistoryBytes  = 4 << 20
	maxCLICacheBytes = 16 << 20
)

// Anchor is one exact reading of a limit window.
type Anchor struct {
	Window  event.Window `json:"window"`
	Percent float64      `json:"percent"`
	// ResetsAt is the window end in Unix seconds; nil when the source does
	// not say.
	ResetsAt *int64 `json:"resets_at,omitempty"`
	AtMs     int64  `json:"at_ms"`
}

// AnchorSource reads exact readings that a tool left on disk.
type AnchorSource interface {
	Anchors() ([]Anchor, error)
}

// NewForMac returns an estimator over Claude's files on macOS: the anchor
// files and ~/.claude/projects under home, with its calibration kept in
// storeFile.
func NewForMac(home fs.FS, storeFile string) *Estimator {
	return &Estimator{
		Sources:   MacSources(home),
		Tally:     &Tally{FS: home, Root: ".claude/projects", Weights: DefaultWeights},
		StoreFile: storeFile,
	}
}

// MacSources are the files Claude writes on macOS, relative to the home
// folder that home is rooted at.
func MacSources(home fs.FS) []AnchorSource {
	return []AnchorSource{
		&DesktopHistory{FS: home, Path: "Library/Application Support/Claude/plan-usage-history.json"},
		&CLICache{FS: home, Path: ".claude.json"},
	}
}

// DesktopHistory reads the plan usage samples the Claude desktop app keeps:
// {"samples":[{"t": ms, "u": {"fh": 5h percent, "sd": week percent}}]}.
// Samples have no reset time.
type DesktopHistory struct {
	FS    fs.FS
	Path  string
	cache fileCache
}

// Anchors returns one anchor per window of every sample, oldest first.
func (h *DesktopHistory) Anchors() ([]Anchor, error) {
	return h.cache.load(h.FS, h.Path, maxHistoryBytes, parseDesktopHistory)
}

func parseDesktopHistory(data []byte) ([]Anchor, error) {
	var history struct {
		Samples []struct {
			T int64 `json:"t"`
			U struct {
				FiveHour *float64 `json:"fh"`
				Week     *float64 `json:"sd"`
			} `json:"u"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, fmt.Errorf("decode desktop usage history: %w", err)
	}
	var anchors []Anchor
	for _, sample := range history.Samples {
		anchors = appendAnchor(anchors, event.FiveHour, sample.U.FiveHour, nil, sample.T)
		anchors = appendAnchor(anchors, event.Week, sample.U.Week, nil, sample.T)
	}
	return anchors, nil
}

// CLICache reads cachedUsageUtilization in ~/.claude.json, which Claude Code
// writes when /usage runs. Only that key is decoded; the file holds other
// private data that is never kept or logged.
type CLICache struct {
	FS    fs.FS
	Path  string
	cache fileCache
}

// Anchors returns the 5h and week readings of the last /usage.
func (c *CLICache) Anchors() ([]Anchor, error) {
	return c.cache.load(c.FS, c.Path, maxCLICacheBytes, parseCLICache)
}

type cachedWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

func parseCLICache(data []byte) ([]Anchor, error) {
	var file struct {
		Cached *struct {
			FetchedAtMs int64 `json:"fetchedAtMs"`
			Utilization struct {
				FiveHour *cachedWindow `json:"five_hour"`
				SevenDay *cachedWindow `json:"seven_day"`
			} `json:"utilization"`
		} `json:"cachedUsageUtilization"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode cached usage: %w", err)
	}
	if file.Cached == nil {
		return nil, nil
	}
	var anchors []Anchor
	for _, window := range []struct {
		name  event.Window
		found *cachedWindow
	}{{event.FiveHour, file.Cached.Utilization.FiveHour}, {event.Week, file.Cached.Utilization.SevenDay}} {
		if window.found == nil {
			continue
		}
		anchors = appendAnchor(anchors, window.name, window.found.Utilization, parseResetTime(window.found.ResetsAt), file.Cached.FetchedAtMs)
	}
	return anchors, nil
}

// parseResetTime reads "2026-10-23T13:00:00.123456+00:00" as Unix seconds.
func parseResetTime(text string) *int64 {
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil
	}
	seconds := parsed.Unix()
	return &seconds
}

func appendAnchor(anchors []Anchor, window event.Window, percent *float64, resetsAt *int64, atMs int64) []Anchor {
	if percent == nil {
		return anchors
	}
	return append(anchors, Anchor{Window: window, Percent: min(max(*percent, 0), 100), ResetsAt: resetsAt, AtMs: atMs})
}

// fileCache parses a file again only when its modification time or size
// changed, so a poll of an unchanged file costs one stat.
type fileCache struct {
	modNs   int64
	size    int64
	loaded  bool
	anchors []Anchor
}

func (c *fileCache) load(fsys fs.FS, path string, maxBytes int64, parse func([]byte) ([]Anchor, error)) ([]Anchor, error) {
	info, err := fs.Stat(fsys, path)
	if errors.Is(err, fs.ErrNotExist) {
		*c = fileCache{}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if c.loaded && info.ModTime().UnixNano() == c.modNs && info.Size() == c.size {
		return c.anchors, nil
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s is %d bytes, limit is %d", path, info.Size(), maxBytes)
	}
	data, err := readBounded(fsys, path, maxBytes)
	if err != nil {
		return nil, err
	}
	anchors, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	*c = fileCache{modNs: info.ModTime().UnixNano(), size: info.Size(), loaded: true, anchors: anchors}
	return anchors, nil
}

// readBounded reads at most maxBytes of a file, so a file that grew after
// the stat cannot take more memory than allowed.
func readBounded(fsys fs.FS, path string, maxBytes int64) ([]byte, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}
