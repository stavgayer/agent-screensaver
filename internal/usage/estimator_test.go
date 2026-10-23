package usage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"agent-screensaver/internal/event"
)

const minuteMs = 60_000

type fakeSource struct {
	anchors []Anchor
	err     error
}

func (f *fakeSource) Anchors() ([]Anchor, error) { return f.anchors, f.err }

// use is one model call: weighted tokens at a time. Its output tokens are
// weighted/5, so the fixture line weighs exactly that.
type use struct {
	atMs     int64
	weighted int
}

type estimatorSetup struct {
	root      string
	estimator *Estimator
	source    *fakeSource
}

func newEstimator(t *testing.T, uses ...use) estimatorSetup {
	t.Helper()
	root := t.TempDir()
	setup := estimatorSetup{root: root, source: &fakeSource{}}
	setup.addUses(t, uses...)
	setup.estimator = &Estimator{
		Sources:   []AnchorSource{setup.source},
		Tally:     &Tally{FS: os.DirFS(root), Root: "projects", Weights: DefaultWeights},
		StoreFile: filepath.Join(root, "data", "claude-usage.json"),
	}
	return setup
}

func (s estimatorSetup) addUses(t *testing.T, uses ...use) {
	t.Helper()
	for _, call := range uses {
		appendTo(t, filepath.Join(s.root, "projects", "p", "s.jsonl"),
			assistantLine(fmt.Sprint(call.atMs, call.weighted), time.UnixMilli(call.atMs), call.weighted/5))
	}
}

func anchor(window event.Window, percent float64, atMs int64) Anchor {
	return Anchor{Window: window, Percent: percent, AtMs: atMs}
}

func withReset(a Anchor, resetsAtMs int64) Anchor {
	a.ResetsAt = seconds(resetsAtMs)
	return a
}

func exact(window event.Window, percent float64, resetsAtMs, atMs int64) Reading {
	reading := Reading{LimitReading: event.LimitReading{Window: window, UsedPercent: percent}, AtMs: atMs}
	if resetsAtMs != 0 {
		reading.ResetsAt = seconds(resetsAtMs)
	}
	return reading
}

func estimated(window event.Window, percent float64, resetsAtMs, atMs int64) Reading {
	reading := exact(window, percent, resetsAtMs, atMs)
	reading.Estimated = true
	return reading
}

