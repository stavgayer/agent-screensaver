package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type noServices struct{ installed, removed int }

func (n *noServices) Install(string, []string, string) (string, error) { n.installed++; return "", nil }
func (n *noServices) Uninstall(string) error                           { n.removed++; return nil }

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newSetup(t *testing.T, stdin string) (setup, *noServices, *bytes.Buffer, string, string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	artifacts := filepath.Join(root, "artifacts")
	writeTree(t, root, map[string]string{
		"artifacts/runtime/index.html":                         "x",
		"artifacts/extensions/builtin.clock/extension.json":    "{}",
		"artifacts/AgentScreensaver.saver/Contents/Info.plist": "x",
		"agent-screensaver":                                    "binary",
		"home/.claude/projects/.keep":                          "",
		"home/.codex/.keep":                                    "",
	})
	for _, id := range []string{"builtin.claude", "builtin.codex"} {
		for _, name := range []string{"extension.json", "mark.svg"} {
			data, err := os.ReadFile(filepath.Join("..", "..", "extensions", id, name))
			if err != nil {
				t.Fatal(err)
			}
			writeTree(t, artifacts, map[string]string{filepath.Join("extensions", id, name): string(data)})
		}
	}
	services := &noServices{}
	var stdout bytes.Buffer
	env := setup{home: home, executable: filepath.Join(root, "agent-screensaver"), services: services,
		stdin: strings.NewReader(stdin), stdout: &stdout, stderr: &stdout}
	return env, services, &stdout, home, artifacts
}

func TestInstallPromptDecidesWhetherToProceed(t *testing.T) {
	tests := []struct {
		name          string
		stdin         string
		args          []string
		wantExit      int
		wantInstalled int
	}{
		{"declined", "n\n", nil, 1, 0},
		{"empty answer", "\n", nil, 1, 0},
		{"accepted", "y\n", nil, 0, 2},
		{"yes flag skips the prompt", "", []string{"--yes"}, 0, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, services, stdout, home, artifacts := newSetup(t, tt.stdin)

			code := env.install(append([]string{"--artifacts", artifacts}, tt.args...))

			if code != tt.wantExit || services.installed != tt.wantInstalled {
				t.Fatalf("exit %d, installs %d\n%s", code, services.installed, stdout)
			}
			_, err := os.Stat(filepath.Join(home, ".claude", "settings.json"))
			if (err == nil) != (tt.wantInstalled > 0) {
				t.Errorf("settings file exists = %v", err == nil)
			}
			if !strings.Contains(stdout.String(), "Proceed?") && tt.stdin != "" {
				t.Errorf("no prompt in:\n%s", stdout)
			}
		})
	}
}

func TestInstallPrintsRemindersLast(t *testing.T) {
	env, _, stdout, home, artifacts := newSetup(t, "")

	if code := env.install([]string{"--yes", "--artifacts", artifacts}); code != 0 {
		t.Fatalf("exit %d\n%s", code, stdout)
	}

	text := stdout.String()
	for _, want := range []string{filepath.Join(home, ".codex", "hooks.json"), "/hooks", "Require password after screen saver begins", "Open Anyway"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "Require password") > strings.Index(text, "Screen Saver -> AgentScreensaver") {
		t.Error("how to select the screen saver must come last")
	}
}

func TestUninstallRemovesWhatInstallAdded(t *testing.T) {
	env, services, stdout, home, artifacts := newSetup(t, "")
	if code := env.install([]string{"--yes", "--artifacts", artifacts}); code != 0 {
		t.Fatalf("install exit %d\n%s", code, stdout)
	}

	if code := env.uninstall([]string{"--yes"}); code != 0 {
		t.Fatalf("uninstall exit %d\n%s", code, stdout)
	}

	if services.removed != 2 {
		t.Errorf("services removed = %d", services.removed)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "Screen Savers", "AgentScreensaver.saver")); !os.IsNotExist(err) {
		t.Errorf("saver still installed (err %v)", err)
	}
}

func TestUninstallWithoutRecordFails(t *testing.T) {
	env, _, stdout, _, _ := newSetup(t, "")
	if code := env.uninstall([]string{"--yes"}); code != 1 || !strings.Contains(stdout.String(), "not installed") {
		t.Errorf("exit %d\n%s", code, stdout)
	}
}
