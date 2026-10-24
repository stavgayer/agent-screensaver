package usage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// derived marks a reading whose reset time was worked out, not read.
func derived(reading Reading) Reading {
	reading.ResetDerived = true
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
			want:    []Reading{exact(event.FiveHour, 10, at10+5*hourMs+10*minuteMs, at10)},
		},
		{
			// 10 points over 1,000 weighted tokens is 0.01 per token; 500
			// tokens after the latest anchor add 5 points.
			name:    "a ratio from two anchors estimates the tokens used since",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at11 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{estimated(event.FiveHour, 25, at10+5*hourMs+10*minuteMs, at11+30*minuteMs)},
		},
		{
			name:    "no tokens since the latest anchor: the anchor stays exact",
			uses:    []use{{at10 + 10*minuteMs, 1000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 20, at10+5*hourMs+10*minuteMs, at11)},
		},
		{
			// 10 -> 11 is too small to trust; 10 -> 20 over all 1,000
			// tokens gives 0.01.
			name: "a small rise keeps the base until the rise can be trusted",
			uses: []use{{at10 + 10*minuteMs, 400}, {at10 + 40*minuteMs, 600}, {at11 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 11, at10+30*minuteMs),
				anchor(event.FiveHour, 20, at11)},
			nowMs: at11 + 30*minuteMs,
			want:  []Reading{estimated(event.FiveHour, 25, at10+5*hourMs+10*minuteMs, at11+30*minuteMs)},
		},
		{
			name:    "the estimate stops at 100",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at11 + 10*minuteMs, 100_000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{estimated(event.FiveHour, 100, at10+5*hourMs+10*minuteMs, at11+30*minuteMs)},
		},
		{
			// The window of the 11:00 reading ends at 12:00; only the 300
			// tokens after 12:00 count, from zero.
			name: "after a known reset the estimate starts again from zero",
			uses: []use{{at10 + 10*minuteMs, 1000}, {at11 + 30*minuteMs, 2000}, {at12 + 10*minuteMs, 300}},
			anchors: []Anchor{withReset(anchor(event.FiveHour, 10, at10), at12),
				withReset(anchor(event.FiveHour, 20, at11), at12)},
			nowMs: at12 + 30*minuteMs,
			want:  []Reading{estimated(event.FiveHour, 3, at12+5*hourMs+10*minuteMs, at12+30*minuteMs)},
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
			want:    []Reading{estimated(event.FiveHour, 2, at10+17*hourMs, at10+13*hourMs)},
		},
		{
			// The window surely closed by 15:15: the old reading is given
			// with that end, so the ring says Reset instead of 20%.
			name:    "no call in a window open now: the old anchor with the end it surely had",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at10 + 6*hourMs, 5000}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at10+15*minuteMs)},
			nowMs:   at10 + 12*hourMs,
			want:    []Reading{exact(event.FiveHour, 20, at10+5*hourMs+15*minuteMs, at10+15*minuteMs)},
		},
		{
			name:    "no ratio and the window surely closed: the end it surely had",
			anchors: []Anchor{anchor(event.FiveHour, 20, at10)},
			nowMs:   at10 + 6*hourMs,
			want:    []Reading{exact(event.FiveHour, 20, at10+5*hourMs, at10)},
		},
		{
			// 10 points over 1,000 tokens is 0.01; then 10 -> 30 over only
			// 100 tokens (use the transcripts do not show) would be 0.2,
			// but a ratio may at most double: 0.02.
			name: "a ratio may change by at most a factor of two per pair",
			uses: []use{{at10 + 10*minuteMs, 1000}, {at10 + 40*minuteMs, 100}, {at11 + 10*minuteMs, 100}},
			anchors: []Anchor{anchor(event.FiveHour, 0, at10), anchor(event.FiveHour, 10, at10+30*minuteMs),
				anchor(event.FiveHour, 30, at11)},
			nowMs: at11 + 30*minuteMs,
			want:  []Reading{estimated(event.FiveHour, 32, at10+5*hourMs+10*minuteMs, at11+30*minuteMs)},
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
			want:  []Reading{estimated(event.FiveHour, 16, at10+5*hourMs+10*minuteMs, at12+30*minuteMs)},
		},
		{
			name:    "anchors far apart with no known end teach nothing",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at13 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 20, at13)},
			nowMs:   at13 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 20, at10+5*hourMs+10*minuteMs, at13)},
		},
		{
			name:    "windows are calibrated apart; the newest anchor of each wins",
			uses:    []use{{at10 + 10*minuteMs, 1000}, {at11 + 10*minuteMs, 500}},
			anchors: []Anchor{anchor(event.Week, 40, at10), anchor(event.FiveHour, 7, at11), anchor(event.Week, 44, at11)},
			nowMs:   at11 + 30*minuteMs,
			want:    []Reading{exact(event.FiveHour, 7, at10+5*hourMs+10*minuteMs, at11), estimated(event.Week, 46, 0, at11+30*minuteMs)},
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

// While the first read of a large history is not done, nothing is given
// (the screen keeps the limits of the last run), and ratios are learned
// once it is done.
func TestEstimateWhileCatchingUp(t *testing.T) {
	setup := newEstimator(t, use{at10 + 10*minuteMs, 1000}, use{at10 + 20*minuteMs, 1000}, use{at11 + 10*minuteMs, 1000})
	setup.estimator.scanBudget = 300 // about one line per call
	setup.source.anchors = []Anchor{anchor(event.FiveHour, 10, at10), anchor(event.FiveHour, 30, at11)}
	nowMs := at11 + 30*minuteMs
	first, err := setup.estimator.Estimate(nowMs)
	if err != nil {
		t.Fatal(err)
	}
	if !first.CatchingUp || first.Readings != nil {
		t.Fatalf("first = %+v, want catching up with no readings", first)
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
	want := []Reading{estimated(event.FiveHour, 40, at10+5*hourMs+10*minuteMs, nowMs)}
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
	want := []Reading{estimated(event.FiveHour, 25, at10+5*hourMs+10*minuteMs, at11+30*minuteMs)}
	if !reflect.DeepEqual(result.Readings, want) {
		t.Errorf("after restart =\n %s\nwant\n %s", show(result.Readings), show(want))
	}
}

func TestWeekEndSurvivesARestart(t *testing.T) {
	const dayMs = 24 * hourMs
	setup := newEstimator(t)
	setup.source.anchors = []Anchor{withReset(anchor(event.Week, 96, at10-16*dayMs), at10-13*dayMs), anchor(event.Week, 40, at10-dayMs)}
	if _, err := setup.estimator.Estimate(at10); err != nil {
		t.Fatal(err)
	}
	if err := setup.estimator.Flush(); err != nil {
		t.Fatal(err)
	}

	restarted := &Estimator{
		Tally:     &Tally{FS: os.DirFS(setup.root), Root: "projects", Weights: DefaultWeights},
		StoreFile: setup.estimator.StoreFile,
	}
	result, err := restarted.Estimate(at10 + hourMs)
	if err != nil {
		t.Fatal(err)
	}
	want := []Reading{derived(exact(event.Week, 40, at10+dayMs, at10-dayMs))}
	if !reflect.DeepEqual(result.Readings, want) {
		t.Errorf("after restart =\n %s\nwant\n %s", show(result.Readings), show(want))
	}
}

func TestLoadedWeekEndFarAheadIsDropped(t *testing.T) {
	setup := newEstimator(t)
	saved := `{"version":1,"windows":{"week":{"latest":{"window":"week","percent":40,"at_ms":` +
		fmt.Sprint(at10) + `},"resets_at":99999999999}}}`
	if err := os.MkdirAll(filepath.Dir(setup.estimator.StoreFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setup.estimator.StoreFile, []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := setup.estimator.Estimate(at10 + minuteMs)
	if err != nil {
		t.Fatal(err)
	}
	want := []Reading{exact(event.Week, 40, 0, at10)}
	if !reflect.DeepEqual(result.Readings, want) {
		t.Errorf("readings = %s, want %s", show(result.Readings), show(want))
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

func TestLoadedCalibrationIsKeptSane(t *testing.T) {
	setup := newEstimator(t)
	saved := `{"version":1,"windows":{"5h":{"latest":{"window":"5h","percent":250,"resets_at":99999999999,"at_ms":` +
		fmt.Sprint(at10) + `},"percent_per_token":-3}}}`
	if err := os.MkdirAll(filepath.Dir(setup.estimator.StoreFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(setup.estimator.StoreFile, []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := setup.estimator.Estimate(at10 + minuteMs)
	if err != nil {
		t.Fatal(err)
	}
	// Percent kept in 0..100, an end far past one window length dropped,
	// a negative ratio forgotten.
	want := []Reading{exact(event.FiveHour, 100, 0, at10)}
	if !reflect.DeepEqual(result.Readings, want) {
		t.Errorf("readings = %s, want %s", show(result.Readings), show(want))
	}
}

func TestFlushSavesAtOnce(t *testing.T) {
	setup := newEstimator(t)
	setup.source.anchors = []Anchor{anchor(event.Week, 50, at10)}
	if _, err := setup.estimator.Estimate(at10 + minuteMs); err != nil {
		t.Fatal(err)
	}
	setup.source.anchors = append(setup.source.anchors, anchor(event.Week, 51, at10+2*minuteMs))
	if _, err := setup.estimator.Estimate(at10 + 3*minuteMs - 59_000); err != nil {
		t.Fatal(err)
	}
	if err := setup.estimator.Flush(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(setup.estimator.StoreFile)
	if err != nil || !strings.Contains(string(data), `"percent":51`) {
		t.Errorf("after Flush the file holds %s (%v), want the newest reading", data, err)
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
			resets = time.Unix(*reading.ResetsAt, 0).UTC().Format("Jan 2 15:04")
		}
		text += fmt.Sprintf("{%s %.2f%% est=%v resets=%s derived=%v at=%s} ", reading.Window, reading.UsedPercent, reading.Estimated, resets, reading.ResetDerived,
			time.UnixMilli(reading.AtMs).UTC().Format("15:04"))
	}
	return text
}

// The reset time is always known when it can be: an exact end wins, else
// the 5h window is read from the calls and the week rolls forward.
func TestEstimateResetTime(t *testing.T) {
	const dayMs = 24 * hourMs
	tests := []struct {
		name    string
		uses    []use
		anchors []Anchor
		nowMs   int64
		window  event.Window
		want    Reading
	}{
		{
			name:    "the first call after a known end opens the next window",
			uses:    []use{{at10 + 2*hourMs, 100}, {at12 + 40*minuteMs, 100}},
			anchors: []Anchor{withReset(anchor(event.FiveHour, 30, at10), at12)},
			nowMs:   at13,
			window:  event.FiveHour,
			want:    exact(event.FiveHour, 30, at12, at10),
		},
		{
			name:    "a call more than five hours after the last opens a new window",
			uses:    []use{{at10 + 10*minuteMs, 100}, {at10 + 5*hourMs + 30*minuteMs, 100}},
			anchors: []Anchor{anchor(event.FiveHour, 0, at10+5*hourMs+40*minuteMs)},
			nowMs:   at10 + 6*hourMs,
			window:  event.FiveHour,
			want:    exact(event.FiveHour, 0, at10+10*hourMs+30*minuteMs, at10+5*hourMs+40*minuteMs),
		},
		{
			name:    "an exact end wins over the one the calls give",
			uses:    []use{{at10 + 10*minuteMs, 100}},
			anchors: []Anchor{withReset(anchor(event.FiveHour, 10, at10), at13)},
			nowMs:   at11,
			window:  event.FiveHour,
			want:    exact(event.FiveHour, 10, at13, at10),
		},
		{
			name:    "a week past its end says the end it had, so the ring says Reset",
			anchors: []Anchor{withReset(anchor(event.Week, 60, at10-4*dayMs), at10-2*dayMs)},
			nowMs:   at10,
			window:  event.Week,
			want:    exact(event.Week, 60, at10-2*dayMs, at10-4*dayMs),
		},
		{
			name:    "the week keeps the exact end while it is ahead",
			anchors: []Anchor{withReset(anchor(event.Week, 60, at10), at10+3*dayMs)},
			nowMs:   at11,
			window:  event.Week,
			want:    exact(event.Week, 60, at10+3*dayMs, at10),
		},
		{
			name:    "a week reading after the old end gets the end rolled forward by a whole week",
			anchors: []Anchor{withReset(anchor(event.Week, 60, at10-4*dayMs), at10-2*dayMs), anchor(event.Week, 5, at10-dayMs)},
			nowMs:   at10,
			window:  event.Week,
			want:    derived(exact(event.Week, 5, at10+5*dayMs, at10-dayMs)),
		},
		{
			// The /usage cache in ~/.claude.json can be weeks old; its week
			// end still gives the weekly cadence.
			name:    "a week end read longer ago than the horizon still rolls forward to the next one",
			anchors: []Anchor{withReset(anchor(event.Week, 96, at10-16*dayMs), at10-13*dayMs), anchor(event.Week, 40, at10-dayMs)},
			nowMs:   at10,
			window:  event.Week,
			want:    derived(exact(event.Week, 40, at10+dayMs, at10-dayMs)),
		},
		{
			name:    "the newest week end seen wins over an older one",
			anchors: []Anchor{withReset(anchor(event.Week, 96, at10-30*dayMs), at10-27*dayMs-hourMs), withReset(anchor(event.Week, 50, at10-16*dayMs), at10-13*dayMs), anchor(event.Week, 40, at10-dayMs)},
			nowMs:   at10,
			window:  event.Week,
			want:    derived(exact(event.Week, 40, at10+dayMs, at10-dayMs)),
		},
		{
			name:    "a week end never seen stays unknown",
			anchors: []Anchor{anchor(event.Week, 60, at10)},
			nowMs:   at11,
			window:  event.Week,
			want:    exact(event.Week, 60, 0, at10),
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
			for _, got := range result.Readings {
				if got.Window == tt.window {
					if !reflect.DeepEqual(got, tt.want) {
						t.Errorf("reading = %s, want %s", show([]Reading{got}), show([]Reading{tt.want}))
					}
					return
				}
			}
			t.Errorf("no %s reading in %s", tt.window, show(result.Readings))
		})
	}
}
