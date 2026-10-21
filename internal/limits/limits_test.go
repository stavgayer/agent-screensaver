package limits

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"testing"

	"agent-screensaver/internal/event"
)

func int64Pointer(value int64) *int64 { return &value }

func TestParseCodexRateLimits(t *testing.T) {
	fixture, err := os.ReadFile("testdata/codex_rate_limits_response.json")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		response string
		want     []event.LimitReading
	}{
		{
			name:     "primary is 5h and secondary is week",
			response: string(fixture),
			want: []event.LimitReading{
				{Window: event.FiveHour, UsedPercent: 42, ResetsAt: int64Pointer(1793491200)},
				{Window: event.Week, UsedPercent: 17, ResetsAt: int64Pointer(1793836800)},
			},
		},
		{
			name:     "missing windows give empty",
			response: `{"rateLimits":{"primary":null,"secondary":null}}`,
			want:     nil,
		},
		{
			name:     "window without percent is skipped",
			response: `{"rateLimits":{"primary":{"resetsAt":5},"secondary":{"usedPercent":3}}}`,
			want:     []event.LimitReading{{Window: event.Week, UsedPercent: 3}},
		},
		{
			name:     "milliseconds are converted to seconds",
			response: `{"rateLimits":{"primary":{"usedPercent":1,"resetsAt":1793491200000}}}`,
			want:     []event.LimitReading{{Window: event.FiveHour, UsedPercent: 1, ResetsAt: int64Pointer(1793491200)}},
		},
		{
			name:     "percent is clamped to 0..100",
			response: `{"rateLimits":{"primary":{"usedPercent":140},"secondary":{"usedPercent":-5}}}`,
			want: []event.LimitReading{
				{Window: event.FiveHour, UsedPercent: 100},
				{Window: event.Week, UsedPercent: 0},
			},
		},
		{
			name:     "invalid json gives empty",
			response: `not json`,
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCodexRateLimits([]byte(tt.response))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPollDue(t *testing.T) {
	const minute = int64(60_000)
	now := 100 * minute
	tests := []struct {
		name     string
		last     *int64
		active   bool
		finished bool
		want     bool
	}{
		{"never polled and active", nil, true, false, true},
		{"inactive never due", nil, false, true, false},
		{"due after 5 min", int64Pointer(now - 5*minute), true, false, true},
		{"not due at 4 min", int64Pointer(now - 4*minute), true, false, false},
		{"due after source finished", int64Pointer(now - minute), true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PollDue(tt.last, tt.active, tt.finished, now); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAll(t *testing.T) {
	found := func(name string) (string, error) { return "/bin/" + name, nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }
	tests := []struct {
		name     string
		lookPath func(string) (string, error)
		want     []string
	}{
		{"codex present gives one provider", found, []string{"codex"}},
		{"codex absent gives none", missing, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, provider := range All(tt.lookPath) {
				got = append(got, provider.Source())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodexReadLive(t *testing.T) {
	if testing.Short() {
		t.Skip("live test skipped under -short")
	}
	bin, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex is not on PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), liveTimeout)
	defer cancel()
	readings, err := Codex{Bin: bin}.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, reading := range readings {
		resets := "none"
		if reading.ResetsAt != nil {
			resets = "unix " + strconv.FormatInt(*reading.ResetsAt, 10)
		}
		t.Logf("window=%s used=%.1f%% resets=%s", reading.Window, reading.UsedPercent, resets)
	}
	t.Logf("%d readings", len(readings))
}
