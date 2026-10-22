package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/limits"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
)

func newTestDaemon(t *testing.T) (*Daemon, paths.Paths) {
	t.Helper()
	p := paths.ForHome(t.TempDir())
	d := New(p)
	d.Logf = t.Logf
	return d, p
}

func spoolEvent(t *testing.T, p paths.Paths, source, session string, kind event.Kind, atMs int64) {
	t.Helper()
	e := event.Event{V: event.Version, Source: source, Session: session, Kind: kind, AtMs: atMs}
	if err := spool.Write(p.SpoolDir, e); err != nil {
		t.Fatal(err)
	}
}

func readStateFile(t *testing.T, p paths.Paths) statefile.StateFile {
	t.Helper()
	data, err := os.ReadFile(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	var file statefile.StateFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestShouldPlaySound(t *testing.T) {
	for combo := range 16 {
		prev, now := combo&8 != 0, combo&4 != 0
		saver, enabled := combo&2 != 0, combo&1 != 0
		want := !prev && now && saver && enabled
		if got := ShouldPlaySound(prev, now, saver, enabled); got != want {
			t.Errorf("ShouldPlaySound(%v, %v, %v, %v) = %v, want %v", prev, now, saver, enabled, got, want)
		}
	}
}

func TestStepDrainsSpoolIntoStateFile(t *testing.T) {
	d, p := newTestDaemon(t)
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 1_000)

	result := d.Step(2_000, false)

	if !result.StateWritten {
		t.Fatal("state file not written")
	}
	file := readStateFile(t, p)
	if len(file.Cards) != 1 || file.Cards[0].Session != "s1" || file.Cards[0].Kind != state.KindRunning {
		t.Fatalf("cards = %+v", file.Cards)
	}
	if file.GeneratedAtMs != 2_000 {
		t.Errorf("generated_at_ms = %d, want 2000", file.GeneratedAtMs)
	}
}

func TestStateRewrittenEvery30s(t *testing.T) {
	d, _ := newTestDaemon(t)
	tests := []struct {
		nowMs int64
		want  bool
	}{
		{0, true}, // the first step always writes
		{1_000, false},
		{29_999, false},
		{30_000, true},
		{31_000, false},
		{60_000, true},
	}
	for _, tt := range tests {
		if got := d.Step(tt.nowMs, false).StateWritten; got != tt.want {
			t.Errorf("Step(%d).StateWritten = %v, want %v", tt.nowMs, got, tt.want)
		}
	}
}

func TestConfigChangeIsPickedUp(t *testing.T) {
	d, p := newTestDaemon(t)
	d.Step(0, false)
	if readStateFile(t, p).View.Privacy {
		t.Fatal("privacy on before the config exists")
	}
	cfg := config.Default()
	cfg.Privacy = true
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(p.ConfigFile, later, later); err != nil {
		t.Fatal(err)
	}

	result := d.Step(1_000, false)

	if !result.StateWritten || !readStateFile(t, p).View.Privacy {
		t.Errorf("written = %v, privacy = %v; want both true", result.StateWritten, readStateFile(t, p).View.Privacy)
	}
}

func TestFinishedCardGoneAfterTTL(t *testing.T) {
	d, p := newTestDaemon(t)
	spoolEvent(t, p, "claude", "s1", event.KindFinished, 0)
	d.Step(1_000, false)
	if len(readStateFile(t, p).Cards) != 1 {
		t.Fatal("finished card missing before the TTL")
	}

	result := d.Step(state.FinishedTTLMs+1_000, false)

	if !result.StateWritten || len(readStateFile(t, p).Cards) != 0 {
		t.Errorf("written = %v, cards = %+v; want the card gone", result.StateWritten, readStateFile(t, p).Cards)
	}
}

func TestSetLimitsAppliedOnNextStep(t *testing.T) {
	d, p := newTestDaemon(t)
	d.Step(0, false)

	d.SetLimits("codex", []event.LimitReading{{Window: event.FiveHour, UsedPercent: 42}}, 500)
	result := d.Step(1_000, false)

	if !result.StateWritten {
		t.Fatal("limits did not trigger a write")
	}
	got := readStateFile(t, p).Limits
	if len(got) != 1 || got[0].Source != "codex" || got[0].UsedPercent != 42 || got[0].UpdatedAtMs != 500 {
		t.Errorf("limits = %+v", got)
	}
}

func TestCodexDueAfterCodexFinished(t *testing.T) {
	d, p := newTestDaemon(t)
	d.SetSources("codex")
	tests := []struct {
		name   string
		before func()
		nowMs  int64
		saver  bool
		want   []string
	}{
		{"never read, saver off", func() {}, 0, false, nil},
		{"never read, saver on", func() {}, 1_000, true, []string{"codex"}},
		{"just read", func() {}, 2_000, true, nil},
		{"another tool finished", func() { spoolEvent(t, p, "claude", "s1", event.KindFinished, 2_500) }, 3_000, true, nil},
		{"codex finished, saver off", func() { spoolEvent(t, p, "codex", "s2", event.KindFinished, 3_500) }, 4_000, false, nil},
		{"codex finished, saver on", func() {}, 5_000, true, []string{"codex"}},
		{"read again after finish", func() {}, 6_000, true, nil},
		{"old reading", func() {}, 6_000 + 5*60_000, true, []string{"codex"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.before()
			if got := d.Step(tt.nowMs, tt.saver).PollSources; !slices.Equal(got, tt.want) {
				t.Errorf("PollSources = %v, want %v", got, tt.want)
			}
		})
	}
}

type fakeWatcher struct{ active atomic.Bool }

func (w *fakeWatcher) Active() bool { return w.active.Load() }

type fakeSound struct {
	mu    sync.Mutex
	files []string
}

func (s *fakeSound) Play(file string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = append(s.files, file)
	return nil
}

func (s *fakeSound) played() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.files)
}

