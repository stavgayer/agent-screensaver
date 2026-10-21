package statusline

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"agent-screensaver/internal/event"
)

func int64Pointer(value int64) *int64 { return &value }

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestLimitsEvent(t *testing.T) {
	const nowMs = int64(1_738_400_000_000)

	tests := []struct {
		name   string
		input  []byte
		want   event.Event
		wantOK bool
	}{
		{
			name:  "both windows",
			input: readFixture(t, "full.json"),
			want: event.Event{
				V: event.Version, Source: "claude", Session: "abc123",
				Kind: event.KindLimits, AtMs: nowMs, Project: "my-project",
				Limits: []event.LimitReading{
					{Window: event.FiveHour, UsedPercent: 62.0, ResetsAt: int64Pointer(1738425600)},
					{Window: event.Week, UsedPercent: 38.0, ResetsAt: int64Pointer(1738857600)},
				},
			},
			wantOK: true,
		},
		{
			name:  "one window",
			input: readFixture(t, "five_hour_only.json"),
			want: event.Event{
				V: event.Version, Source: "claude", Session: "abc123",
				Kind: event.KindLimits, AtMs: nowMs, Project: "my-project",
				Limits: []event.LimitReading{
					{Window: event.FiveHour, UsedPercent: 23.5, ResetsAt: int64Pointer(1738425600)},
				},
			},
			wantOK: true,
		},
		{
			name:  "no rate limits",
			input: readFixture(t, "no_rate_limits.json"),
		},
		{
			name: "percent clamped to 0..100",
			input: []byte(`{"session_id":"s","rate_limits":{
				"five_hour":{"used_percentage":130},
				"seven_day":{"used_percentage":-5}}}`),
			want: event.Event{
				V: event.Version, Source: "claude", Session: "s",
				Kind: event.KindLimits, AtMs: nowMs,
				Limits: []event.LimitReading{
					{Window: event.FiveHour, UsedPercent: 100},
					{Window: event.Week, UsedPercent: 0},
				},
			},
			wantOK: true,
		},
		{
			name:  "window without percentage is skipped",
			input: []byte(`{"session_id":"s","rate_limits":{"five_hour":{"resets_at":1},"seven_day":{"used_percentage":10}}}`),
			want: event.Event{
				V: event.Version, Source: "claude", Session: "s",
				Kind: event.KindLimits, AtMs: nowMs,
				Limits: []event.LimitReading{{Window: event.Week, UsedPercent: 10}},
			},
			wantOK: true,
		},
		{
			name:  "cwd is the project fallback",
			input: []byte(`{"cwd":"/a/b/fallback","session_id":"s","rate_limits":{"five_hour":{"used_percentage":1}}}`),
			want: event.Event{
				V: event.Version, Source: "claude", Session: "s",
				Kind: event.KindLimits, AtMs: nowMs, Project: "fallback",
				Limits: []event.LimitReading{{Window: event.FiveHour, UsedPercent: 1}},
			},
			wantOK: true,
		},
		{name: "invalid json", input: []byte(`{not json`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LimitsEvent(tt.input, nowMs)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("event = %+v, want %+v", got, tt.want)
			}
		})
	}
}
