package usage

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"agent-screensaver/internal/atomicfile"
	"agent-screensaver/internal/event"
)

const (
	hourMs = 3_600_000
	// horizonMs is how far back transcripts and anchors are used: the week
	// window plus a day.
	horizonMs = 8 * 24 * hourMs
	// defaultScanBudget bounds the bytes read from transcripts per poll, so
	// the first read of a large history is spread over several polls.
	defaultScanBudget = 32 << 20
	// saveEveryMs bounds how often the calibration file is written.
	saveEveryMs = 60_000
	// resetToleranceS treats two reset times this close as the same window
	// end: /usage reports them to the microsecond.
	resetToleranceS = 300
	storeVersion    = 1
)

// limitWindow is a Claude limit window. minRise is how many points the
// percent must rise between two anchors before their ratio is trusted:
// the desktop app reports whole percents, so a 1 point rise can be off by
// half a point either way.
type limitWindow struct {
	name     event.Window
	lengthMs int64
	minRise  float64
}

var limitWindows = []limitWindow{
	{name: event.FiveHour, lengthMs: 5 * hourMs, minRise: 3},
	{name: event.Week, lengthMs: 7 * 24 * hourMs, minRise: 2},
}

// Calibration is what the estimator knows about one window. It is saved so
// a restarted service estimates at once.
type Calibration struct {
	// Latest is the newest exact reading.
	Latest *Anchor `json:"latest,omitempty"`
	// Base is the reading the next ratio is measured from.
	Base *Anchor `json:"base,omitempty"`
	// PercentPerToken turns weighted tokens into percent; 0 is unknown.
	PercentPerToken float64 `json:"percent_per_token,omitempty"`
	// ResetsAt is the last known window end (Unix seconds), used for
	// anchors that do not say it.
	ResetsAt *int64 `json:"resets_at,omitempty"`
}

// Reading is one window's reading as of AtMs.
type Reading struct {
	event.LimitReading
	AtMs int64
}

// Result is what one Estimate call found.
type Result struct {
	Readings []Reading
	// CatchingUp is true while the first read of the transcripts is not
	// done: call Estimate again soon. Only exact readings are given then.
	CatchingUp bool
}

// Estimator keeps the Claude 5h and week limits live. The newest exact
// reading (anchor) of a window wins; when tokens were used after it, the
// estimate is the anchor percent plus those weighted tokens times a ratio
// learned from two earlier anchors of the same window. Without a ratio it
// gives the anchor only, never a made-up number.
type Estimator struct {
	Sources []AnchorSource
	Tally   *Tally
	// StoreFile keeps the calibration across restarts.
	StoreFile string

	scanBudget int64 // 0 means defaultScanBudget; tests set it
	loaded     bool
	windows    map[event.Window]*Calibration
	observed   []Anchor
	dirty      bool
	savedMs    int64
}

// Source is the limits source the estimator speaks for.
func (e *Estimator) Source() string { return "claude" }

// Observe records exact readings that arrived as an event (the status line).
func (e *Estimator) Observe(readings []event.LimitReading, atMs int64) {
	for _, reading := range readings {
		if _, ok := windowOf(reading.Window); ok && !reading.Estimated {
			e.observed = append(e.observed, Anchor{Window: reading.Window, Percent: reading.UsedPercent, ResetsAt: copyInt64(reading.ResetsAt), AtMs: atMs})
		}
	}
}

// Estimate reads the anchor files and the new transcript lines and returns
// the current reading of each window it knows. Errors are reported but do
// not stop it: what could be read is still used.
func (e *Estimator) Estimate(nowMs int64) (Result, error) {
	var problems []error
	if !e.loaded {
		problems = append(problems, e.load())
		e.loaded = true
	}
	budget := e.scanBudget
	if budget == 0 {
		budget = defaultScanBudget
	}
	caughtUp, err := e.Tally.Scan(nowMs-horizonMs, budget)
	problems = append(problems, err)
	anchors := slices.Clone(e.observed)
	for _, source := range e.Sources {
		found, err := source.Anchors()
		problems = append(problems, err)
		anchors = append(anchors, found...)
	}
	if caughtUp {
		// Ratios need the tokens between anchors, so anchors are folded in
		// only once the transcripts are read.
		e.advance(anchors, nowMs)
		e.observed = nil
	}
	result := Result{CatchingUp: !caughtUp}
	for _, window := range limitWindows {
		if reading, ok := e.reading(window, anchors, caughtUp, nowMs); ok {
			result.Readings = append(result.Readings, reading)
		}
	}
	if e.dirty && nowMs-e.savedMs >= saveEveryMs {
		problems = append(problems, e.save())
		e.savedMs = nowMs
	}
	return result, errors.Join(problems...)
}

// advance folds the anchors newer than each window's latest one in, oldest
// first, learning a ratio from each pair that allows it.
func (e *Estimator) advance(anchors []Anchor, nowMs int64) {
	slices.SortStableFunc(anchors, func(a, b Anchor) int { return cmp.Compare(a.AtMs, b.AtMs) })
	for _, anchor := range anchors {
		window, ok := windowOf(anchor.Window)
		if !ok || anchor.AtMs <= nowMs-horizonMs {
			continue
		}
		calibration := e.calibration(anchor.Window)
		if calibration.Latest != nil && anchor.AtMs <= calibration.Latest.AtMs {
			continue
		}
		calibration.add(anchor, window, e.Tally)
		e.dirty = true
	}
}