type fakeProvider struct{ reads atomic.Int32 }

func (f *fakeProvider) Source() string { return "codex" }

func (f *fakeProvider) Read(context.Context) ([]event.LimitReading, error) {
	f.reads.Add(1)
	return []event.LimitReading{{Window: event.Week, UsedPercent: 7}}, nil
}

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRunPlaysSoundOnceOnNewApproval(t *testing.T) {
	previous := tickInterval
	tickInterval = 5 * time.Millisecond
	t.Cleanup(func() { tickInterval = previous })
	d, p := newTestDaemon(t)
	watcher, sound, provider := &fakeWatcher{}, &fakeSound{}, &fakeProvider{}
	watcher.active.Store(true)
	spoolEvent(t, p, "claude", "s1", event.KindApproval, time.Now().UnixMilli())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, d, watcher, sound, []limits.Provider{provider}) }()

	eventually(t, "the sound", func() bool { return len(sound.played()) > 0 })
	eventually(t, "the limits", func() bool { return len(readStateFile(t, p).Limits) == 1 })
	time.Sleep(10 * tickInterval) // many more ticks must not repeat the sound
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := sound.played(); len(got) != 1 || got[0] != config.Default().SoundFile {
		t.Errorf("sound plays = %v, want one of %s", got, config.Default().SoundFile)
	}
	if got := provider.reads.Load(); got != 1 {
		t.Errorf("provider read %d times, want 1", got)
	}
}

func TestFinishReadLogsFailureAndEmptyAnswer(t *testing.T) {
	tests := []struct {
		name string
		read readResult
		want string
	}{
		{"error", readResult{source: "codex", err: errors.New("boom")}, "read codex limits: boom"},
		{"no windows", readResult{source: "codex"}, "read codex limits: no windows in the answer"},
		{"readings are quiet", readResult{source: "codex", readings: []event.LimitReading{{Window: event.Plan, UsedPercent: 1}}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := newTestDaemon(t)
			var logs []string
			d.Logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
			loop := newLoop(context.Background(), d, &fakeWatcher{}, &fakeSound{}, nil)
			loop.finishRead(tt.read)
			if tt.want == "" && len(logs) != 0 || tt.want != "" && !slices.Contains(logs, tt.want) {
				t.Errorf("logs = %q, want %q", logs, tt.want)
			}
		})
	}
}
