package cli

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
)

func TestRunStatuslineWritesLimitsAndInfo(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		input string
		want  []event.Kind
	}{
		{"limits and details", []string{"--tool", "claude"},
			`{"session_id":"s1","session_name":"Login fix","rate_limits":{"five_hour":{"used_percentage":20}}}`,
			[]event.Kind{event.KindLimits, event.KindInfo}},
		{"details only", []string{"--tool", "claude"}, `{"session_id":"s1","context_window":{"used_percentage":42}}`, []event.Kind{event.KindInfo}},
		{"nothing to say", []string{"--tool", "claude"}, `{"session_id":"s1"}`, nil},
		{"the command of before Task 6.4 has no --tool", nil, `{"session_id":"s1","session_name":"Login fix"}`, []event.Kind{event.KindInfo}},
		{"an agent without a status line", []string{"--tool", "codex"}, `{"session_id":"s1","session_name":"Login fix"}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := paths.ForHome(t.TempDir())
			copyBuiltinAgent(t, p, "builtin.claude")
			copyBuiltinAgent(t, p, "builtin.codex")
			now := func() int64 { return 1_000 }
			if code := RunStatusline(tt.args, strings.NewReader(tt.input), io.Discard, p, now); code != 0 {
				t.Fatalf("code = %d", code)
			}
			var kinds []event.Kind
			for _, written := range spool.Drain(p.SpoolDir) {
				kinds = append(kinds, written.Kind)
			}
			slices.Sort(kinds)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(kinds, want) {
				t.Errorf("kinds = %v, want %v", kinds, want)
			}
		})
	}
}

func TestIsOwnStatusline(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{"plain", "agent-screensaver statusline", true},
		{"with the tool", "/opt/bin/agent-screensaver statusline --tool claude", true},
		{"full path", "/usr/local/bin/agent-screensaver statusline", true},
		{"with args", "/opt/bin/agent-screensaver statusline --x", true},
		{"other binary name", "foo/agent-screensaver-x statusline", false},
		{"empty", "", false},
		{"no statusline word", "agent-screensaver hook --tool claude", false},
		{"someone else", "echo statusline", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOwnStatusline(tt.command); got != tt.want {
				t.Errorf("isOwnStatusline(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestRunPrevious(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
		wantOK  bool
		limit   time.Duration
	}{
		// The limits are loose so a busy machine does not fail them; well
		// under the sleep 3 is what proves the 300 ms timeout works.
		{"success", "echo hi", "hi\n", true, 1500 * time.Millisecond},
		{"exit failure", "exit 1", "", false, 1500 * time.Millisecond},
		{"timeout in pipeline", "sleep 3 | cat", "", false, 1500 * time.Millisecond},
		{"timeout with background child", "(sleep 3; echo x) & wait", "", false, 1500 * time.Millisecond},
		{"background process holds stdout", "echo hi; sleep 3 &", "hi\n", true, 1500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()

			got, ok := RunPrevious(tt.command, []byte("{}"), 300*time.Millisecond)

			if got != tt.want || ok != tt.wantOK {
				t.Errorf("RunPrevious(%q) = %q, %v; want %q, %v", tt.command, got, ok, tt.want, tt.wantOK)
			}
			if elapsed := time.Since(start); elapsed > tt.limit {
				t.Errorf("took %v, want under %v", elapsed, tt.limit)
			}
		})
	}
}
