package daemon

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
)

// configVersion gives every saved config its own modification time, so the
// daemon sees each save as a change.
var configVersion int64

// saveDemoConfig writes cfg the way the CLI does, with a new modification time.
func saveDemoConfig(t *testing.T, p paths.Paths, cfg config.Config) {
	t.Helper()
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	configVersion++
	stamp := time.Now().Add(time.Duration(configVersion) * time.Minute)
	if err := os.Chtimes(p.ConfigFile, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func armDemo(t *testing.T, p paths.Paths, seconds int, armedMs int64, privacy bool) {
	t.Helper()
	cfg := config.Default()
	cfg.DemoSeconds, cfg.DemoArmedMs, cfg.Privacy = seconds, armedMs, privacy
	saveDemoConfig(t, p, cfg)
}

func showsDemo(file statefile.StateFile) bool {
	return slices.ContainsFunc(file.Cards, func(card state.Card) bool {
		return strings.HasPrefix(card.Session, "demo-")
	})
}

func spoolTitled(t *testing.T, p paths.Paths, session, title string, kind event.Kind, atMs int64) {
	t.Helper()
	e := event.Event{V: event.Version, Source: "claude", Session: session, Kind: kind, AtMs: atMs, Title: title}
	if err := spool.Write(p.SpoolDir, e); err != nil {
		t.Fatal(err)
	}
}

func TestDemoStartsWhenSaverShows(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 60, 1_000, false)
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 500)

	d.Step(2_000, false)
	if file := readStateFile(t, p); showsDemo(file) || len(file.Cards) != 1 || file.Cards[0].Session != "s1" {
		t.Fatalf("saver off: cards = %+v, want the real card", file.Cards)
	}

	d.Step(3_000, true)
	if file := readStateFile(t, p); !showsDemo(file) || len(file.Cards) != 3 || len(file.Limits) != 4 {
		t.Fatalf("saver on: cards = %+v, limits = %+v, want the demo scene", file.Cards, file.Limits)
	}
	if !d.Step(3_500, true).StateWritten {
		t.Error("a playing demo must be written on every tick")
	}
}

func TestDemoEndsAfterItsSeconds(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 10, 1, false)
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 0)
	d.Step(1_000, true)

	d.Step(10_999, true)
	if !showsDemo(readStateFile(t, p)) {
		t.Fatal("demo ended before its 10 seconds")
	}
	result := d.Step(11_000, true)
	if file := readStateFile(t, p); !result.StateWritten || showsDemo(file) || len(file.Cards) != 1 {
		t.Fatalf("written = %v, cards = %+v, want the real card after the demo", result.StateWritten, file.Cards)
	}

	d.Step(12_000, true)
	if showsDemo(readStateFile(t, p)) {
		t.Error("the same armed demo played again")
	}
	if got := config.Load(p.ConfigFile); got.DemoArmedMs != 1 || got.DemoSeconds != 10 {
		t.Errorf("config = %+v, the daemon must not change the config", got)
	}
}

func TestArmedDemoExpiresWithoutSaver(t *testing.T) {
	const expiry = 5 * 60_000
	tests := []struct {
		name     string
		showAtMs int64
		want     bool
	}{
		{"saver shows just before 5 minutes", expiry - 1, true},
		{"saver shows at 5 minutes", expiry, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, p := newTestDaemon(t)
			armDemo(t, p, 60, 1, false)
			d.Step(1_000, false)
			d.Step(tt.showAtMs, false)

			d.Step(tt.showAtMs+1, true)

			if got := showsDemo(readStateFile(t, p)); got != tt.want {
				t.Errorf("demo shown = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRealApprovalEndsDemo(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 60, 1, false)
	d.Step(1_000, true)
	if !d.Step(11_000, true).PlaySound {
		t.Fatal("the scripted approval did not play the sound")
	}

	spoolEvent(t, p, "claude", "s1", event.KindApproval, 11_200)
	result := d.Step(11_500, true)

	file := readStateFile(t, p)
	if showsDemo(file) || len(file.Cards) != 1 || file.Cards[0].Kind != state.KindApproval {
		t.Fatalf("cards = %+v, want only the real approval", file.Cards)
	}
	if !result.PlaySound {
		t.Error("a real approval that ends the demo must play the sound")
	}
	if d.Step(12_000, true).PlaySound || showsDemo(readStateFile(t, p)) {
		t.Error("the demo came back or the sound repeated")
	}
}

func TestDemoStopEndsDemoAndNewDemoPlays(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 60, 1, false)
	d.Step(1_000, true)

	saveDemoConfig(t, p, config.Default()) // what `demo stop` leaves
	d.Step(2_000, true)
	if showsDemo(readStateFile(t, p)) {
		t.Fatal("demo still shown after demo stop")
	}

	armDemo(t, p, 60, 3_000, false)
	d.Step(4_000, true)
	if !showsDemo(readStateFile(t, p)) {
		t.Error("a newly armed demo did not play")
	}
}

func TestDemoTextIsNotMaskedInPrivacyMode(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 60, 1, true)

	d.Step(1_000, true)

	file := readStateFile(t, p)
	if !file.View.Privacy {
		t.Fatal("privacy is off")
	}
	if !slices.ContainsFunc(file.Cards, func(card state.Card) bool { return card.Title == "refactor auth middleware" }) {
		t.Errorf("cards = %+v, want the demo titles unmasked", file.Cards)
	}
}

func TestRealCardsMaskedAfterDemo(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 10, 1, true)
	spoolTitled(t, p, "s1", "secret work", event.KindStarted, 0)
	d.Step(1_000, true)

	d.Step(11_000, true)

	file := readStateFile(t, p)
	if showsDemo(file) || len(file.Cards) != 1 || file.Cards[0].Title != "" {
		t.Errorf("cards = %+v, want the real card with its title masked", file.Cards)
	}
}

func TestDemoApprovalPlaysSound(t *testing.T) {
	d, p := newTestDaemon(t)
	armDemo(t, p, 60, 1, false)
	var soundsAt []int64
	for elapsed := int64(0); elapsed < 30_000; elapsed += 500 {
		if d.Step(1_000+elapsed, true).PlaySound {
			soundsAt = append(soundsAt, elapsed)
		}
	}
	if !slices.Equal(soundsAt, []int64{10_000}) {
		t.Errorf("sound at %v ms into the demo, want once at 10000", soundsAt)
	}
}
