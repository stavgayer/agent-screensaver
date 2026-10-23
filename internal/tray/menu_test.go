package tray

import (
	"reflect"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
)

func approvalCard(source, project string, sinceMs int64) state.Card {
	return state.Card{Source: source, Session: source + project, Kind: state.KindApproval,
		Project: project, ApprovalSinceMs: &sinceMs, UpdatedAtMs: sinceMs}
}

func runningCard(source, session string) state.Card {
	return state.Card{Source: source, Session: session, Kind: state.KindRunning, UpdatedAtMs: nowMs}
}

func limit(source string, window event.Window, percent float64, resetsAtS int64) state.Limit {
	reading := state.Limit{Source: source, Window: window, UsedPercent: percent, UpdatedAtMs: nowMs}
	if resetsAtS != 0 {
		reading.ResetsAt = &resetsAtS
	}
	return reading
}

func plain(label string) Item { return Item{Label: label} }

var separator = Item{}

// tail is the part of the menu that never changes, for the default config
// (sound on, privacy off).
func tail(sound, privacy bool) []Item {
	return []Item{
		separator,
		{ID: "preview", Label: "Preview screen saver", Enabled: true},
		{ID: "settings", Label: "Settings…", Enabled: true},
		separator,
		{ID: "sound", Label: "Sound", Enabled: true, Checked: sound},
		{ID: "privacy", Label: "Privacy mode", Enabled: true, Checked: privacy},
		separator,
		{ID: "quit", Label: "Quit menu bar app", Enabled: true},
	}
}

// live marks a state file as written just now by a running daemon.
func live(f statefile.StateFile) statefile.StateFile {
	f.ServiceOK, f.GeneratedAtMs = true, nowMs
	return f
}

