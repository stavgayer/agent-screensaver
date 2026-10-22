package limits

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"agent-screensaver/internal/event"
)

func int64Pointer(value int64) *int64 { return &value }

func TestParseCodexRateLimits(t *testing.T) {
	fixture, err := os.ReadFile("testdata/codex_rate_limits_response.json")
	if err != nil {
		t.Fatal(err)
	}
	usageBased, err := os.ReadFile("testdata/codex_0162_usage_based_response.json")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		response string
		want     []event.LimitReading
	}{
		{
			name:     "codex 0.162.0 usage-based answer gives the plan reading",
			response: string(usageBased),
			want:     []event.LimitReading{{Window: event.Plan, UsedPercent: 19, ResetsAt: int64Pointer(1793491200)}},
		},
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
			name:     "usage-based plan gives one plan reading",
			response: `{"rateLimits":{"primary":null,"secondary":null,"individualLimit":{"limit":"8824","used":"1636.33","remainingPercent":81,"resetsAt":1793491200}}}`,
			want:     []event.LimitReading{{Window: event.Plan, UsedPercent: 19, ResetsAt: int64Pointer(1793491200)}},
		},
		{
			name:     "plan falls back to used over limit",
			response: `{"rateLimits":{"individualLimit":{"limit":"200","used":50}}}`,
			want:     []event.LimitReading{{Window: event.Plan, UsedPercent: 25}},
		},
		{
			name:     "plan with zero limit and no percent is skipped",
			response: `{"rateLimits":{"individualLimit":{"limit":"0","used":"5"}}}`,
			want:     nil,
		},
		{
			name:     "both windows and plan give three readings",
			response: `{"rateLimits":{"primary":{"usedPercent":10},"secondary":{"usedPercent":20},"individualLimit":{"remainingPercent":"70"}}}`,
			want: []event.LimitReading{
				{Window: event.FiveHour, UsedPercent: 10},
				{Window: event.Week, UsedPercent: 20},
				{Window: event.Plan, UsedPercent: 30},
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
	const second, minute = int64(1000), int64(60_000)
	now := 100 * minute
	tests := []struct {
		name     string
		last     *int64
		failed   bool
		active   bool
		finished bool
		want     bool
	}{
		{"never read is due", nil, false, true, false, true},
		{"inactive never due", nil, false, false, true, false},
		{"inactive after failure is not due", int64Pointer(now - 40*second), true, false, false, false},
		{"failed 20 s ago is not due", int64Pointer(now - 20*second), true, true, false, false},
		{"failed 40 s ago is due", int64Pointer(now - 40*second), true, true, false, true},
		{"success 4 min ago is not due", int64Pointer(now - 4*minute), false, true, false, false},
		{"success 6 min ago is due", int64Pointer(now - 6*minute), false, true, false, true},
		{"success 40 s ago is not due", int64Pointer(now - 40*second), false, true, false, false},
		{"due after source finished", int64Pointer(now - minute), false, true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PollDue(tt.last, tt.failed, tt.active, tt.finished, now); got != tt.want {
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
			for _, provider := range All(tt.lookPath, nil) {
				got = append(got, provider.Source())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolDirs(t *testing.T) {
	glob := func(pattern string) ([]string, error) {
		if pattern != "/h/.nvm/versions/node/*/bin" {
			t.Errorf("unexpected pattern %q", pattern)
		}
		return []string{"/h/.nvm/versions/node/v24.14.0/bin", "/h/.nvm/versions/node/v25.5.0/bin"}, nil
	}
	got := ToolDirs("/h", glob)
	if got[0] != "/opt/homebrew/bin" {
		t.Errorf("first dir = %q", got[0])
	}
	nvm := got[len(got)-2:]
	want := []string{"/h/.nvm/versions/node/v25.5.0/bin", "/h/.nvm/versions/node/v24.14.0/bin"}
	if !reflect.DeepEqual(nvm, want) {
		t.Errorf("nvm dirs = %v, want newest first %v", nvm, want)
	}
}

func TestToolDirsSortNvmVersionsNumerically(t *testing.T) {
	glob := func(string) ([]string, error) {
		return []string{
			"/h/.nvm/versions/node/v9.11.2/bin",
			"/h/.nvm/versions/node/v100.0.0/bin",
			"/h/.nvm/versions/node/v25.5.0/bin",
			"/h/.nvm/versions/node/v25.10.1/bin",
			"/h/.nvm/versions/node/system/bin",
		}, nil
	}
	got := ToolDirs("/h", glob)
	want := []string{
		"/h/.nvm/versions/node/v100.0.0/bin",
		"/h/.nvm/versions/node/v25.10.1/bin",
		"/h/.nvm/versions/node/v25.5.0/bin",
		"/h/.nvm/versions/node/v9.11.2/bin",
		"/h/.nvm/versions/node/system/bin",
	}
	if tail := got[len(got)-len(want):]; !reflect.DeepEqual(tail, want) {
		t.Errorf("nvm dirs = %v, want %v", tail, want)
	}
}

func TestCodexEnv(t *testing.T) {
	tests := []struct {
		name string
		base []string
		want []string
	}{
		{"folder goes first, old PATH follows", []string{"A=1", "PATH=/usr/bin:/bin"}, []string{"A=1", "PATH=/n/bin:/usr/bin:/bin"}},
		{"no PATH gives only the folder, no empty entry", []string{"A=1"}, []string{"A=1", "PATH=/n/bin"}},
		{"empty PATH gives only the folder, no empty entry", []string{"PATH="}, []string{"PATH=/n/bin"}},
		{"nil base", nil, []string{"PATH=/n/bin"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := codexEnv(tt.base, "/n/bin"); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithFallbackDirs(t *testing.T) {
	installed := map[string]bool{"codex": false}
	lookPath := func(name string) (string, error) {
		switch name {
		case "codex":
			if installed["codex"] {
				return "/usr/bin/codex", nil
			}
		case "/b/codex", "/c/codex":
			return name, nil
		}
		return "", errors.New("not found")
	}
	dirs := []string{"/a", "/b", "/c"}
	tests := []struct {
		name    string
		onPath  bool
		want    string
		wantErr bool
	}{
		{"PATH wins", true, "/usr/bin/codex", false},
		{"first fallback dir that has it", false, "/b/codex", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installed["codex"] = tt.onPath
			got, err := WithFallbackDirs(lookPath, dirs)("codex")
			if err != nil || got != tt.want {
				t.Errorf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
	if _, err := WithFallbackDirs(lookPath, dirs)("other"); err == nil {
		t.Error("missing tool must fail")
	}
}

// fakeCodex writes a shell script that stands in for the codex binary.
func fakeCodex(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexRead(t *testing.T) {
	ok := `{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":42,"resetsAt":7}}}}`
	tests := []struct {
		name    string
		script  string
		want    []event.LimitReading
		wantErr bool
	}{
		{
			name:   "skips notifications and junk before the answer",
			script: "read line\necho '{\"method\":\"configWarning\",\"id\":2}'\necho 'not json'\necho '" + ok + "'\nsleep 60",
			want:   []event.LimitReading{{Window: event.FiveHour, UsedPercent: 42, ResetsAt: int64Pointer(7)}},
		},
		{
			name:    "error response fails",
			script:  `echo '{"id":2,"error":{"code":-1,"message":"x"}}'; sleep 60`,
			wantErr: true,
		},
		{
			name:    "exit without answer fails",
			script:  "exit 0",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := Codex{Bin: fakeCodex(t, tt.script), Env: os.Environ()}.Read(ctx)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// launchd gives no node on PATH, and codex starts with "env node": the
// child must get its own folder first on PATH.
func TestCodexReadPutsItsFolderFirstOnPath(t *testing.T) {
	script := "case \"$PATH\" in \"$(dirname \"$0\")\":*) p=1;; *) p=2;; esac\n" +
		`echo "{\"id\":2,\"result\":{\"rateLimits\":{\"primary\":{\"usedPercent\":$p}}}}"`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := Codex{Bin: fakeCodex(t, script), Env: os.Environ()}.Read(ctx)
	if err != nil || len(got) != 1 || got[0].UsedPercent != 1 {
		t.Errorf("got %+v, %v; want one reading with 1 (folder first on PATH)", got, err)
	}
}

// The timeout is generous because the first run of a fresh script can be
// slow on macOS; the script has to write its pid before the deadline.
func TestCodexReadTimeoutKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	// The inner sleep is a grandchild: only a group kill removes it.
	script := fakeCodex(t, "sleep 60 &\necho $! > "+pidFile+"\nwait")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	_, err := Codex{Bin: script, Env: os.Environ()}.Read(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Read took %v", elapsed)
	}
	raw, readErr := os.ReadFile(pidFile)
	if readErr != nil {
		t.Fatal(readErr)
	}
	grandchild, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	// Wait briefly: a killed process needs a moment to be reaped by init.
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(grandchild, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(grandchild, 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("grandchild %d still alive: %v", grandchild, err)
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
	ctx, cancel := context.WithTimeout(context.Background(), ReadTimeout)
	defer cancel()
	readings, err := Codex{Bin: bin, Env: os.Environ()}.Read(ctx)
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
