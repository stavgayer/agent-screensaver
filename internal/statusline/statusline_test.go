package statusline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		{
			name:  "missing session uses fallback",
			input: []byte(`{"rate_limits":{"five_hour":{"used_percentage":1}}}`),
			want: event.Event{
				V: event.Version, Source: "claude", Session: FallbackSession,
				Kind: event.KindLimits, AtMs: nowMs,
				Limits: []event.LimitReading{{Window: event.FiveHour, UsedPercent: 1}},
			},
			wantOK: true,
		},
		{
			name:  "empty session uses fallback",
			input: []byte(`{"session_id":"","rate_limits":{"five_hour":{"used_percentage":1}}}`),
			want: event.Event{
				V: event.Version, Source: "claude", Session: FallbackSession,
				Kind: event.KindLimits, AtMs: nowMs,
				Limits: []event.LimitReading{{Window: event.FiveHour, UsedPercent: 1}},
			},
			wantOK: true,
		},
		{
			name:  "oversized session uses fallback",
			input: []byte(`{"session_id":"` + strings.Repeat("x", event.MaxIdentifierLength+1) + `","rate_limits":{"five_hour":{"used_percentage":1}}}`),
			want: event.Event{
				V: event.Version, Source: "claude", Session: FallbackSession,
				Kind: event.KindLimits, AtMs: nowMs,
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
			if !ok {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("event = %+v, want %+v", got, tt.want)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if _, err := event.Parse(encoded); err != nil {
				t.Errorf("event does not round-trip through event.Parse: %v", err)
			}
		})
	}
}

func float64Pointer(value float64) *float64 { return &value }

func TestInfoEvent(t *testing.T) {
	const nowMs = int64(1_738_400_000_000)
	const transcript = "/Users/dev/.claude/projects/-Users-dev-work-my-project/abc123.jsonl"
	tests := []struct {
		name   string
		input  []byte
		want   event.Event
		wantOK bool
	}{
		{
			name:  "name, model and context",
			input: readFixture(t, "session.json"),
			want: event.Event{V: event.Version, Source: "claude", Session: "abc123", Kind: event.KindInfo, AtMs: nowMs,
				Project: "my-project", Name: "Login fix", Model: "Opus 5.5", ContextPercent: float64Pointer(42),
				ModelID: "claude-opus-5-5[1m]", ContextWindow: 1_000_000,
				Cwd: "/Users/dev/work/my-project", Transcript: transcript},
			wantOK: true,
		},
		{
			name:  "context unknown before the first message",
			input: []byte(`{"session_id":"s1","model":{"id":"claude-opus-5-5","display_name":"Opus 5.5"},"context_window":{"used_percentage":null}}`),
			want: event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindInfo, AtMs: nowMs,
				Model: "Opus 5.5", ModelID: "claude-opus-5-5"},
			wantOK: true,
		},
		{
			name:  "model id when there is no display name",
			input: []byte(`{"session_id":"s1","model":{"id":"claude-sonnet-4-5-20250929"}}`),
			want: event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindInfo, AtMs: nowMs,
				Model: "Sonnet 4.5", ModelID: "claude-sonnet-4-5-20250929"},
			wantOK: true,
		},
		{
			name:  "context kept inside 0 to 100",
			input: []byte(`{"session_id":"s1","context_window":{"used_percentage":104}}`),
			want: event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindInfo, AtMs: nowMs,
				ContextPercent: float64Pointer(100)},
			wantOK: true,
		},
		{
			name:  "name is masked and shortened",
			input: []byte(`{"session_id":"s1","session_name":"` + strings.Repeat("word ", 20) + `"}`),
			want: event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindInfo, AtMs: nowMs,
				Name: strings.TrimSpace(strings.Repeat("word ", 7)) + " word…"},
			wantOK: true,
		},
		{name: "no session id", input: []byte(`{"session_name":"x","model":{"id":"m"}}`)},
		{name: "no details", input: []byte(`{"session_id":"s1","cwd":"/a"}`)},
		{name: "not json", input: []byte(`nope`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := InfoEvent(tt.input, nowMs)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (event %+v)", ok, tt.wantOK, got)
			}
			if ok && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("event:\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}
