package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
)

var binaryPath string

func TestMain(m *testing.M) {
	os.Exit(buildAndRun(m))
}

// buildAndRun builds the binary once so each test runs the real executable.
func buildAndRun(m *testing.M) int {
	dir, err := os.MkdirTemp("", "ags-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	binaryPath = filepath.Join(dir, "agent-screensaver")
	if out, err := exec.Command("go", "build", "-o", binaryPath, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

type result struct {
	stdout   string
	exitCode int
	elapsed  time.Duration
}

func runBinary(t *testing.T, home, stdin string, args ...string) result {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the built binary")
	}
	command := exec.Command(binaryPath, args...)
	command.Env = append(os.Environ(), "HOME="+home)
	command.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	start := time.Now()
	err := command.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run binary: %v", err)
	}
	return result{stdout.String(), exitCode, time.Since(start)}
}

func spoolFiles(t *testing.T, home string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(paths.ForHome(home).SpoolDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

const stopPayload = `{"session_id":"s1","hook_event_name":"Stop","cwd":"/work/demo"}`

func TestHookWritesEventFile(t *testing.T) {
	home := t.TempDir()

	got := runBinary(t, home, stopPayload, "hook", "--tool", "claude")

	if got.exitCode != 0 || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
	files := spoolFiles(t, home)
	if len(files) != 1 {
		t.Fatalf("got %d spool files, want 1", len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var written struct{ Source, Session, Kind string }
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if written.Source != "claude" || written.Session != "s1" || written.Kind != "finished" {
		t.Errorf("event = %+v", written)
	}
}

func TestHookGarbageStdinExitsZeroFast(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
	}{
		{"garbage", "not json at all {{{"},
		{"empty", ""},
		{"5 MiB", strings.Repeat("x", 5<<20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runBinary(t, t.TempDir(), tt.stdin, "hook", "--tool", "claude")

			if got.exitCode != 0 || got.stdout != "" {
				t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
			}
			if got.elapsed > 100*time.Millisecond {
				t.Errorf("took %v, want under 100ms", got.elapsed)
			}
		})
	}
}

func TestHookUnknownToolExitsZeroAndLogs(t *testing.T) {
	home := t.TempDir()

	got := runBinary(t, home, stopPayload, "hook", "--tool", "nope")

	if got.exitCode != 0 || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
	log, err := os.ReadFile(filepath.Join(paths.ForHome(home).DataDir, "hook-errors.log"))
	if err != nil || !strings.Contains(string(log), `unknown tool "nope"`) {
		t.Errorf("error log = %q, err %v", log, err)
	}
}

func TestHookCreatesDataDir(t *testing.T) {
	home := t.TempDir()

	runBinary(t, home, stopPayload, "hook", "--tool", "claude")

	if info, err := os.Stat(paths.ForHome(home).DataDir); err != nil || !info.IsDir() {
		t.Errorf("data dir missing: %v", err)
	}
}

func TestHookRecordsPayloadsWhenAsked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGS_RECORD_PAYLOADS", "1")

	runBinary(t, home, stopPayload, "hook", "--tool", "claude")

	files, _ := filepath.Glob(filepath.Join(paths.ForHome(home).DataDir, "payloads", "claude-Stop-*.json"))
	if len(files) != 1 {
		t.Fatalf("got %d payload files, want 1", len(files))
	}
}

const limitsPayload = `{"session_id":"s1","rate_limits":{"five_hour":{"used_percentage":42}}}`

func saveConfig(t *testing.T, home, previous string) {
	t.Helper()
	cfg := config.Default()
	cfg.PreviousStatusline = previous
	if err := cfg.Save(paths.ForHome(home).ConfigFile); err != nil {
		t.Fatal(err)
	}
}

func TestStatuslinePassesThroughPrevious(t *testing.T) {
	home := t.TempDir()
	saveConfig(t, home, "echo hi")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "hi\n" {
		t.Errorf("exit %d, stdout %q; want 0 and %q", got.exitCode, got.stdout, "hi\n")
	}
	if len(spoolFiles(t, home)) != 1 {
		t.Errorf("limits event not written")
	}
}

func TestStatuslineTimesOutSlowPrevious(t *testing.T) {
	home := t.TempDir()
	saveConfig(t, home, "sleep 2")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
	if got.elapsed > 500*time.Millisecond {
		t.Errorf("took %v, want under 500ms", got.elapsed)
	}
}

func TestStatuslineDoesNotCallItself(t *testing.T) {
	home := t.TempDir()
	saveConfig(t, home, binaryPath+" statusline")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
	if files := spoolFiles(t, home); len(files) != 1 {
		t.Errorf("got %d spool events, want exactly 1", len(files))
	}
}

func TestStatuslineStopsWhenAlreadyInsideStatusline(t *testing.T) {
	home := t.TempDir()
	saveConfig(t, home, "echo hi")
	t.Setenv("AGS_IN_STATUSLINE", "1")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
}
