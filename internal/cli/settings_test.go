package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/paths"
)

const cliTheme = `{"id":"acme.dark","name":"Dark","version":"1.0.0","type":"theme","api":1,
"theme":{"background":{"id":"acme.rain"},"layout":"builtin.layout","style":{}}}`

const cliBackground = `{"id":"acme.rain","name":"Rain","version":"1.0.0","type":"background","api":1,"entry":"main.js",
"options":{"speed":{"type":"number","default":1,"min":0.1,"max":5}}}`

func extensionFolder(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"extension.json": manifest, "main.js": "//"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runSettings(t *testing.T, p paths.Paths, command string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Settings(command, args, &stdout, &stderr, p)
	return code, stdout.String(), stderr.String()
}

func TestSettingsCommands(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	background, theme := extensionFolder(t, cliBackground), extensionFolder(t, cliTheme)
	steps := []struct {
		name       string
		command    string
		args       []string
		wantCode   int
		wantStdout string // substring
		wantStderr string // substring
	}{
		{"install background", "ext", []string{"install", background}, 0, "installed: acme.rain 1.0.0", ""},
		{"install theme", "ext", []string{"install", theme}, 0, "installed: acme.dark 1.0.0", ""},
		{"theme set", "theme", []string{"set", "acme.dark"}, 0, "theme: acme.dark", ""},
		{"theme list marks active", "theme", []string{"list"}, 0, "acme.dark theme *\n", ""},
		{"ext list shows every type", "ext", []string{"list"}, 0, "acme.rain background\n", ""},
		{"background list marks the theme's", "background", []string{"list"}, 0, "acme.rain *\n", ""},
		{"background set", "background", []string{"set", "acme.rain"}, 0, "background: acme.rain\n", ""},
		{"background set a theme", "background", []string{"set", "acme.dark"}, 1, "", "not a background"},
		{"background set unknown", "background", []string{"set", "nobody.none"}, 1, "", "not installed"},
		{"background set default", "background", []string{"set", "default"}, 0, "background: default (the theme's)\n", ""},
		{"background no arguments", "background", nil, 1, "", "usage: background"},
		{"theme set unknown", "theme", []string{"set", "nobody.none"}, 1, "", "not installed"},
		{"option set prints stored value", "option", []string{"set", "acme.rain", "speed", "2"}, 0, "option: acme.rain speed = 2", ""},
		{"option set out of range", "option", []string{"set", "acme.rain", "speed", "99"}, 1, "", "above the maximum"},
		{"option reset all", "option", []string{"reset", "acme.rain"}, 0, "reset", ""},
		{"privacy on", "privacy", []string{"on"}, 0, "privacy: on", ""},
		{"privacy bad word", "privacy", []string{"maybe"}, 1, "", "usage: privacy on|off"},
		{"sound off", "sound", []string{"off"}, 0, "sound: off", ""},
		{"fps number", "fps", []string{"max", "30"}, 0, "fps max: 30", ""},
		{"fps display", "fps", []string{"max", "display"}, 0, "fps max: display", ""},
		{"fps too high", "fps", []string{"max", "500"}, 1, "", "out of range"},
		{"fps not a number", "fps", []string{"max", "fast"}, 1, "", "not a number"},
		{"ext remove", "ext", []string{"remove", "acme.rain"}, 0, "removed: acme.rain", ""},
		{"ext remove builtin", "ext", []string{"remove", "builtin.default"}, 1, "", "reserved"},
		{"no arguments", "theme", nil, 1, "", "usage: theme"},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			code, stdout, stderr := runSettings(t, p, step.command, step.args...)
			if code != step.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr %q)", code, step.wantCode, stderr)
			}
			if !strings.Contains(stdout, step.wantStdout) || !strings.Contains(stderr, step.wantStderr) {
				t.Errorf("stdout %q stderr %q, want %q / %q", stdout, stderr, step.wantStdout, step.wantStderr)
			}
			if step.wantCode != 0 && stdout != "" {
				t.Errorf("a failure must print nothing on stdout, got %q", stdout)
			}
		})
	}
}

func TestOptionSetPrintsAerialsHint(t *testing.T) {
	const hint = "Pick it once in System Settings → Screen Saver, and macOS will download the videos.\n"
	p := paths.ForHome(t.TempDir())
	p.Aerials.Manifest = filepath.Join(t.TempDir(), "missing.json")
	p.Aerials.Catalog = p.Aerials.Manifest
	manifest := `{"id":"builtin.aerials","type":"background","api":1,"entry":"index.js",
"options":{"group":{"type":"enum","default":"cities","values":["cities","space"]}}}`
	dir := filepath.Join(p.ExtensionsDir, "builtin.aerials")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"extension.json": manifest, "index.js": "//"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	code, stdout, stderr := runSettings(t, p, "option", "set", "builtin.aerials", "group", "space")

	if want := "option: builtin.aerials group = space\n" + hint; code != 0 || stdout != want {
		t.Errorf("exit %d, stdout %q (stderr %q), want %q", code, stdout, stderr, want)
	}
}
