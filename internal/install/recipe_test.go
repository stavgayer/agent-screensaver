package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	writeFile(t, filepath.Join(dir, "mark.svg"), readRepoFile(t, filepath.Join("extensions", "builtin.cursor", "mark.svg")))
	writeFile(t, filepath.Join(dir, "plugin", "agent-screensaver.js"), []byte("// runs agent-screensaver hook --tool acme"))
	writeFile(t, filepath.Join(dir, "bin", "convert"), []byte("#!/bin/sh\ncat >/dev/null\n"))
	if err := os.Chmod(filepath.Join(dir, "bin", "convert"), 0o755); err != nil {
		t.Fatal(err)
	}
	return loadAgentDir(t, dir)
}

func cursorAgent(t *testing.T) agents.Agent {
	t.Helper()
	return loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.cursor"))
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

	// The built-in Cursor agent has no reply of its own any more; the engine
	// still supports one.
	agent := cursorAgent(t)
	agent.Hooks.Reply = agents.Reply{Events: map[string]string{"beforeShellExecution": agents.ReplyEmptyObject}}
	addAgent(t, home, agent, &rec)

	file := readJSON(t, cursorPath(home))
	withReply := shellQuote(testBin) + " hook --tool cursor --event beforeShellExecution --reply empty_object"
	plain := shellQuote(testBin) + " hook --tool cursor"
	if got := flatCommands(t, file, "beforeShellExecution"); !slices.Equal(got, []string{withReply}) {
		t.Errorf("beforeShellExecution = %v, want [%s]", got, withReply)
	}
	if got := flatCommands(t, file, "stop"); !slices.Equal(got, []string{plain}) {
		t.Errorf("stop = %v, want [%s]", got, plain)
	}
	entry, _ := rec.agentRecord("builtin.cursor")
	if !slices.Contains(entry.Commands, withReply) || !slices.Contains(entry.Commands, plain) {
		t.Errorf("record commands = %v", entry.Commands)
	}
}

// Older installs answered Cursor's permission hooks with --reply ask. The
// engine no longer writes it, but uninstall must still take it out.
func TestUninstallRemovesOldReplyAskCommand(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	agent := cursorAgent(t)
	addAgent(t, home, agent, &rec)
	plain := shellQuote(testBin) + " hook --tool cursor"
	old := plain + " --event beforeShellExecution --reply ask"
	data := mustRead(t, cursorPath(home))
	at := bytes.Index(data, []byte(`"beforeShellExecution"`))
	if at < 0 {
		t.Fatalf("install wrote no beforeShellExecution:\n%s", data)
	}
	edited := append(slices.Clone(data[:at]), bytes.Replace(data[at:], []byte(jsonEscaped(plain)+`"`), []byte(jsonEscaped(old)+`"`), 1)...)
	writeFile(t, cursorPath(home), edited)
	for index := range rec.Agents {
		if rec.Agents[index].ID == agent.ID {
			rec.Agents[index].Commands = append(rec.Agents[index].Commands, old)
		}
	}

	if err := RemoveAgentHooks(home, agent, &rec); err != nil {
		t.Fatal(err)
	}

	if after, err := os.ReadFile(cursorPath(home)); err == nil && bytes.Contains(after, []byte("hook --tool cursor")) {
		t.Errorf("an old command is left:\n%s", after)
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

	record, err := Apply(home, binary, artifacts, &fakeServices{}, Options{})

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

// objectKeys lists the keys of a JSON object in file order.
func objectKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if _, err := decoder.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			t.Fatal(err)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key.(string))
	}
	return keys
}