// add makes anchor the latest reading. An anchor without a reset time takes
// the last known one when it was read inside that window.
func (c *Calibration) add(anchor Anchor, window limitWindow, tally *Tally) {
	if anchor.ResetsAt == nil && c.ResetsAt != nil && insideWindow(anchor.AtMs, *c.ResetsAt, window.lengthMs) {
		anchor.ResetsAt = copyInt64(c.ResetsAt)
	}
	if anchor.ResetsAt != nil {
		c.ResetsAt = copyInt64(anchor.ResetsAt)
	}
	c.Latest = &anchor
	base := c.Base
	if base == nil || !samePeriod(*base, anchor, window.lengthMs) || anchor.Percent < base.Percent {
		c.Base = &anchor
		return
	}
	rise := anchor.Percent - base.Percent
	if rise < window.minRise {
		return // keep the base until the rise is big enough to trust
	}
	if tokens := tally.Between(base.AtMs, anchor.AtMs); tokens > 0 {
		c.PercentPerToken = rise / tokens
	}
	// With no tokens the rise came from use the transcripts do not show
	// (claude.ai chat, Cowork, another computer): no ratio to learn.
	c.Base = &anchor
}

// reading gives the window's current reading: the estimate when a ratio is
// known and tokens were used after the latest anchor, else that anchor.
func (e *Estimator) reading(window limitWindow, pending []Anchor, caughtUp bool, nowMs int64) (Reading, bool) {
	calibration := e.calibration(window.name)
	latest := calibration.Latest
	if !caughtUp {
		latest = newest(latest, pending, window.name)
	}
	if latest == nil {
		return Reading{}, false
	}
	exact := Reading{
		LimitReading: event.LimitReading{Window: window.name, UsedPercent: latest.Percent, ResetsAt: copyInt64(latest.ResetsAt)},
		AtMs:         latest.AtMs,
	}
	if !caughtUp || calibration.PercentPerToken <= 0 {
		return exact, true
	}
	from, basePercent, resetsAt := latest.AtMs, latest.Percent, copyInt64(latest.ResetsAt)
	closedAtMs, closed := int64(0), false
	switch {
	case resetsAt != nil && nowMs >= *resetsAt*1000:
		closedAtMs, closed = *resetsAt*1000, true
	case resetsAt == nil && nowMs >= latest.AtMs+window.lengthMs:
		// The window of the reading closed one length after it at the
		// latest; calls before that point may count a little too much.
		closedAtMs, closed = latest.AtMs+window.lengthMs, true
	}
	if closed {
		// Count from zero in the window open now. Its end is not known
		// until the next exact reading.
		start, open := e.Tally.windowStart(closedAtMs, window.lengthMs, nowMs)
		if !open {
			return exact, true
		}
		from, basePercent, resetsAt = start-1, 0, nil
	}
	tokens := e.Tally.Between(from, nowMs)
	if tokens == 0 {
		return exact, true
	}
	return Reading{
		LimitReading: event.LimitReading{
			Window:      window.name,
			UsedPercent: min(max(basePercent+tokens*calibration.PercentPerToken, 0), 100),
			ResetsAt:    resetsAt,
			Estimated:   true,
		},
		AtMs: nowMs,
	}, true
}

func (e *Estimator) calibration(window event.Window) *Calibration {
	if e.windows == nil {
		e.windows = map[event.Window]*Calibration{}
	}
	if e.windows[window] == nil {
		e.windows[window] = &Calibration{}
	}
	return e.windows[window]
}

type store struct {
	Version int                           `json:"version"`
	Windows map[event.Window]*Calibration `json:"windows"`
}

func (e *Estimator) load() error {
	data, err := os.ReadFile(e.StoreFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read usage calibration: %w", err)
	}
	var saved store
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("decode usage calibration: %w", err)
	}
	if saved.Version != storeVersion {
		return nil // an unknown format: start over
	}
	e.windows = saved.Windows
	return nil
}

func (e *Estimator) save() error {
	data, err := json.Marshal(store{Version: storeVersion, Windows: e.windows})
	if err != nil {
		return fmt.Errorf("encode usage calibration: %w", err)
	}
	if err := atomicfile.Write(e.StoreFile, data, 0o600); err != nil {
		return fmt.Errorf("save usage calibration: %w", err)
	}
	e.dirty = false
	return nil
}

// samePeriod says whether two anchors were read in the same window.
func samePeriod(earlier, later Anchor, lengthMs int64) bool {
	switch {
	case earlier.ResetsAt != nil && later.ResetsAt != nil:
		return max(*earlier.ResetsAt-*later.ResetsAt, *later.ResetsAt-*earlier.ResetsAt) <= resetToleranceS
	case later.ResetsAt != nil:
		return insideWindow(earlier.AtMs, *later.ResetsAt, lengthMs)
	case earlier.ResetsAt != nil:
		// The later anchor would have taken the known end if it were read
		// inside that window.
		return false
	default:
		// No end known: trust only pairs close enough that a reset and a
		// climb past the old percent are unlikely in between.
		return later.AtMs-earlier.AtMs <= lengthMs/2
	}
}

func insideWindow(atMs, resetsAtS, lengthMs int64) bool {
	end := resetsAtS * 1000
	return atMs < end && atMs >= end-lengthMs
}

func newest(latest *Anchor, anchors []Anchor, window event.Window) *Anchor {
	for index := range anchors {
		if anchors[index].Window == window && (latest == nil || anchors[index].AtMs > latest.AtMs) {
			latest = &anchors[index]
		}
	}
	return latest
}

func windowOf(name event.Window) (limitWindow, bool) {
	index := slices.IndexFunc(limitWindows, func(window limitWindow) bool { return window.name == name })
	if index < 0 {
		return limitWindow{}, false
	}
	return limitWindows[index], true
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
