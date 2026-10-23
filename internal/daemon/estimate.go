package daemon

import (
	"reflect"
	"slices"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/statefile"
	"agent-screensaver/internal/usage"
)

// estimateEveryMs is how often the live limits are worked out again. Each
// time costs a stat of every recent transcript and a read of what was added.
const estimateEveryMs = 10_000

// LimitEstimator keeps one source's limits live between exact readings.
type LimitEstimator interface {
	// Source is the limits source it speaks for, for example "claude".
	Source() string
	// Observe gives it an exact reading that arrived as an event.
	Observe(readings []event.LimitReading, atMs int64)
	// Estimate returns the current readings.
	Estimate(nowMs int64) (usage.Result, error)
	// Flush saves what it learned; called when the service stops.
	Flush() error
}

// observeLimits hands the exact readings of the estimator's source to it.
func (d *Daemon) observeLimits(incoming event.Event) {
	if d.Estimator == nil || incoming.Kind != event.KindLimits || incoming.Source != d.Estimator.Source() {
		return
	}
	d.Estimator.Observe(incoming.Limits, incoming.AtMs)
	d.estimateDue = true
}

// estimateLimits folds the estimator's readings into the state when one is
// due and reports whether a limit changed. While the estimator catches up
// on a large history it is asked again on every step.
func (d *Daemon) estimateLimits(nowMs int64) bool {
	if d.Estimator == nil {
		return false
	}
	if !d.estimateDue && d.estimatedOnce && nowMs-d.lastEstimateMs < estimateEveryMs {
		return false
	}
	d.estimatedOnce, d.lastEstimateMs = true, nowMs
	result, err := d.Estimator.Estimate(nowMs)
	d.estimateDue = result.CatchingUp
	d.reportEstimateError(err)
	before := slices.Clone(d.state.Limits)
	for _, reading := range result.Readings {
		d.state.ReplaceLimit(d.Estimator.Source(), reading.LimitReading, reading.AtMs)
	}
	return !reflect.DeepEqual(before, d.state.Limits)
}

// reportEstimateError logs a problem once, not on every poll.
func (d *Daemon) reportEstimateError(err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if message != "" && message != d.estimateErr {
		d.Logf("estimate %s limits: %s", d.Estimator.Source(), message)
	}
	d.estimateErr = message
}

// restoreLimits shows the limits of the last run until new readings come,
// so the rings are not empty after a restart.
func (d *Daemon) restoreLimits() {
	limits, err := statefile.ReadLimits(d.paths.StateFile)
	if err != nil {
		d.Logf("restore limits: %v", err)
		return
	}
	d.state.Limits = limits
}

// restoreSources reads back the last event time of each source, which the
// Connections tab shows, so a restart does not lose it.
func (d *Daemon) restoreSources() {
	sources, err := statefile.ReadSources(d.paths.StateFile)
	if err != nil {
		d.Logf("restore sources: %v", err)
		return
	}
	for source, known := range sources {
		d.state.NoteEvent(source, known.LastEventMs)
	}
}
