package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
)

const testBin = "/Applications/agent screensaver/agent-screensaver"

// pluginAgent writes an outside, opencode-shaped agent: a plugin file the
// tool runs, the event name from --event and a converter program.
func pluginAgent(t *testing.T, id string) agents.Agent {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	writeFile(t, filepath.Join(dir, "extension.json"), []byte(fmt.Sprintf(`{"id":%q,"name":"Acme","version":"1.0.0",
		"type":"agent","api":1,"agent":{"source":"acme","mark":"mark.svg",
		"hooks":{"event":{"arg":"event"},"converter":{"run":"bin/convert"}},
		"install":{"files":[{"from":"plugin/agent-screensaver.js","to":"~/.acme/plugins/agent-screensaver.js"}]}}}`, id)))
	writeFile(t, filepath.Join(dir, "mark.svg"), readFixture(t, filepath.Join("cursor", "mark.svg")))
	writeFile(t, filepath.Join(dir, "plugin", "agent-screensaver.js"), []byte("// runs agent-screensaver hook --tool acme"))
	writeFile(t, filepath.Join(dir, "bin", "convert"), []byte("#!/bin/sh\ncat >/dev/null\n"))
	if err := os.Chmod(filepath.Join(dir, "bin", "convert"), 0o755); err != nil {
		t.Fatal(err)
	}
	return loadAgentDir(t, dir)
}

func cursorAgent(t *testing.T) agents.Agent {
	t.Helper()
	return loadAgentDir(t, filepath.Join("testdata", "cursor"))
}

func cursorPath(home string) string { return filepath.Join(home, ".cursor", "hooks.json") }

// flatCommands lists the commands of one event of a flat hook file.
func flatCommands(t *testing.T, file map[string]any, event string) []string {
	t.Helper()
	var commands []string
	for _, handler := range file["hooks"].(map[string]any)[event].([]any) {
		commands = append(commands, handler.(map[string]any)["command"].(string))
	}
	return commands
}

func addAgent(t *testing.T, home string, agent agents.Agent, rec *Record) {
	t.Helper()
	if err := AddAgentHooks(home, testBin, agent, rec); err != nil {
		t.Fatalf("AddAgentHooks: %v", err)
	}
}

func TestFlatShapeWritesNewFileKeysAndHandlerKeys(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}

	addAgent(t, home, cursorAgent(t), &rec)

	data, err := os.ReadFile(cursorPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "{\n  \"version\": 1,\n  \"hooks\": {") {
		t.Errorf("new_file key must come first:\n%s", data)
	}
	stop := readJSON(t, cursorPath(home))["hooks"].(map[string]any)["stop"].([]any)
	handler := stop[0].(map[string]any)
	if len(stop) != 1 || handler["type"] != "command" || handler["timeout"] != float64(5) {
		t.Errorf("stop = %v", stop)
	}
	again := rec
	addAgent(t, home, cursorAgent(t), &again)
	if second, _ := os.ReadFile(cursorPath(home)); !bytes.Equal(data, second) {
		t.Error("a second add changed the file")
	}
}

func TestEventWithReplyGetsEventArg(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}

	addAgent(t, home, cursorAgent(t), &rec)

	file := readJSON(t, cursorPath(home))
	ask := shellQuote(testBin) + " hook --tool cursor --event beforeShellExecution --reply ask"
	plain := shellQuote(testBin) + " hook --tool cursor --reply empty_object"
	if got := flatCommands(t, file, "beforeShellExecution"); !slices.Equal(got, []string{ask}) {
		t.Errorf("beforeShellExecution = %v, want [%s]", got, ask)
	}
	if got := flatCommands(t, file, "stop"); !slices.Equal(got, []string{plain}) {
		t.Errorf("stop = %v, want [%s]", got, plain)
	}
	entry, _ := rec.agentRecord("builtin.cursor")
	if !slices.Contains(entry.Commands, ask) || !slices.Contains(entry.Commands, plain) {
		t.Errorf("record commands = %v", entry.Commands)
	}
}

