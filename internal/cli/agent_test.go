package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/install"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/settings"
)

const agentNow = int64(1_000 * 60_000)

func agentPaths(t *testing.T) paths.Paths {
	t.Helper()
	p := paths.ForHome(t.TempDir())
	for _, id := range []string{"builtin.claude", "builtin.codex", "builtin.cursor"} {
		if err := os.CopyFS(filepath.Join(p.ExtensionsDir, id), os.DirFS(filepath.Join("..", "..", "extensions", id))); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(p.Home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := install.SaveRecord(p.InstallRecord, install.Record{Version: install.RecordVersion, Bin: install.BinPath(p.Home)}); err != nil {
		t.Fatal(err)
	}
	return p
}

func runAgent(t *testing.T, p paths.Paths, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Agent(args, strings.NewReader(stdin), &stdout, &stderr, p, agentNow)
	return code, stdout.String(), stderr.String()
}

func TestAgentCommands(t *testing.T) {
	p := agentPaths(t)
	hookFile := filepath.Join(p.Home, ".cursor", "hooks.json")
	steps := []struct {
		name       string
		stdin      string
		args       []string
		wantCode   int
		wantStdout string // substring
		wantStderr string // substring
	}{
		{"list shows every agent and its state", "", []string{"list"}, 0, "builtin.cursor\tCursor\tbuilt in\tHooks not added\tno events yet", ""},
		{"a tool that is not here", "", []string{"list"}, 0, "builtin.codex\tCodex\tbuilt in\tNot installed", ""},
		{"no answer cancels", "\n", []string{"hooks", "add", "builtin.cursor"}, 1, "Cancelled. Nothing was changed.", ""},
		{"the plan is shown first", "n\n", []string{"hooks", "add", "builtin.cursor"}, 1, "Add hooks for Cursor (built in)", ""},
		{"yes adds", "y\n", []string{"hooks", "add", "builtin.cursor"}, 0, "hooks added: builtin.cursor", ""},
		{"now idle", "", []string{"list"}, 0, "builtin.cursor\tCursor\tbuilt in\tIdle · no events yet\tno events yet", ""},
		{"off", "", []string{"off", "builtin.cursor"}, 0, "builtin.cursor: off", ""},
		{"listed as off", "", []string{"list"}, 0, "builtin.cursor\tCursor\tbuilt in\tOff", ""},
		{"on", "", []string{"on", "builtin.cursor"}, 0, "builtin.cursor: on", ""},
		{"--yes removes without asking", "", []string{"hooks", "remove", "--yes", "builtin.cursor"}, 0, "hooks removed: builtin.cursor", ""},
		{"nothing left to remove", "", []string{"hooks", "remove", "--yes", "builtin.cursor"}, 1, "", "no hooks to remove"},
		{"unknown agent", "", []string{"off", "acme.none"}, 1, "", "not installed"},
		{"usage", "", []string{"hooks", "add"}, 1, "", "usage: agent"},
	}
	for _, step := range steps {
		code, stdout, stderr := runAgent(t, p, step.stdin, step.args...)
		if code != step.wantCode || !strings.Contains(stdout, step.wantStdout) || !strings.Contains(stderr, step.wantStderr) {
			t.Fatalf("%s: code %d, stdout %q, stderr %q", step.name, code, stdout, stderr)
		}
	}
	if data, err := os.ReadFile(hookFile); err == nil && strings.Contains(string(data), "agent-screensaver") {
		t.Errorf("hooks left after remove:\n%s", data)
	}
}

func TestStateText(t *testing.T) {
	at := func(minutesAgo int64) *int64 {
		value := agentNow - minutesAgo*60_000
		return &value
	}
	tests := []struct {
		name       string
		connection settings.Connection
		want       string
	}{
		{"active", settings.Connection{State: settings.StateActive, LastEventMs: at(4)}, "Active · 4m ago"},
		{"idle for hours", settings.Connection{State: settings.StateIdle, LastEventMs: at(180)}, "Idle · 3h"},
		{"idle with no events", settings.Connection{State: settings.StateIdle}, "Idle · no events yet"},
		{"no hooks", settings.Connection{State: settings.StateNoHooks}, "Hooks not added"},
		{"not installed", settings.Connection{State: settings.StateNotInstalled}, "Not installed"},
		{"off", settings.Connection{State: settings.StateOff, LastEventMs: at(1)}, "Off"},
		{"changed outside", settings.Connection{State: settings.StateIdle, HooksChanged: true}, "Idle · no events yet · Hooks were changed outside agent-screensaver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StateText(tt.connection, agentNow); got != tt.want {
				t.Fatalf("StateText = %q, want %q", got, tt.want)
			}
		})
	}
}
