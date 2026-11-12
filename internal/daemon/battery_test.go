package daemon

import (
	"errors"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/power"
)

// fakeBattery answers each read with the next of its readings, then repeats
// the last one. A set err fails every read.
type fakeBattery struct {
	readings []power.Reading
	err      error
	reads    int
}

func (f *fakeBattery) Read() (power.Reading, error) {
	reading := f.readings[min(f.reads, len(f.readings)-1)]
	f.reads++
	return reading, f.err
}

func onBattery(percent int) power.Reading {
	return power.Reading{HasBattery: true, OnBattery: true, Percent: percent, MinutesLeft: 90}
}

func newBatteryDaemon(t *testing.T, battery *fakeBattery) (*Daemon, paths.Paths) {
	t.Helper()
	d, p := newTestDaemon(t)
	d.Battery = battery
	return d, p
}

func TestBatteryOnBatteryWritesPower(t *testing.T) {
	d, p := newBatteryDaemon(t, &fakeBattery{readings: []power.Reading{onBattery(64)}})
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 500)
	d.Step(1_000, false)
	got := readStateFile(t, p).Power
	if got == nil || got.Percent != 64 || got.Warn || got.Working != 1 {
		t.Fatalf("power = %+v, want 64%% with no warning and 1 working", got)
	}
}

func TestBatteryPluggedInWritesNoPower(t *testing.T) {
	plugged := power.Reading{HasBattery: true, Percent: 15, Charging: true}
	d, p := newBatteryDaemon(t, &fakeBattery{readings: []power.Reading{plugged}})
	d.Step(1_000, false)
	if got := readStateFile(t, p).Power; got != nil {
		t.Errorf("power = %+v, want none", got)
	}
}

func TestBatteryReadEvery10s(t *testing.T) {
	battery := &fakeBattery{readings: []power.Reading{onBattery(64)}}
	d, _ := newBatteryDaemon(t, battery)
	for _, nowMs := range []int64{0, 500, 9_500} {
		d.Step(nowMs, false)
	}
	if battery.reads != 1 {
		t.Fatalf("reads = %d after 9.5 s, want 1", battery.reads)
	}
	d.Step(10_000, false)
	if battery.reads != 2 {
		t.Errorf("reads = %d at 10 s, want 2", battery.reads)
	}
}

func TestBatteryErrorLoggedOnce(t *testing.T) {
	battery := &fakeBattery{readings: []power.Reading{onBattery(64)}, err: errors.New("no IOKit")}
	d, p := newBatteryDaemon(t, battery)
	var logged int
	d.Logf = func(string, ...any) { logged++ }
	for _, nowMs := range []int64{0, 10_000, 20_000} {
		d.Step(nowMs, false)
	}
	if battery.reads != 3 || logged != 1 {
		t.Errorf("reads = %d, logs = %d, want 3 reads and 1 log", battery.reads, logged)
	}
	if got := readStateFile(t, p).Power; got != nil {
		t.Errorf("power = %+v after failed reads, want none", got)
	}
}

func TestBatteryWarnPlaysSoundOnce(t *testing.T) {
	battery := &fakeBattery{readings: []power.Reading{onBattery(40), onBattery(18)}}
	d, p := newBatteryDaemon(t, battery)
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 500)
	var soundsAt []int64
	for _, nowMs := range []int64{1_000, 5_000, 11_000, 15_000, 21_000} {
		if d.Step(nowMs, true).PlaySound {
			soundsAt = append(soundsAt, nowMs)
		}
	}
	if len(soundsAt) != 1 || soundsAt[0] != 11_000 {
		t.Errorf("sound at %v, want once at 11000 (the read that went low)", soundsAt)
	}
	if got := readStateFile(t, p).Power; got == nil || !got.Warn {
		t.Errorf("power = %+v, want a warning", got)
	}
}

func TestBatteryWarnAfterApprovalNoSecondSound(t *testing.T) {
	battery := &fakeBattery{readings: []power.Reading{onBattery(40), onBattery(18)}}
	d, p := newBatteryDaemon(t, battery)
	spoolEvent(t, p, "claude", "s1", event.KindApproval, 500)
	var sounds int
	for _, nowMs := range []int64{1_000, 5_000, 11_000, 15_000} {
		if d.Step(nowMs, true).PlaySound {
			sounds++
		}
	}
	if sounds != 1 {
		t.Errorf("sounds = %d, want 1 (the approval; the low battery adds none)", sounds)
	}
}

func TestBatteryWarnEndsWhenWorkEnds(t *testing.T) {
	d, p := newBatteryDaemon(t, &fakeBattery{readings: []power.Reading{onBattery(18)}})
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 500)
	d.Step(1_000, false)
	if got := readStateFile(t, p).Power; got == nil || !got.Warn {
		t.Fatalf("power = %+v, want a warning while the card runs", got)
	}
	spoolEvent(t, p, "claude", "s1", event.KindFinished, 1_500)
	d.Step(2_000, false)
	got := readStateFile(t, p).Power
	if got == nil || got.Warn || !got.Low {
		t.Errorf("power = %+v, want low without a warning once the work ended", got)
	}
}

func TestBatteryPrivacyKeepsPower(t *testing.T) {
	d, p := newBatteryDaemon(t, &fakeBattery{readings: []power.Reading{onBattery(64)}})
	cfg := config.Default()
	cfg.Privacy = true
	saveDemoConfig(t, p, cfg)
	d.Step(1_000, false)
	if got := readStateFile(t, p).Power; got == nil {
		t.Error("power is missing in privacy mode, want it kept")
	}
}

func TestApprovalSoundsDuringBatteryWarning(t *testing.T) {
	d, p := newBatteryDaemon(t, &fakeBattery{readings: []power.Reading{onBattery(18)}})
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 500)
	if !d.Step(1_000, true).PlaySound {
		t.Fatal("the warning did not sound when it started")
	}
	if d.Step(2_000, true).PlaySound {
		t.Fatal("the warning sounded again")
	}
	spoolEvent(t, p, "claude", "s1", event.KindApproval, 2_500)
	if !d.Step(3_000, true).PlaySound {
		t.Error("an approval during a battery warning made no sound")
	}
}