func TestCreatedFileRemovedWhenOnlyNewFileKeysLeft(t *testing.T) {
	tests := []struct {
		name       string
		userKey    bool
		wantExists bool
	}{
		{"only new_file keys left", false, false},
		{"the user added a key", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			rec := Record{Version: RecordVersion}
			agent := cursorAgent(t)
			addAgent(t, home, agent, &rec)
			if tt.userKey {
				data, _ := os.ReadFile(cursorPath(home))
				writeFile(t, cursorPath(home), bytes.Replace(data, []byte(`"version": 1,`), []byte(`"version": 1, "mine": true,`), 1))
			}

			if err := RemoveAgentHooks(home, agent, &rec); err != nil {
				t.Fatal(err)
			}

			_, err := os.Stat(cursorPath(home))
			if exists := err == nil; exists != tt.wantExists {
				t.Fatalf("file exists = %v, want %v", exists, tt.wantExists)
			}
			if tt.wantExists {
				if got := compact(t, mustRead(t, cursorPath(home))); got != `{"version":1,"mine":true}` {
					t.Errorf("file = %s", got)
				}
			}
			if _, ok := rec.agentRecord(agent.ID); ok {
				t.Error("the agent is still in the record")
			}
		})
	}
}

func TestUninstallUsesRecordOnly(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	agent := cursorAgent(t)
	addAgent(t, home, agent, &rec)
	// The manifest changes after install to list one more event, and the
	// user put our command there by hand.
	ours := shellQuote(testBin) + " hook --tool cursor --reply empty_object"
	data := mustRead(t, cursorPath(home))
	writeFile(t, cursorPath(home), bytes.Replace(data, []byte(`"hooks": {`),
		[]byte(`"hooks": {"subagentStop": [{"type": "command", "command": "`+jsonEscaped(ours)+`"}],`), 1))
	edited := agent
	recipe := *agent.Install.Hooks
	recipe.Events = append(slices.Clone(recipe.Events), "subagentStop")
	edited.Install.Hooks = &recipe

	if err := RemoveAgentHooks(home, edited, &rec); err != nil {
		t.Fatal(err)
	}

	if got := compact(t, mustRead(t, cursorPath(home))); got != `{"version":1,"hooks":{"subagentStop":[{"type":"command","command":"`+jsonEscaped(ours)+`"}]}}` {
		t.Errorf("file = %s", got)
	}
}

