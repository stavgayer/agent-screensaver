package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
)

// fakeServices records calls instead of touching launchd.
type fakeServices struct {
	installed   map[string][]string
	uninstalled []string
}

func (f *fakeServices) Install(label string, args []string) (string, error) {
	if f.installed == nil {
		f.installed = map[string][]string{}
	}
	f.installed[label] = args
	return "/fake/" + label + ".plist", nil
}

func (f *fakeServices) Uninstall(label string) error {
	f.uninstalled = append(f.uninstalled, label)
	return nil
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// artifactsFixture builds a fake build folder and a fake binary. Every test
// uses a temp home; nothing here may point at the real one.
func artifactsFixture(t *testing.T) (artifacts, binary string) {
	t.Helper()
	root := t.TempDir()
	artifacts = filepath.Join(root, "artifacts")
	writeFile(t, filepath.Join(artifacts, "runtime", "index.html"), []byte("<html>"))
	writeFile(t, filepath.Join(artifacts, "extensions", "builtin.clock", "extension.json"), []byte("{}"))
	writeFile(t, filepath.Join(artifacts, "AgentScreensaver.saver", "Contents", "Info.plist"), []byte("plist"))
	binary = filepath.Join(root, "agent-screensaver")
	writeFile(t, binary, []byte("fake binary"))
	return artifacts, binary
}

func claudePath(home string) string { return filepath.Join(home, ".claude", "settings.json") }
func codexPath(home string) string  { return filepath.Join(home, ".codex", "hooks.json") }

func compact(t *testing.T, data []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, data); err != nil {
		t.Fatalf("compact: %v\n%s", err, data)
	}
	return out.String()
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return parsed
}

// commandsOf lists every hook command of one event, in file order.
func commandsOf(t *testing.T, file map[string]any, event string) []string {
	t.Helper()
	hooks, _ := file["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	var commands []string
	for _, group := range groups {
		for _, handler := range group.(map[string]any)["hooks"].([]any) {
			commands = append(commands, handler.(map[string]any)["command"].(string))
		}
	}
	return commands
}

func countOf(commands []string, want string) int {
	count := 0
	for _, command := range commands {
		if command == want {
			count++
		}
	}
	return count
}

func apply(t *testing.T, home string, sm *fakeServices) Record {
	t.Helper()
	artifacts, binary := artifactsFixture(t)
	record, err := Apply(home, binary, artifacts, sm)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return record
}

func TestInstallIntoMissingFiles(t *testing.T) {
	home := t.TempDir()
	sm := &fakeServices{}
	record := apply(t, home, sm)

	claudeEvents := []string{"UserPromptSubmit", "PreToolUse", "Notification", "PermissionRequest", "PostToolUse", "Stop", "SessionEnd"}
	codexEvents := []string{"UserPromptSubmit", "PreToolUse", "PermissionRequest", "PostToolUse", "Stop", "SessionEnd"}
	tests := []struct {
		name   string
		file   string
		events []string
		tool   string
	}{
		{"claude", claudePath(home), claudeEvents, "claude"},
		{"codex", codexPath(home), codexEvents, "codex"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := readJSON(t, tt.file)
			hooks := file["hooks"].(map[string]any)
			if len(hooks) != len(tt.events) {
				t.Fatalf("events = %d, want %d", len(hooks), len(tt.events))
			}
			want := shellQuote(record.Bin) + " hook --tool " + tt.tool
			for _, event := range tt.events {
				if got := commandsOf(t, file, event); len(got) != 1 || got[0] != want {
					t.Errorf("%s = %v, want [%s]", event, got, want)
				}
			}
		})
	}
	if _, ok := sm.installed[DaemonLabel]; !ok {
		t.Errorf("daemon service not installed: %v", sm.installed)
	}
	if got := sm.installed[DaemonLabel]; len(got) != 2 || got[0] != record.Bin || got[1] != "daemon" {
		t.Errorf("daemon args = %v", got)
	}
	for _, path := range []string{record.Bin, filepath.Join(paths.ForHome(home).RuntimeDir, "index.html"),
		filepath.Join(paths.ForHome(home).ExtensionsDir, "builtin.clock", "extension.json"), record.Saver} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}
}

