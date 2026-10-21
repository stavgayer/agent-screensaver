package tray

import (
	"testing"
	"time"
)

// These tables are copies of the cases in web/test/format.test.ts and
// web/test/builtins.test.ts. If a rule changes in one language, change the
// other and its table, so the menu and the screen saver never disagree.

const minuteMs = 60_000

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		ms   int64
		want string
	}{
		{"negative", -1, "<1m"},
		{"zero", 0, "<1m"},
		{"under a minute", 59_999, "<1m"},
		{"three minutes", 3 * minuteMs, "3m"},
		{"floors partial minutes", 24*minuteMs + 59_000, "24m"},
		{"just under an hour", 59 * minuteMs, "59m"},
		{"exactly an hour", 60 * minuteMs, "1h"},
		{"hour and minutes", 72 * minuteMs, "1h 12m"},
		{"whole hours", 120 * minuteMs, "2h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatDuration(tt.ms); got != tt.want {
				t.Errorf("FormatDuration(%d) = %q, want %q", tt.ms, got, tt.want)
			}
		})
	}
}

const nowMs int64 = 1_800_000_000_000

func inMinutes(minutes int64) int64 { return (nowMs + minutes*minuteMs) / 1000 }

func TestFormatResetIn(t *testing.T) {
	tests := []struct {
		name     string
		resetsAt int64
		want     string
	}{
		{"hour and minutes", inMinutes(100), "1h40m"},
		{"minutes only", inMinutes(35), "35m"},
		{"whole hours", inMinutes(120), "2h"},
		{"rounds up so it never shows 0m", nowMs/1000 + 1, "1m"},
		{"rounds a partial minute up", nowMs/1000 + 61, "2m"},
		{"reset in the past says waiting", inMinutes(-5), "reset, waiting for new data"},
		{"reset exactly now says waiting", nowMs / 1000, "reset, waiting for new data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatResetIn(tt.resetsAt, nowMs); got != tt.want {
				t.Errorf("FormatResetIn(%d) = %q, want %q", tt.resetsAt, got, tt.want)
			}
		})
	}
}

func TestFormatResetDate(t *testing.T) {
	tests := []struct {
		name string
		date time.Time
		want string
	}{
		{"two-digit day", time.Date(2026, time.November, 13, 8, 30, 0, 0, time.Local), "resets 13 Nov"},
		{"single-digit day", time.Date(2026, time.November, 3, 8, 30, 0, 0, time.Local), "resets 3 Nov"},
		{"december", time.Date(2026, time.December, 25, 23, 59, 0, 0, time.Local), "resets 25 Dec"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatResetDate(tt.date.Unix()); got != tt.want {
				t.Errorf("FormatResetDate = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatResetLabel(t *testing.T) {
	const dayMs = 24 * 60 * minuteMs
	far := (nowMs + 13*dayMs) / 1000
	tests := []struct {
		name     string
		resetsAt int64
		want     string
	}{
		{"near reset is a countdown", inMinutes(100), "1h40m"},
		{"exactly 24 hours is still a countdown", inMinutes(24 * 60), "24h"},
		{"far reset is a date", far, FormatResetDate(far)},
		{"past reset says waiting", inMinutes(-5), "reset, waiting for new data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatResetLabel(tt.resetsAt, nowMs); got != tt.want {
				t.Errorf("FormatResetLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct{ source, want string }{
		{"claude", "Claude"},
		{"codex", "Codex"},
		{"other", "Other"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			if got := DisplayName(tt.source); got != tt.want {
				t.Errorf("DisplayName(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}