func TestBuildMenu(t *testing.T) {
	soundOn := config.Config{Sound: true}
	tests := []struct {
		name          string
		file          statefile.StateFile
		cfg           config.Config
		wantAttention bool
		wantInfo      []Item
		wantSound     bool
		wantPrivacy   bool
	}{
		{
			name:      "all quiet",
			cfg:       soundOn,
			wantInfo:  []Item{plain("All quiet")},
			wantSound: true,
		},
		{
			name: "one approval two running",
			file: statefile.StateFile{Cards: []state.Card{
				approvalCard("codex", "api-server", nowMs-3*minuteMs),
				runningCard("claude", "a"), runningCard("claude", "b"),
			}},
			cfg:           soundOn,
			wantAttention: true,
			wantInfo: []Item{
				plain("1 waiting for approval · 2 running"),
				plain("Codex · api-server · waiting 3m"),
			},
			wantSound: true,
		},
		{
			name: "only running agents",
			file: statefile.StateFile{Cards: []state.Card{runningCard("claude", "a")}},
			cfg:  soundOn,
			wantInfo: []Item{
				plain("1 running"),
			},
			wantSound: true,
		},
		{
			name: "limits lines with reset countdown",
			file: statefile.StateFile{Limits: []state.Limit{
				limit("codex", event.Week, 55, 0),
				limit("claude", event.FiveHour, 62.4, inMinutes(100)),
				limit("codex", event.Plan, 20, inMinutes(100)),
			}},
			cfg: soundOn,
			wantInfo: []Item{
				plain("All quiet"),
				plain("Claude 62% · resets in 1h40m"),
				plain("Codex week 55%"),
				plain("Codex plan 20% · resets in 1h40m"),
			},
			wantSound: true,
		},
		{
			name: "an estimated limit says so",
			file: statefile.StateFile{Limits: []state.Limit{
				estimatedLimit(limit("claude", event.FiveHour, 47.6, inMinutes(100))),
				estimatedLimit(limit("claude", event.Week, 61, 0)),
			}},
			cfg: soundOn,
			wantInfo: []Item{
				plain("All quiet"),
				plain("Claude 48% · resets in 1h40m · est."),
				plain("Claude week 61% · est."),
			},
			wantSound: true,
		},
		{
			name: "plan reset far away shows the date",
			file: statefile.StateFile{Limits: []state.Limit{
				limit("codex", event.Plan, 20, (nowMs+13*24*60*minuteMs)/1000),
			}},
			cfg: soundOn,
			wantInfo: []Item{
				plain("All quiet"),
				plain("Codex plan 20% · " + FormatResetDate((nowMs+13*24*60*minuteMs)/1000)),
			},
			wantSound: true,
		},
		{
			name: "old limit reading says how old",
			file: statefile.StateFile{Limits: []state.Limit{
				{Source: "claude", Window: event.Week, UsedPercent: 40, UpdatedAtMs: nowMs - 20*minuteMs},
				{Source: "codex", Window: event.Week, UsedPercent: 40, UpdatedAtMs: nowMs - 15*minuteMs},
			}},
			cfg: soundOn,
			wantInfo: []Item{
				plain("All quiet"),
				plain("Claude week 40% · updated 20m ago"),
				plain("Codex week 40%"),
			},
			wantSound: true,
		},
		{
			name: "reset in past says waiting",
			file: statefile.StateFile{Limits: []state.Limit{
				limit("claude", event.FiveHour, 90, inMinutes(-5)),
			}},
			cfg: soundOn,
			wantInfo: []Item{
				plain("All quiet"),
				plain("Claude 90% · reset, waiting for new data"),
			},
			wantSound: true,
		},
		{
			name: "privacy hides project names",
			file: statefile.StateFile{Cards: []state.Card{
				approvalCard("codex", "api-server", nowMs-3*minuteMs),
			}},
			cfg:           config.Config{Sound: true, Privacy: true},
			wantAttention: true,
			wantInfo: []Item{
				plain("1 waiting for approval"),
				plain("Codex · waiting 3m"),
			},
			wantSound:   true,
			wantPrivacy: true,
		},
		{
			name: "approval without a project",
			file: statefile.StateFile{Cards: []state.Card{approvalCard("claude", "", nowMs)}},
			cfg:  soundOn,
			wantInfo: []Item{
				plain("1 waiting for approval"),
				plain("Claude · waiting <1m"),
			},
			wantAttention: true,
			wantSound:     true,
		},
		{
			name:        "checked sound and privacy reflect config",
			cfg:         config.Config{Sound: false, Privacy: true},
			wantInfo:    []Item{plain("All quiet")},
			wantSound:   false,
			wantPrivacy: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMenu(live(tt.file), tt.cfg, nowMs)
			want := Model{Attention: tt.wantAttention, Items: append(tt.wantInfo, tail(tt.wantSound, tt.wantPrivacy)...)}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("BuildMenu:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestBuildMenuServiceDown(t *testing.T) {
	cards := []state.Card{approvalCard("codex", "api-server", nowMs-minuteMs)}
	limits := []state.Limit{limit("claude", event.FiveHour, 62, inMinutes(100))}
	tests := []struct {
		name string
		file statefile.StateFile
	}{
		{"file missing", statefile.StateFile{}},
		{"service_ok false", statefile.StateFile{ServiceOK: false, GeneratedAtMs: nowMs, Cards: cards, Limits: limits}},
		{"generated more than 120 s ago", statefile.StateFile{ServiceOK: true, GeneratedAtMs: nowMs - 120_001, Cards: cards, Limits: limits}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMenu(tt.file, config.Config{Sound: true}, nowMs)
			want := Model{Items: append([]Item{plain("Service not running")}, tail(true, false)...)}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("BuildMenu:\n got %+v\nwant %+v", got, want)
			}
		})
	}
	t.Run("exactly 120 s old is still alive", func(t *testing.T) {
		file := statefile.StateFile{ServiceOK: true, GeneratedAtMs: nowMs - 120_000}
		if got := BuildMenu(file, config.Config{}, nowMs); got.Items[0].Label != "All quiet" {
			t.Errorf("first line = %q", got.Items[0].Label)
		}
	})
}

func estimatedLimit(reading state.Limit) state.Limit {
	reading.Estimated = true
	return reading
}