func TestInstallPreservesForeignHooks(t *testing.T) {
	home := t.TempDir()
	original := readFixture(t, "claude_settings.json")
	writeFile(t, claudePath(home), original)
	record := apply(t, home, &fakeServices{})

	got := readJSON(t, claudePath(home))
	wantKeys := []string{"model", "env", "hooks", "statusLine", "permissions"}
	data, _ := os.ReadFile(claudePath(home))
	last := -1
	for _, key := range wantKeys {
		index := strings.Index(string(data), "\n  \""+key+"\"")
		if index <= last {
			t.Errorf("key %q moved or missing in:\n%s", key, data)
		}
		last = index
	}
	if !strings.Contains(string(data), "a && b <c>") {
		t.Errorf("user value was rewritten:\n%s", data)
	}
	stop := commandsOf(t, got, "Stop")
	if len(stop) != 2 || !strings.Contains(stop[0], "SUPERSET_HOME_DIR") || stop[1] != shellQuote(record.Bin)+" hook --tool claude" {
		t.Errorf("Stop = %v", stop)
	}
	if start := commandsOf(t, got, "SessionStart"); len(start) != 1 || start[0] != "/opt/fake/start.sh" {
		t.Errorf("SessionStart changed: %v", start)
	}
	if allow := got["permissions"].(map[string]any)["allow"].([]any); len(allow) != 1 {
		t.Errorf("permissions changed: %v", allow)
	}

}

func TestInstallIsIdempotent(t *testing.T) {
	home := t.TempDir()
	writeFile(t, claudePath(home), readFixture(t, "claude_settings.json"))
	writeFile(t, codexPath(home), readFixture(t, "codex_hooks.json"))
	sm := &fakeServices{}
	first := apply(t, home, sm)
	claudeAfterFirst, _ := os.ReadFile(claudePath(home))
	codexAfterFirst, _ := os.ReadFile(codexPath(home))

	second := apply(t, home, sm)

	claudeAfterSecond, _ := os.ReadFile(claudePath(home))
	codexAfterSecond, _ := os.ReadFile(codexPath(home))
	if !bytes.Equal(claudeAfterFirst, claudeAfterSecond) || !bytes.Equal(codexAfterFirst, codexAfterSecond) {
		t.Error("second install changed the files")
	}
	if second.PreviousStatusline != first.PreviousStatusline || second.PreviousStatusline == "" {
		t.Errorf("previous statusline lost: %q then %q", first.PreviousStatusline, second.PreviousStatusline)
	}
	ours := shellQuote(first.Bin) + " hook --tool codex"
	if count := countOf(commandsOf(t, readJSON(t, codexPath(home)), "Stop"), ours); count != 1 {
		t.Errorf("our Codex Stop entries = %d, want 1", count)
	}
}

func TestStatuslineSwappedAndSaved(t *testing.T) {
	tests := []struct {
		name         string
		fixture      string
		wantPrevious string
		wantPadding  bool
	}{
		{"user has one", "claude_settings.json", "~/.claude/fake-statusline.sh", true},
		{"user has none", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.fixture != "" {
				writeFile(t, claudePath(home), readFixture(t, tt.fixture))
			}
			record := apply(t, home, &fakeServices{})

			line := readJSON(t, claudePath(home))["statusLine"].(map[string]any)
			if line["command"] != shellQuote(record.Bin)+" statusline" || line["type"] != "command" {
				t.Errorf("statusLine = %v", line)
			}
			if _, hasPadding := line["padding"]; hasPadding != tt.wantPadding {
				t.Errorf("padding kept = %v, want %v", hasPadding, tt.wantPadding)
			}
			if record.PreviousStatusline != tt.wantPrevious {
				t.Errorf("record previous = %q, want %q", record.PreviousStatusline, tt.wantPrevious)
			}
			if got := config.Load(paths.ForHome(home).ConfigFile).PreviousStatusline; got != tt.wantPrevious {
				t.Errorf("config previous = %q, want %q", got, tt.wantPrevious)
			}
		})
	}
}

func TestUninstallRestoresEverything(t *testing.T) {
	tests := []struct {
		name string
		path func(string) string
		file string
	}{
		{"claude", claudePath, "claude_settings.json"},
		{"codex", codexPath, "codex_hooks.json"},
	}
	home := t.TempDir()
	for _, tt := range tests {
		writeFile(t, tt.path(home), readFixture(t, tt.file))
	}
	sm := &fakeServices{}
	record := apply(t, home, sm)

	if err := Uninstall(home, sm); err != nil {
		t.Fatal(err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.path(home))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := compact(t, data), compact(t, readFixture(t, tt.file)); got != want {
				t.Errorf("after uninstall:\n%s\nwant:\n%s", got, want)
			}
		})
	}
	if len(sm.uninstalled) != 1 || sm.uninstalled[0] != DaemonLabel {
		t.Errorf("uninstalled services = %v", sm.uninstalled)
	}
	p := paths.ForHome(home)
	for _, path := range []string{record.Bin, record.Saver, p.RuntimeDir, p.InstallRecord,
		filepath.Join(p.ExtensionsDir, "builtin.clock")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s still exists (err %v)", path, err)
		}
	}
	if got := config.Load(p.ConfigFile).PreviousStatusline; got != "" {
		t.Errorf("config previous = %q after uninstall", got)
	}
}

