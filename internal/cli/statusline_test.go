package cli

import (
	"testing"
	"time"
)

func TestIsOwnStatusline(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{"plain", "agent-screensaver statusline", true},
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
		{"success", "echo hi", "hi\n", true, 500 * time.Millisecond},
		{"exit failure", "exit 1", "", false, 500 * time.Millisecond},
		{"timeout in pipeline", "sleep 3 | cat", "", false, 500 * time.Millisecond},
		{"timeout with background child", "(sleep 3; echo x) & wait", "", false, 500 * time.Millisecond},
		{"background process holds stdout", "echo hi; sleep 3 &", "hi\n", true, 500 * time.Millisecond},
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
