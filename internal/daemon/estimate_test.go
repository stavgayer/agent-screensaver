package daemon

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
	"agent-screensaver/internal/usage"
	"agent-screensaver/internal/view"
)

type observation struct {
	readings []event.LimitReading
	atMs     int64
}

type fakeEstimator struct {
	observed  []observation
	estimates []int64 // the times Estimate was called
	result    usage.Result
	err       error
	flushes   int
}

func (f *fakeEstimator) Source() string { return "claude" }

func (f *fakeEstimator) Flush() error {
	f.flushes++
	return nil
}

func (f *fakeEstimator) Observe(readings []event.LimitReading, atMs int64) {
	f.observed = append(f.observed, observation{readings, atMs})
}

func (f *fakeEstimator) Estimate(nowMs int64) (usage.Result, error) {
	f.estimates = append(f.estimates, nowMs)
	return f.result, f.err
}

func estimatedReading(window event.Window, percent float64, atMs int64) usage.Reading {
	return usage.Reading{LimitReading: event.LimitReading{Window: window, UsedPercent: percent, Estimated: true}, AtMs: atMs}
}

func TestEstimatesReachTheStateFile(t *testing.T) {
	d, p := newTestDaemon(t)
	estimator := &fakeEstimator{result: usage.Result{Readings: []usage.Reading{
		estimatedReading(event.FiveHour, 42, 900),
		{LimitReading: event.LimitReading{Window: event.Week, UsedPercent: 60}, AtMs: 400},
	}}}
	d.Estimator = estimator

	d.Step(1_000, false)

	want := []state.Limit{
		{Source: "claude", Window: event.FiveHour, UsedPercent: 42, UpdatedAtMs: 900, Estimated: true},
		{Source: "claude", Window: event.Week, UsedPercent: 60, UpdatedAtMs: 400},
	}
	if got := readStateFile(t, p).Limits; !reflect.DeepEqual(got, want) {
		t.Errorf("limits =\n %+v\nwant\n %+v", got, want)
	}
}

func TestStatusLineReadingsAreObservedAndEstimatedAtOnce(t *testing.T) {
	d, p := newTestDaemon(t)
	estimator := &fakeEstimator{}
	d.Estimator = estimator
	d.Step(0, false)

	reading := event.LimitReading{Window: event.FiveHour, UsedPercent: 30}
	limits := event.Event{V: event.Version, Source: "claude", Session: "statusline", Kind: event.KindLimits, AtMs: 1_500,
		Limits: []event.LimitReading{reading}}
	codex := event.Event{V: event.Version, Source: "codex", Session: "x", Kind: event.KindLimits, AtMs: 1_500,
		Limits: []event.LimitReading{reading}}
	for _, incoming := range []event.Event{limits, codex} {
		if err := spool.Write(p.SpoolDir, incoming); err != nil {
			t.Fatal(err)
		}
	}
	d.Step(2_000, false)

	want := []observation{{[]event.LimitReading{reading}, 1_500}}
	if !reflect.DeepEqual(estimator.observed, want) {
		t.Errorf("observed = %+v, want only the claude reading", estimator.observed)
	}
	if !reflect.DeepEqual(estimator.estimates, []int64{0, 2_000}) {
		t.Errorf("estimates at %v, want at the start and right after the reading", estimator.estimates)
	}
}

func TestEstimatesAreSpacedOutUnlessCatchingUp(t *testing.T) {
	tests := []struct {
		name       string
		catchingUp bool
		want       []int64
	}{
		{"every 10 s", false, []int64{0, 10_000, 20_000}},
		{"every step while the first read goes on", true, []int64{0, 500, 5_000, 10_000, 15_000, 20_000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := newTestDaemon(t)
			estimator := &fakeEstimator{result: usage.Result{CatchingUp: tt.catchingUp}}
			d.Estimator = estimator
			for _, nowMs := range []int64{0, 500, 5_000, 10_000, 15_000, 20_000} {
				d.Step(nowMs, false)
			}
			if !reflect.DeepEqual(estimator.estimates, tt.want) {
				t.Errorf("estimates at %v, want %v", estimator.estimates, tt.want)
			}
		})
	}
}

func TestEstimateErrorsAreLoggedOncePerChange(t *testing.T) {
	d, _ := newTestDaemon(t)
	var logged []string
	d.Logf = func(format string, args ...any) { logged = append(logged, format) }
	estimator := &fakeEstimator{err: errors.New("broken file")}
	d.Estimator = estimator
	for _, nowMs := range []int64{0, 10_000, 20_000} {
		d.Step(nowMs, false)
	}
	estimator.err = nil
	d.Step(30_000, false)
	if len(logged) != 1 {
		t.Errorf("logged %d times, want once: %v", len(logged), logged)
	}
}

func TestLimitsOfTheLastRunAreShownAtStart(t *testing.T) {
	d, p := newTestDaemon(t)
	before := []state.Limit{{Source: "codex", Window: event.Week, UsedPercent: 12, UpdatedAtMs: 100}}
	if err := statefile.WriteAtomic(p.StateFile, statefile.Build(state.State{Limits: before}, view.View{}, nil, 100)); err != nil {
		t.Fatal(err)
	}
	d.Step(1_000, false)
	if got := readStateFile(t, p).Limits; !reflect.DeepEqual(got, before) {
		t.Errorf("limits = %+v, want %+v", got, before)
	}
}

// The estimator speaks for its source: an exact reading it gives after an
// estimate replaces it with its own time, so an old number never looks new.
func TestAnExactReadingAfterAnEstimateKeepsItsOwnTime(t *testing.T) {
	d, p := newTestDaemon(t)
	estimator := &fakeEstimator{result: usage.Result{Readings: []usage.Reading{estimatedReading(event.FiveHour, 40, 2_000)}}}
	d.Estimator = estimator
	d.Step(2_000, false)
	resets := int64(5)
	estimator.result = usage.Result{Readings: []usage.Reading{
		{LimitReading: event.LimitReading{Window: event.FiveHour, UsedPercent: 30, ResetsAt: &resets}, AtMs: 1_000}}}
	d.Step(12_000, false)
	want := []state.Limit{{Source: "claude", Window: event.FiveHour, UsedPercent: 30, ResetsAt: &resets, UpdatedAtMs: 1_000}}
	if got := readStateFile(t, p).Limits; !reflect.DeepEqual(got, want) {
		t.Errorf("limits =\n %+v\nwant\n %+v", got, want)
	}
}

func TestEstimatorIsFlushedWhenTheServiceStops(t *testing.T) {
	d, _ := newTestDaemon(t)
	estimator := &fakeEstimator{}
	d.Estimator = estimator
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, d, &fakeWatcher{}, &fakeSound{}, nil); err != nil {
		t.Fatal(err)
	}
	if estimator.flushes != 1 {
		t.Errorf("flushed %d times, want once", estimator.flushes)
	}
}