func TestUninstallAfterUserEditKeepsUserEntries(t *testing.T) {
	home := t.TempDir()
	writeFile(t, claudePath(home), readFixture(t, "claude_settings.json"))
	sm := &fakeServices{}
	record := apply(t, home, sm)

	// The user adds a hook inside our group, a new group, and a new event.
	edited := readJSON(t, claudePath(home))
	hooks := edited["hooks"].(map[string]any)
	userHook := map[string]any{"type": "command", "command": "/mine/hook.sh"}
	stop := hooks["Stop"].([]any)
	ourGroup := stop[len(stop)-1].(map[string]any)
	ourGroup["hooks"] = append(ourGroup["hooks"].([]any), userHook)
	hooks["Stop"] = append(stop, map[string]any{"hooks": []any{userHook}})
	hooks["PreCompact"] = []any{map[string]any{"hooks": []any{userHook}}}
	data, _ := json.Marshal(edited)
	writeFile(t, claudePath(home), data)

	if err := Uninstall(home, sm); err != nil {
		t.Fatal(err)
	}

	got := readJSON(t, claudePath(home))
	ours := shellQuote(record.Bin) + " hook --tool claude"
	for event := range got["hooks"].(map[string]any) {
		if countOf(commandsOf(t, got, event), ours) != 0 {
			t.Errorf("our entry left in %s", event)
		}
	}
	if stopCommands := commandsOf(t, got, "Stop"); len(stopCommands) != 3 || stopCommands[1] != "/mine/hook.sh" || stopCommands[2] != "/mine/hook.sh" {
		t.Errorf("Stop = %v", stopCommands)
	}
	if compactCommands := commandsOf(t, got, "PreCompact"); len(compactCommands) != 1 {
		t.Errorf("user event lost: %v", compactCommands)
	}
	if _, ok := got["hooks"].(map[string]any)["UserPromptSubmit"]; ok {
		t.Error("empty event we created was not removed")
	}
}

func TestUninstallKeepsUserStatuslineIfChanged(t *testing.T) {
	home := t.TempDir()
	writeFile(t, claudePath(home), readFixture(t, "claude_settings.json"))
	sm := &fakeServices{}
	apply(t, home, sm)
	edited := readJSON(t, claudePath(home))
	edited["statusLine"] = map[string]any{"type": "command", "command": "/mine/line.sh"}
	data, _ := json.Marshal(edited)
	writeFile(t, claudePath(home), data)

	if err := Uninstall(home, sm); err != nil {
		t.Fatal(err)
	}

	line := readJSON(t, claudePath(home))["statusLine"].(map[string]any)
	if line["command"] != "/mine/line.sh" {
		t.Errorf("statusLine = %v", line)
	}
}

func TestInstallAbortsOnInvalidUserJSON(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{"truncated", readFixture(t, "invalid.json")},
		{"byte order mark", append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"model":"opus"}`)...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, claudePath(home), tt.content)
			codexOriginal := readFixture(t, "codex_hooks.json")
			writeFile(t, codexPath(home), codexOriginal)
			sm := &fakeServices{}
			artifacts, binary := artifactsFixture(t)

			_, err := Apply(home, binary, artifacts, sm)

			if err == nil || !strings.Contains(err.Error(), claudePath(home)) || !strings.Contains(err.Error(), "nothing was changed") {
				t.Fatalf("error = %v", err)
			}
			if got, _ := os.ReadFile(claudePath(home)); !bytes.Equal(got, tt.content) {
				t.Error("invalid file was modified")
			}
			if got, _ := os.ReadFile(codexPath(home)); !bytes.Equal(got, codexOriginal) {
				t.Error("valid file was modified")
			}
			p := paths.ForHome(home)
			for _, path := range []string{claudePath(home) + backupSuffix, codexPath(home) + backupSuffix, p.DataDir} {
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Errorf("%s exists after aborted install", path)
				}
			}
			if len(sm.installed) != 0 {
				t.Error("service installed after abort")
			}
		})
	}
}

func TestInstallAbortsOnCorruptOwnConfig(t *testing.T) {
	home := t.TempDir()
	p := paths.ForHome(home)
	corrupt := []byte("{not json")
	writeFile(t, p.ConfigFile, corrupt)
	claudeOriginal := readFixture(t, "claude_settings.json")
	writeFile(t, claudePath(home), claudeOriginal)
	sm := &fakeServices{}
	artifacts, binary := artifactsFixture(t)

	_, err := Apply(home, binary, artifacts, sm)

	if err == nil || !strings.Contains(err.Error(), p.ConfigFile) || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("error = %v", err)
	}
	if got, _ := os.ReadFile(p.ConfigFile); !bytes.Equal(got, corrupt) {
		t.Error("corrupt config was replaced")
	}
	if got, _ := os.ReadFile(claudePath(home)); !bytes.Equal(got, claudeOriginal) {
		t.Error("user file was written")
	}
	if _, statErr := os.Stat(claudePath(home) + backupSuffix); !os.IsNotExist(statErr) {
		t.Error("backup written after aborted install")
	}
	if len(sm.installed) != 0 {
		t.Error("service installed after abort")
	}
}

func TestInstallWritesBackupOfOriginal(t *testing.T) {
	home := t.TempDir()
	original := readFixture(t, "claude_settings.json")
	writeFile(t, claudePath(home), original)
	writeFile(t, claudePath(home)+backupSuffix, []byte("older backup"))
	apply(t, home, &fakeServices{})

	got, err := os.ReadFile(claudePath(home) + backupSuffix)
	if err != nil || !bytes.Equal(got, original) {
		t.Errorf("backup = %q (err %v)", got, err)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/plain/path", "'/plain/path'"},
		{"/Application Support/bin", "'/Application Support/bin'"},
		{"it's", `'it'\''s'`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := shellQuote(tt.in); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPlanNamesRealPaths(t *testing.T) {
	home := "/home/example"
	var text strings.Builder
	for _, step := range Plan(home) {
		text.WriteString(step.Description + "\n")
	}
	for _, want := range []string{claudePath(home), codexPath(home), paths.ForHome(home).DataDir, "Screen Savers"} {
		if !strings.Contains(text.String(), want) {
			t.Errorf("plan lacks %q:\n%s", want, text.String())
		}
	}
}

