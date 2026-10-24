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

	"agent-screensaver/internal/cli"
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
	// Start it once: macOS checks a new program on its first start, which
	// can take most of a second, and no test should pay for that.
	_ = exec.Command(binaryPath, "version").Run()
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

// homeWithAgents is a temp home whose extensions folder holds the built-in
// agent extensions, as an installed Mac has them.
func homeWithAgents(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, id := range []string{"builtin.claude", "builtin.codex"} {
		for _, name := range []string{"extension.json", "mark.svg"} {
			data, err := os.ReadFile(filepath.Join("..", "..", "extensions", id, name))
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(paths.ForHome(home).ExtensionsDir, id, name)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return home
}

const stopPayload = `{"session_id":"s1","hook_event_name":"Stop","cwd":"/work/demo"}`

func TestHookWritesEventFile(t *testing.T) {
	home := homeWithAgents(t)

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
			got := runBinary(t, homeWithAgents(t), tt.stdin, "hook", "--tool", "claude")

			if got.exitCode != 0 || got.stdout != "" {
				t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
			}
			// A loose guard against a hook that blocks; the real latency is
			// measured by BenchmarkHookWithManyExtensions in internal/cli.
			if got.elapsed > time.Second {
				t.Errorf("took %v, want under 1 s", got.elapsed)
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
	home := homeWithAgents(t)

	runBinary(t, home, stopPayload, "hook", "--tool", "claude")

	if info, err := os.Stat(paths.ForHome(home).DataDir); err != nil || !info.IsDir() {
		t.Errorf("data dir missing: %v", err)
	}
}

func TestHookRecordsPayloadsWhenAsked(t *testing.T) {
	home := homeWithAgents(t)
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
	home := homeWithAgents(t)
	saveConfig(t, home, "echo hi")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "hi\n" {
		t.Errorf("exit %d, stdout %q; want 0 and %q", got.exitCode, got.stdout, "hi\n")
	}
	if len(spoolFiles(t, home)) != 1 {
		t.Errorf("limits event not written")
	}
}

func TestStatuslineWithToolWritesLimits(t *testing.T) {
	home := homeWithAgents(t)

	got := runBinary(t, home, limitsPayload, "statusline", "--tool", "claude")

	if got.exitCode != 0 || len(spoolFiles(t, home)) != 1 {
		t.Errorf("exit %d, spool files %d; want 0 and 1", got.exitCode, len(spoolFiles(t, home)))
	}
}

func TestStatuslineTimesOutSlowPrevious(t *testing.T) {
	home := homeWithAgents(t)
	saveConfig(t, home, "sleep 2")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
	// Well under the previous command's sleep 2, which proves the timeout.
	if got.elapsed > 1500*time.Millisecond {
		t.Errorf("took %v, want under 1.5 s", got.elapsed)
	}
}

func TestStatuslineDoesNotCallItself(t *testing.T) {
	home := homeWithAgents(t)
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
	home := homeWithAgents(t)
	saveConfig(t, home, "echo hi")
	t.Setenv("AGS_IN_STATUSLINE", "1")

	got := runBinary(t, home, limitsPayload, "statusline")

	if got.exitCode != 0 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q; want 0 and empty", got.exitCode, got.stdout)
	}
}

func TestVersionPrintsBuildVersion(t *testing.T) {
	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			got := runBinary(t, t.TempDir(), "", arg)

			if got.exitCode != 0 || got.stdout != "dev\n" {
				t.Errorf("exit %d, stdout %q; want 0 and \"dev\\n\"", got.exitCode, got.stdout)
			}
		})
	}
}

// Every subcommand that run does not dispatch by name is a settings command,
// so none falls through to an unknown-command error.
func TestEverySubcommandIsDispatched(t *testing.T) {
	byName := map[string]bool{"hook": true, "statusline": true, "daemon": true, "install": true,
		"uninstall": true, "tray": true, "settings": true, "agent": true}
	for _, name := range subcommands {
		if !byName[name] && !cli.IsSettingsCommand(name) {
			t.Errorf("subcommand %q is neither dispatched by name nor a settings command", name)
		}
	}
}