func TestEstimate(t *testing.T) {
	tests := []struct {
		name    string
		uses    []use
		anchors []Anchor
		nowMs   int64
		want    []Reading
	}{
		{
			name:    "no ratio yet: only the anchor, never a made-up number",
			uses:    []use{{at10 + 10*minuteMs, 1000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10)},
			nowMs:   at10 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 10, 0, at10)},
		},
		{
			// 10 points over 1,000 weighted tokens is 0.01 per token; 500
			// tokens after the latest anchor add 5 points.
			name:    "a ratio from two anchors estimates the tokens used since",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at11 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{estimated(event.FiveHour, 25, 0, at11+30*minuteMs)},
		},
		{
			name:    "no tokens since the latest anchor: the anchor stays exact",
			uses:    []use{{at10 + 10*minuteMs, 1000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 20, 0, at11)},
		},
		{
			// 10 -> 11 is too small to trust; 10 -> 20 over all 1,000
			// tokens gives 0.01.
			name: "a small rise keeps the base until the rise can be trusted",
			uses: []use{{at10 + 10*minuteMs, 400}, {at10 + 40*minuteMs, 600}, {at11 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 11, at10+30*minuteMs),
				anchor(event.FiveHour, 20, at11)},
			nowMs: at11 + 30*minuteMs,
			want:  []Reading{estimated(event.FiveHour, 25, 0, at11+30*minuteMs)},
		},
		{
			name:    "the estimate stops at 100",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at11 + 10*minuteMs, 100_000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{estimated(event.FiveHour, 100, 0, at11+30*minuteMs)},
		},
		{
			// The window of the 11:00 reading ends at 12:00; only the 300
			// tokens after 12:00 count, from zero.
			name: "after a known reset the estimate starts again from zero",
			uses: []use{{at10 + 10*minuteMs, 1000}, {at11 + 30*minuteMs, 2000}, {at12 + 10*minuteMs, 300}},
			anchors: []Anchor{withReset(anchor(event.FiveHour, 10, at10), at12),
				withReset(anchor(event.FiveHour, 20, at11), at12)},
			nowMs: at12 + 30*minuteMs,
			want:  []Reading{estimated(event.FiveHour, 3, 0, at12+30*minuteMs)},
		},
		{
			// The 10:00 window closed by 15:00 at the latest. A window opened
			// at 16:00 and closed at 21:00; the one open now began at 22:00,
			// so only its 200 tokens count.
			name: "with no known end, only the window open now counts",
			uses: []use{{at10 + 10*minuteMs, 1000}, {at10 + 20*minuteMs, 1000}, {at10 + 6*hourMs, 5000},
				{at10 + 8*hourMs, 5000}, {at10 + 12*hourMs, 200}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at10+15*minuteMs)},
			nowMs:   at10 + 13*hourMs,
			want:    []Reading{estimated(event.FiveHour, 2, 0, at10+13*hourMs)},
		},
		{
			name:    "no call in a window open now: the old anchor stays",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at10 + 6*hourMs, 5000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at10+15*minuteMs)},
			nowMs:   at10 + 12*hourMs,
			want:    []Reading{exact(event.FiveHour, 20, 0, at10+15*minuteMs)},
		},
		{
			name:    "after a reset with no ratio the old anchor stays, so the ring says Reset",
			uses:    []use{{at12 + 10*minuteMs, 300}},
			anchors: []Anchor{withReset(anchor(event.FiveHour, 20, at11), at12)},
			nowMs:   at12 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 20, at12, at11)},
		},
		{
			name: "an anchor without a reset time takes the known end of its window",
			anchors: []Anchor{withReset(anchor(event.FiveHour, 30, at10), at13),
				anchor(event.FiveHour, 35, at11)},
			nowMs: at11 + minuteMs,
			want:  []Reading{exact(event.FiveHour, 35, at13, at11)},
		},
		{
			name: "an anchor after that window does not take its end",
			anchors: []Anchor{withReset(anchor(event.FiveHour, 30, at10), at11),
				anchor(event.FiveHour, 5, at12)},
			nowMs: at12 + minuteMs,
			want:  []Reading{exact(event.FiveHour, 5, 0, at12)},
		},
		{
			// 80 -> 5 is a new window, not a negative ratio; 5 -> 15 then
			// gives 10 points over 1,000 tokens.
			name: "a drop starts a new base",
			uses: []use{{at10 + 10*minuteMs, 9999}, {at11 + 10*minuteMs, 1000}, {at12 + 10*minuteMs, 100}},
			anchors: []Anchor{anchor(event.FiveHour, 80, at10), anchor(event.FiveHour, 5, at11),
				anchor(event.FiveHour, 15, at12)},
			nowMs: at12 + 30*minuteMs,
			want:  []Reading{estimated(event.FiveHour, 16, 0, at12+30*minuteMs)},
		},
		{
			name:    "anchors far apart with no known end teach nothing",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at13 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at13)},
			nowMs:   at13 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 20, 0, at13)},
		},
		{
			name:    "windows are calibrated apart; the newest anchor of each wins",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at11 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.Week, 40, at10), anchor(event.FiveHour, 7, at11), anchor(event.Week, 44, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 7, 0, at11), estimated(event.Week, 46, 0, at11+30*minuteMs)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setup := newEstimator(t, tt.uses...)
			setup.source.anchors = tt.anchors
			result, err := setup.estimator.Estimate(tt.nowMs)
			if err != nil || result.CatchingUp {
				t.Fatalf("Estimate error %v, catching up %v", err, result.CatchingUp)
			}
			if !reflect.DeepEqual(result.Readings, tt.want) {
				t.Errorf("readings =\n %s\nwant\n %s", show(result.Readings), show(tt.want))
			}
		})
	}
}

// The status line reading arrives as an event and wins over older files.
func TestObservedReadingIsTheNewestAnchor(t *testing.T) {
	setup := newEstimator(t, use{at10 + 10*minuteMs, 1000}, use{at11 + 10*minuteMs, 500})
	setup.source.anchors = []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 15, at10+20*minuteMs)}
	setup.estimator.Observe([]event.LimitReading{
		{Window: event.FiveHour, UsedPercent: 20, ResetsAt: seconds(at13)},
		{Window: event.Plan, UsedPercent: 1}, // not a Claude window
	}, at11)
	result, err := setup.estimator.Estimate(at11 + 30*minuteMs)
	if err != nil {
		t.Fatal(err)
	}
	// 5 points over 1,000 tokens from the files; the 500 tokens after the
	// status line reading add 2.5 to its 20.
	want := []Reading{estimated(event.FiveHour, 22.5, at13, at11+30*minuteMs)}
	if !reflect.DeepEqual(result.Readings, want) {
		t.Errorf("readings =\n %s\nwant\n %s", show(result.Readings), show(want))
	}
}