// failingServices is a service manager whose Install fails after, as launchd
// can, leaving its plist behind.
type failingServices struct{ fakeServices }

func (f *failingServices) Install(string, []string) (string, error) {
	return "", errors.New("launchctl bootstrap: Input/output error")
}

func TestUninstallAfterFailedServiceInstallStillRemovesService(t *testing.T) {
	home := t.TempDir()
	sm := &failingServices{}
	artifacts, binary := artifactsFixture(t)
	if _, err := Apply(home, binary, artifacts, sm); err == nil {
		t.Fatal("Apply should report the service failure")
	}

	if err := Uninstall(home, &sm.fakeServices); err != nil {
		t.Fatal(err)
	}

	if len(sm.uninstalled) != 1 || sm.uninstalled[0] != DaemonLabel {
		t.Errorf("uninstalled services = %v", sm.uninstalled)
	}
}

func TestRetryAfterPartialFailureKeepsOriginalStatusline(t *testing.T) {
	home := t.TempDir()
	original := readFixture(t, "claude_settings.json")
	writeFile(t, claudePath(home), original)
	writeFile(t, codexPath(home), readFixture(t, "codex_hooks.json"))
	codexDir := filepath.Dir(codexPath(home))
	if err := os.Chmod(codexDir, 0o500); err != nil { // the Codex write fails after the Claude write succeeded
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(codexDir, 0o755) })
	sm := &fakeServices{}
	artifacts, binary := artifactsFixture(t)
	if _, err := Apply(home, binary, artifacts, sm); err == nil {
		t.Fatal("first Apply should fail on the read-only Codex folder")
	}
	if err := os.Chmod(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(home, binary, artifacts, sm); err != nil {
		t.Fatalf("retry: %v", err)
	}

	if got, err := os.ReadFile(claudePath(home) + backupSuffix); err != nil || !bytes.Equal(got, original) {
		t.Errorf("backup no longer holds the original (err %v)", err)
	}
	if err := Uninstall(home, sm); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(claudePath(home))
	if got, want := compact(t, data), compact(t, original); got != want {
		t.Errorf("after uninstall:\n%s\nwant:\n%s", got, want)
	}
}

func TestUninstallDeletesOnlyOwnPathsNotRecordPaths(t *testing.T) {
	home := t.TempDir()
	sm := &fakeServices{}
	apply(t, home, sm)
	p := paths.ForHome(home)
	writeFile(t, filepath.Join(p.ExtensionsDir, "acme.rain", "extension.json"), []byte("{}"))
	record, err := loadRecord(p.InstallRecord)
	if err != nil {
		t.Fatal(err)
	}
	record.Bin = p.DataDir
	record.Saver = home
	if err := saveRecord(p.InstallRecord, record); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(home, sm); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{p.ConfigFile, filepath.Join(p.ExtensionsDir, "acme.rain", "extension.json"), claudePath(home)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s was deleted: %v", path, err)
		}
	}
}
