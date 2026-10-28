package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
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
	code := Settings(command, args, strings.NewReader(""), &stdout, &stderr, p)
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

func TestDemoCommand(t *testing.T) {
	armed := []string{"--seconds", "90"} // a demo armed before the step
	tests := []struct {
		name        string
		before      []string // demo arguments run first; nil runs nothing
		args        []string
		wantCode    int
		wantOut     string // substring of stdout, or of stderr on failure
		wantSeconds int
		wantArmed   bool
	}{
		{"default length", nil, nil, 0, "Demo ready. Start the screen saver now; it plays for 60 seconds.\n", 60, true},
		{"chosen length", nil, []string{"--seconds", "90"}, 0, "it plays for 90 seconds.", 90, true},
		{"too short keeps the armed demo", armed, []string{"--seconds", "5"}, 1, "out of range", 90, true},
		{"too long keeps the armed demo", armed, []string{"--seconds", "601"}, 1, "out of range", 90, true},
		{"not a number", nil, []string{"--seconds", "long"}, 1, "usage: demo", 0, false},
		{"unknown word", nil, []string{"start"}, 1, "usage: demo", 0, false},
		{"stop clears", armed, []string{"stop"}, 0, "Demo stopped.", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := paths.ForHome(t.TempDir())
			if tt.before != nil {
				if code, _, stderr := runSettings(t, p, "demo", tt.before...); code != 0 {
					t.Fatalf("setup failed: %s", stderr)
				}
			}
			code, stdout, stderr := runSettings(t, p, "demo", tt.args...)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, stderr)
			}
			if out := stdout + stderr; !strings.Contains(out, tt.wantOut) {
				t.Errorf("output %q, want %q", out, tt.wantOut)
			}
			cfg := config.Load(p.ConfigFile)
			if cfg.DemoSeconds != tt.wantSeconds || (cfg.DemoArmedMs != 0) != tt.wantArmed {
				t.Errorf("demo_seconds = %d, demo_armed_ms = %d; want %d, armed %v",
					cfg.DemoSeconds, cfg.DemoArmedMs, tt.wantSeconds, tt.wantArmed)
			}
		})
	}
}

const (
	cliPackTheme = `{"id":"acme.dark","name":"Dark","version":"1.0.0","type":"theme","api":1,
"theme":{"background":{"id":"acme.rain"},"layout":"acme.layout","style":{}}}`
	cliPackLayout = `{"id":"acme.layout","name":"Layout","version":"1.0.0","type":"layout","api":1,
"layout":{"modes":{"idle":{"center":[{"id":"acme.clock"}]},"attention":{"bottom":[]}}}}`
	cliPackClock  = `{"id":"acme.clock","name":"Clock","version":"2.0.0","type":"component","api":1,"entry":"main.js"}`
	cliLightTheme = `{"id":"acme.light","name":"Light","version":"1.0.0","type":"theme","api":1,
"theme":{"background":{"id":"acme.rain"},"layout":"builtin.layout","style":{}}}`
)

// packFolder puts each manifest in its own folder of one pack folder.
func packFolder(t *testing.T, manifests ...string) string {
	t.Helper()
	pack := t.TempDir()
	for index, manifest := range manifests {
		if err := os.Rename(extensionFolder(t, manifest), filepath.Join(pack, "part"+string(rune('a'+index)))); err != nil {
			t.Fatal(err)
		}
	}
	return pack
}

func TestExtInstallAPackAndRemoveItsTheme(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	pack := packFolder(t, cliPackTheme, cliPackLayout, cliBackground, cliPackClock)

	code, stdout, stderr := runSettings(t, p, "ext", "install", pack)

	want := "installed: acme.dark 1.0.0\ninstalled: acme.layout 1.0.0\ninstalled: acme.rain 1.0.0\ninstalled: acme.clock 2.0.0\n"
	if code != 0 || stdout != want {
		t.Fatalf("install: exit %d, stdout %q (stderr %q), want %q", code, stdout, stderr, want)
	}
	if code, _, stderr := runSettings(t, p, "ext", "install", extensionFolder(t, cliLightTheme)); code != 0 {
		t.Fatal(stderr)
	}
	if code, _, stderr := runSettings(t, p, "theme", "set", "acme.dark"); code != 0 {
		t.Fatal(stderr)
	}

	code, stdout, stderr = runSettings(t, p, "ext", "remove", "acme.dark")

	want = "removed: acme.dark\nremoved: acme.layout\nremoved: acme.clock\nkept: acme.rain (used by acme.light)\n" +
		"The removed theme was in use, so the screen saver now uses the default theme.\n"
	if code != 0 || stdout != want {
		t.Fatalf("remove: exit %d, stdout %q (stderr %q), want %q", code, stdout, stderr, want)
	}
	if theme := config.Load(p.ConfigFile).Theme; theme != "builtin.default" {
		t.Errorf("theme = %q, want builtin.default", theme)
	}
}

func TestExtInstallAPackWithABadPartInstallsNothing(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	pack := packFolder(t, cliPackTheme, `{"id":"acme.broken","type":"background","api":1}`)

	code, stdout, stderr := runSettings(t, p, "ext", "install", pack)

	if code != 1 || stdout != "" || !strings.Contains(stderr, `part "partb"`) {
		t.Errorf("exit %d, stdout %q, stderr %q; want a failure that names partb", code, stdout, stderr)
	}
	if entries, _ := os.ReadDir(p.ExtensionsDir); len(entries) != 0 {
		t.Errorf("extensions folder holds %d entries, want none", len(entries))
	}
}

func TestExtRemoveOnlyRemovesOneID(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	if code, _, stderr := runSettings(t, p, "ext", "install", packFolder(t, cliPackTheme, cliPackLayout, cliBackground, cliPackClock)); code != 0 {
		t.Fatal(stderr)
	}

	code, stdout, stderr := runSettings(t, p, "ext", "remove", "--only", "acme.dark")

	if code != 0 || stdout != "removed: acme.dark\n" {
		t.Fatalf("exit %d, stdout %q (stderr %q)", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(p.ExtensionsDir, "acme.layout")); err != nil {
		t.Errorf("acme.layout was removed too: %v", err)
	}
}