// hooksObject is the raw "hooks" object of a Cursor hook file.
func hooksObject(t *testing.T, data []byte) []byte {
	t.Helper()
	var file struct {
		Hooks json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	return file.Hooks
}

// ourHandlers are the handlers of a Cursor hook file whose command runs our
// binary, by event.
func ourHandlers(t *testing.T, data []byte) map[string][]map[string]any {
	t.Helper()
	var file struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	ours := map[string][]map[string]any{}
	for event, handlers := range file.Hooks {
		for _, handler := range handlers {
			if command, _ := handler["command"].(string); strings.HasPrefix(command, shellQuote(testBin)+" ") {
				ours[event] = append(ours[event], handler)
			}
		}
	}
	return ours
}

func TestAgentInstall(t *testing.T) {
	userFile := readFixture(t, "cursor_hooks.json")
	tests := []struct {
		name  string
		user  []byte // the hook file before install; nil means none
		check func(t *testing.T, home string, agent agents.Agent, rec *Record)
	}{
		{name: "cursor into missing file", check: func(t *testing.T, home string, agent agents.Agent, _ *Record) {
			data := mustRead(t, cursorPath(home))
			if keys := objectKeys(t, data); !slices.Equal(keys, []string{"version", "hooks"}) {
				t.Errorf("top-level keys = %v, want [version hooks]", keys)
			}
			if keys := objectKeys(t, hooksObject(t, data)); !slices.Equal(keys, agent.Install.Hooks.Events) {
				t.Errorf("events = %v, want %v", keys, agent.Install.Hooks.Events)
			}
			for event, handlers := range ourHandlers(t, data) {
				if len(handlers) != 1 {
					t.Errorf("%s has %d handlers of ours", event, len(handlers))
				}
			}
		}},
		{name: "cursor keeps other tools' entries and key order", user: userFile,
			check: func(t *testing.T, home string, agent agents.Agent, _ *Record) {
				data := mustRead(t, cursorPath(home))
				if keys := objectKeys(t, data); !slices.Equal(keys, []string{"hooks", "version"}) {
					t.Errorf("top-level keys = %v, want the user's order [hooks version]", keys)
				}
				before := objectKeys(t, hooksObject(t, userFile))
				after := objectKeys(t, hooksObject(t, data))
				if !slices.Equal(after[:len(before)], before) {
					t.Errorf("events = %v, want the user's %v first", after, before)
				}
				var original, updated struct {
					Hooks map[string][]map[string]any `json:"hooks"`
				}
				if json.Unmarshal(userFile, &original) != nil || json.Unmarshal(data, &updated) != nil {
					t.Fatal("decode")
				}
				for event, handlers := range original.Hooks {
					got := updated.Hooks[event]
					if len(got) < len(handlers) || !reflect.DeepEqual(got[:len(handlers)], handlers) {
						t.Errorf("%s: the user's handlers changed:\n got  %v\n want %v first", event, got, handlers)
					}
				}
			}},
		{name: "cursor commands carry no --event or --reply", user: userFile,
			check: func(t *testing.T, home string, agent agents.Agent, _ *Record) {
				for event, handlers := range ourHandlers(t, mustRead(t, cursorPath(home))) {
					command := handlers[0]["command"].(string)
					want := shellQuote(testBin) + " hook --tool cursor"
					if command != want {
						t.Errorf("%s command = %q, want %q", event, command, want)
					}
				}
			}},
		{name: "cursor uninstall restores the original after re-indent", user: userFile,
			check: func(t *testing.T, home string, agent agents.Agent, rec *Record) {
				if err := RemoveAgentHooks(home, agent, rec); err != nil {
					t.Fatal(err)
				}
				var indented bytes.Buffer
				if err := json.Indent(&indented, userFile, "", "  "); err != nil {
					t.Fatal(err)
				}
				if got := mustRead(t, cursorPath(home)); strings.TrimSpace(string(got)) != strings.TrimSpace(indented.String()) {
					t.Errorf("after uninstall:\n%s\nwant:\n%s", got, indented.String())
				}
			}},
		{name: "cursor keeps a file without a final newline", user: bytes.TrimRight(userFile, "\n"),
			check: func(t *testing.T, home string, agent agents.Agent, rec *Record) {
				if data := mustRead(t, cursorPath(home)); bytes.HasSuffix(data, []byte("\n")) {
					t.Error("install added a final newline")
				}
				if err := RemoveAgentHooks(home, agent, rec); err != nil {
					t.Fatal(err)
				}
				if got, want := mustRead(t, cursorPath(home)), bytes.TrimRight(userFile, "\n"); !bytes.Equal(got, want) {
					t.Errorf("after uninstall:\n%q\nwant byte for byte:\n%q", got, want)
				}
			}},
		{name: "cursor keeps the final newline of a file that has one", user: userFile,
			check: func(t *testing.T, home string, agent agents.Agent, rec *Record) {
				if data := mustRead(t, cursorPath(home)); !bytes.HasSuffix(data, []byte("}\n")) {
					t.Error("install dropped the final newline")
				}
				if err := RemoveAgentHooks(home, agent, rec); err != nil {
					t.Fatal(err)
				}
				if got := mustRead(t, cursorPath(home)); !bytes.Equal(got, userFile) {
					t.Errorf("after uninstall:\n%s\nwant byte for byte:\n%s", got, userFile)
				}
			}},
		{name: "cursor keeps a four-space indent and uninstall gives the file back", user: reindent(t, userFile, "    "),
			check: func(t *testing.T, home string, agent agents.Agent, rec *Record) {
				assertIndentKept(t, home, agent, rec, reindent(t, userFile, "    "), "    ")
			}},
		{name: "cursor keeps a tab indent and uninstall gives the file back", user: reindent(t, userFile, "\t"),
			check: func(t *testing.T, home string, agent agents.Agent, rec *Record) {
				assertIndentKept(t, home, agent, rec, reindent(t, userFile, "\t"), "\t")
			}},
		{name: "cursor uninstall of a file we created removes the file",
			check: func(t *testing.T, home string, agent agents.Agent, rec *Record) {
				if err := RemoveAgentHooks(home, agent, rec); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(cursorPath(home)); !os.IsNotExist(err) {
					t.Errorf("the hook file is still there (%v)", err)
				}
			}},
		{name: "cursor handler has timeout and no failClosed", user: userFile,
			check: func(t *testing.T, home string, agent agents.Agent, _ *Record) {
				ours := ourHandlers(t, mustRead(t, cursorPath(home)))
				if len(ours) != len(agent.Install.Hooks.Events) {
					t.Errorf("our events = %d, want %d", len(ours), len(agent.Install.Hooks.Events))
				}
				for event, handlers := range ours {
					handler := handlers[0]
					if _, found := handler["failClosed"]; found || handler["timeout"] != float64(5) || handler["type"] != "command" {
						t.Errorf("%s handler = %v", event, handler)
					}
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.user != nil {
				writeFile(t, cursorPath(home), tt.user)
			}
			agent := cursorAgent(t)
			rec := Record{Version: RecordVersion}

			addAgent(t, home, agent, &rec)

			tt.check(t, home, agent, &rec)
		})
	}
}

// reindent writes data with the given indent per level and a final newline.
func reindent(t *testing.T, data []byte, indent string) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(data), "", indent); err != nil {
		t.Fatal(err)
	}
	out.WriteByte('\n')
	return out.Bytes()
}

// assertIndentKept checks that the installed file uses indent, and that
// uninstall gives back original byte for byte.
func assertIndentKept(t *testing.T, home string, agent agents.Agent, rec *Record, original []byte, indent string) {
	t.Helper()
	installed := mustRead(t, cursorPath(home))
	if !bytes.Equal(installed, reindent(t, installed, indent)) {
		t.Errorf("install changed the indent to:\n%s", installed)
	}
	if err := RemoveAgentHooks(home, agent, rec); err != nil {
		t.Fatal(err)
	}
	if got := mustRead(t, cursorPath(home)); !bytes.Equal(got, original) {
		t.Errorf("after uninstall:\n%s\nwant byte for byte:\n%s", got, original)
	}
}