// While the first read of a large history is not done, only exact readings
// are given, and ratios are learned once it is.
func TestEstimateWhileCatchingUp(t *testing.T) {
	setup := newEstimator(t, use{at10 + 10*minuteMs, 1000}, use{at10 + 20*minuteMs, 1000}, use{at11 + 10*minuteMs, 1000})
	setup.estimator.scanBudget = 300 // about one line per call
	setup.source.anchors = []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 30, at11)}
	nowMs := at11 + 30*minuteMs
	first, err := setup.estimator.Estimate(nowMs)
	if err != nil {
		t.Fatal(err)
	}
	if !first.CatchingUp || !reflect.DeepEqual(first.Readings, []Reading{exact(event.FiveHour, 30, 0, at11)}) {
		t.Fatalf("first = %+v, want catching up with the newest anchor", first)
	}
	var last Result
	for range 10 {
		if last, err = setup.estimator.Estimate(nowMs); err != nil {
			t.Fatal(err)
		}
		if !last.CatchingUp {
			break
		}
	}
	// 20 points over 2,000 tokens; 1,000 tokens since add 10.
	want := []Reading{estimated(event.FiveHour, 40, 0, nowMs)}
	if last.CatchingUp || !reflect.DeepEqual(last.Readings, want) {
		t.Errorf("after catching up = %+v, want %s", last, show(want))
	}
}

func TestCalibrationSurvivesARestart(t *testing.T) {
	setup := newEstimator(t, use{at10 + 10*minuteMs, 1000}, use{at11 + 10*minuteMs, 500})
	setup.source.anchors = []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)}
	if _, err := setup.estimator.Estimate(at11 + 5*minuteMs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(setup.estimator.StoreFile); err != nil {
		t.Fatalf("calibration not saved: %v", err)
	}

	restarted := &Estimator{
		Tally:     &Tally{FS: os.DirFS(setup.root), Root: "projects", Weights: DefaultWeights},
		StoreFile: setup.estimator.StoreFile,
	}
	result, err := restarted.Estimate(at11 + 30*minuteMs)
	if err != nil {
		t.Fatal(err)
	}
	want := []Reading{estimated(event.FiveHour, 25, 0, at11+30*minuteMs)}
	if !reflect.DeepEqual(result.Readings, want) {
		t.Errorf("after restart =\n %s\nwant\n %s", show(result.Readings), show(want))
	}
}

func TestCalibrationSavesAreSpacedOut(t *testing.T) {
	setup := newEstimator(t)
	setup.source.anchors = []Anchor{anchor(event.FiveHour, 10, at10)}
	if _, err := setup.estimator.Estimate(at10 + minuteMs); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(setup.estimator.StoreFile); err != nil {
		t.Fatal(err)
	}
	setup.source.anchors = append(setup.source.anchors, anchor(event.FiveHour, 12, at10+2*minuteMs))
	if _, err := setup.estimator.Estimate(at10 + minuteMs + 30_000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(setup.estimator.StoreFile); err == nil {
		t.Error("saved again within a minute")
	}
	if _, err := setup.estimator.Estimate(at10 + 2*minuteMs + 1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(setup.estimator.StoreFile); err != nil {
		t.Errorf("not saved after a minute: %v", err)
	}
}

func TestEstimateReportsSourceErrorsAndUsesTheRest(t *testing.T) {
	setup := newEstimator(t)
	broken := &fakeSource{err: errors.New("broken file")}
	setup.estimator.Sources = []AnchorSource{broken, setup.source}
	setup.source.anchors = []Anchor{anchor(event.Week, 50, at10)}
	result, err := setup.estimator.Estimate(at10 + minuteMs)
	if err == nil {
		t.Error("the broken source was not reported")
	}
	if !reflect.DeepEqual(result.Readings, []Reading{exact(event.Week, 50, 0, at10)}) {
		t.Errorf("readings = %s", show(result.Readings))
	}
}

func TestBrokenCalibrationFileStartsOver(t *testing.T) {
	setup := newEstimator(t)
	if err := os.MkdirAll(filepath.Dir(setup.estimator.StoreFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setup.estimator.StoreFile, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	setup.source.anchors = []Anchor{anchor(event.Week, 50, at10)}
	result, err := setup.estimator.Estimate(at10 + minuteMs)
	if err == nil {
		t.Error("the broken file was not reported")
	}
	if len(result.Readings) != 1 {
		t.Errorf("readings = %s", show(result.Readings))
	}
}

func show(readings []Reading) string {
	text := ""
	for _, reading := range readings {
		resets := "none"
		if reading.ResetsAt != nil {
			resets = time.Unix(*reading.ResetsAt, 0).UTC().Format("15:04")
		}
		text += fmt.Sprintf("{%s %.2f%% est=%v resets=%s at=%s} ", reading.Window, reading.UsedPercent, reading.Estimated, resets,
			time.UnixMilli(reading.AtMs).UTC().Format("15:04"))
	}
	return text
}