func TestUninstallAgentWhoseExtensionIsGone(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	addAgent(t, home, cursorAgent(t), &rec)
	data := mustRead(t, cursorPath(home))
	writeFile(t, cursorPath(home), bytes.Replace(data, []byte(`"stop": [`),
		[]byte(`"stop": [{"type": "command", "command": "/mine/stop.sh"},`), 1))
	gone := agents.Agent{ID: "builtin.cursor"}

	t.Run("a record path outside the rules is refused", func(t *testing.T) {
		bad := rec
		bad.Agents = []AgentRecord{rec.Agents[0]}
		bad.Agents[0].File = "~/../elsewhere/hooks.json"
		if err := RemoveAgentHooks(home, gone, &bad); err == nil || !strings.Contains(err.Error(), "..") {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("only our exact commands go", func(t *testing.T) {
		if err := RemoveAgentHooks(home, gone, &rec); err != nil {
			t.Fatal(err)
		}
		if got := compact(t, mustRead(t, cursorPath(home))); got != `{"version":1,"hooks":{"stop":[{"type":"command","command":"/mine/stop.sh"}]}}` {
			t.Errorf("file = %s", got)
		}
	})
}

func TestCopiedFileRemovedOnlyIfHashMatches(t *testing.T) {
	tests := []struct {
		name       string
		userEdit   bool
		wantExists bool
	}{
		{"our copy goes", false, false},
		{"a file the user changed stays", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			rec := Record{Version: RecordVersion}
			agent := pluginAgent(t, "acme.plugin")
			addAgent(t, home, agent, &rec)
			target := filepath.Join(home, ".acme", "plugins", "agent-screensaver.js")
			if tt.userEdit {
				writeFile(t, target, []byte("// my own changes"))
			}

			if err := RemoveAgentHooks(home, agent, &rec); err != nil {
				t.Fatal(err)
			}

			if _, err := os.Stat(target); (err == nil) != tt.wantExists {
				t.Errorf("file exists = %v, want %v", err == nil, tt.wantExists)
			}
		})
	}
}

func TestPlanNamesOutsideAgentAndConverter(t *testing.T) {
	home := t.TempDir()

	steps, err := PlanAgent(home, testBin, pluginAgent(t, "acme.x"), Record{})

	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"from acme.x 1.0.0", "runs its own program bin/convert", "Copy a plugin file that Acme will run"} {
		index := slices.IndexFunc(steps, func(step PlannedStep) bool { return strings.Contains(step.Description, want) })
		if index < 0 || !steps[index].Warning {
			t.Errorf("no warning line with %q in %+v", want, steps)
		}
	}
	builtin, err := PlanAgent(home, testBin, cursorAgent(t), Record{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(builtin, func(step PlannedStep) bool { return step.Warning }) {
		t.Errorf("a built-in agent got a warning: %+v", builtin)
	}
}

func TestInstallSkipsAgentWhoseDetectIsMissing(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	artifacts, binary := artifactsFixture(t)

	record, err := Apply(home, binary, artifacts, &fakeServices{})

	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(codexPath(home)); !os.IsNotExist(err) {
		t.Errorf("codex hooks were written without Codex (err %v)", err)
	}
	if _, ok := record.agentRecord("builtin.codex"); ok {
		t.Error("codex is in the record")
	}
	if _, ok := record.agentRecord("builtin.claude"); !ok {
		t.Error("claude is missing from the record")
	}
}

// v1Home lays out what the install code before Task 6.4 left on disk:
// its output files (the golden files) and its version 1 record.
func v1Home(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	withTools(t, home)
	for golden, path := range map[string]string{"user_claude_settings.json": claudePath(home), "user_codex_hooks.json": codexPath(home)} {
		writeFile(t, path, bytes.ReplaceAll(readFixture(t, filepath.Join("golden", golden)), []byte(homeMarker), []byte(home)))
	}
	p := paths.ForHome(home)
	bin := filepath.Join(p.DataDir, "bin", binaryName)
	writeFile(t, p.InstallRecord, []byte(fmt.Sprintf(`{"bin":%q,"agents":["claude","codex"],
		"previous_statusline":"~/.claude/fake-statusline.sh","services":[%q,%q],"saver":%q}`,
		bin, DaemonLabel, TrayLabel, filepath.Join(home, "Library", "Screen Savers", saverName))))
	cfg := config.Default()
	cfg.PreviousStatusline = "~/.claude/fake-statusline.sh"
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestUpgradeFromRecordV1(t *testing.T) {
	home := v1Home(t)
	codexBefore := mustRead(t, codexPath(home))
	claudeBefore := mustRead(t, claudePath(home))

	record := apply(t, home, &fakeServices{})

	if got := mustRead(t, codexPath(home)); !bytes.Equal(got, codexBefore) {
		t.Errorf("codex hooks changed:\n%s", got)
	}
	want := bytes.ReplaceAll(claudeBefore, statuslineBefore, statuslineNow)
	if got := mustRead(t, claudePath(home)); !bytes.Equal(got, want) {
		t.Errorf("claude settings:\n%s\nwant only the status line command changed:\n%s", got, want)
	}
	for _, path := range []string{claudePath(home), codexPath(home)} {
		if _, err := os.Stat(path + backupSuffix); !os.IsNotExist(err) {
			t.Errorf("%s was backed up again: our earlier entries are in it", path)
		}
	}
	if previousStatusline(record) != "~/.claude/fake-statusline.sh" {
		t.Errorf("saved command = %q", previousStatusline(record))
	}
	saved, err := LoadRecord(paths.ForHome(home).InstallRecord, nil)
	if err != nil || saved.Version != RecordVersion || len(saved.Agents) != 2 {
		t.Fatalf("record = %+v, err %v", saved, err)
	}
	if err := Uninstall(home, &fakeServices{}); err != nil {
		t.Fatal(err)
	}
	if got, want := compact(t, mustRead(t, claudePath(home))), compact(t, readFixture(t, "claude_settings.json")); got != want {
		t.Errorf("after uninstall:\n%s\nwant:\n%s", got, want)
	}
}

func TestLoadRecordReadsVersion1(t *testing.T) {
	home := v1Home(t)
	known := []agents.Agent{
		loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.claude")),
		loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.codex")),
	}

	record, err := LoadRecord(paths.ForHome(home).InstallRecord, known)

	if err != nil {
		t.Fatal(err)
	}
	claude, _ := record.agentRecord("builtin.claude")
	bin := shellQuote(record.Bin)
	if claude.File != "~/.claude/settings.json" || !slices.Equal(claude.Commands, []string{bin + " hook --tool claude"}) ||
		claude.StatuslineCommand != bin+" statusline" || claude.PreviousCommand != "~/.claude/fake-statusline.sh" || len(claude.Events) != 7 {
		t.Errorf("claude entry = %+v", claude)
	}
	codex, _ := record.agentRecord("builtin.codex")
	if codex.File != "~/.codex/hooks.json" || codex.StatuslineKey != "" || len(codex.Events) != 6 {
		t.Errorf("codex entry = %+v", codex)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestUninstallRemovesOnlyCommandsOfOurForm(t *testing.T) {
	home := t.TempDir()
	writeFile(t, claudePath(home), readFixture(t, "claude_settings.json"))
	sm := &fakeServices{}
	apply(t, home, sm)
	p := paths.ForHome(home)
	record, err := LoadRecord(p.InstallRecord, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Someone edits the record to claim the user's own Stop hook and status
	// line as ours.
	userStop := `[ -x "$SUPERSET_HOME_DIR/hooks/notify.sh" ] && "$SUPERSET_HOME_DIR/hooks/notify.sh" || true`
	for index := range record.Agents {
		if record.Agents[index].ID == "builtin.claude" {
			record.Agents[index].Commands = append(record.Agents[index].Commands, userStop, "/opt/fake/start.sh")
			record.Agents[index].Events = append(record.Agents[index].Events, "SessionStart")
		}
	}
	if err := SaveRecord(p.InstallRecord, record); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(home, sm); err != nil {
		t.Fatal(err)
	}

	if got, want := compact(t, mustRead(t, claudePath(home))), compact(t, readFixture(t, "claude_settings.json")); got != want {
		t.Errorf("after uninstall:\n%s\nwant the user's file back:\n%s", got, want)
	}
}

func TestOwnCommand(t *testing.T) {
	bin := "/Applications/agent screensaver/agent-screensaver"
	tests := []struct {
		command string
		want    bool
	}{
		{shellQuote(bin) + " hook --tool claude", true},
		{shellQuote(bin) + " hook --tool cursor --event beforeShellExecution --reply ask", true},
		{shellQuote(bin) + " hook --tool cursor --reply empty_object", true},
		{shellQuote(bin) + " statusline", true},
		{shellQuote(bin) + " statusline --tool claude", true},
		{shellQuote(bin) + " hook --tool codex", false},
		{shellQuote(bin) + " hook --tool claude; rm -rf ~", false},
		{shellQuote(bin) + " hook --tool claude --reply allow", false},
		{"'/other/agent-screensaver' hook --tool claude", false},
		{"/opt/fake/start.sh", false},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			if got := ownCommand(bin, "claude", strings.Replace(tt.command, "--tool cursor", "--tool claude", 1)); got != tt.want {
				t.Errorf("ownCommand = %v, want %v", got, tt.want)
			}
		})
	}
}
