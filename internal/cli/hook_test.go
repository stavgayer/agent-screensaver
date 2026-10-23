package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
)

const testMark = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8" fill="#888"/></svg>`

// cursorLike is a test agent of the Cursor shape: a flat hook file, a
// reply per event and the event name from --event.
const cursorLike = `{
  "event": {"arg": "event"},
  "session": "conversation_id",
  "reply": {"default": "empty_object", "events": {"beforeShellExecution": "ask"}},
  "rules": [
    {"on": "beforeShellExecution", "kind": "approval", "set": {"title": {"path": "command", "as": "short_title"}}},
    {"on": "stop", "kind": "finished"}
  ]
}`

const flatInstall = `{"hooks": {"file": "~/.acme/hooks.json", "format": "json", "shape": "flat",
  "events": ["beforeShellExecution", "stop"]}}`

// writeAgent writes an agent extension folder into the extensions folder.
// hooks is the agent.hooks JSON; script, when set, becomes bin/convert.
func writeAgent(t *testing.T, p paths.Paths, id, source, hooks, script string) {
	t.Helper()
	dir := filepath.Join(p.ExtensionsDir, id)
	manifest := fmt.Sprintf(`{"id":%q,"name":"Acme","version":"1.0.0","type":"agent","api":1,
		"agent":{"source":%q,"mark":"mark.svg","hooks":%s,"install":%s}}`, id, source, hooks, flatInstall)
	writeTestFile(t, filepath.Join(dir, "extension.json"), manifest, 0o644)
	writeTestFile(t, filepath.Join(dir, "mark.svg"), testMark, 0o644)
	if script != "" {
		writeTestFile(t, filepath.Join(dir, "bin", "convert"), script, 0o755)
	}
}

// copyBuiltinAgent copies a built-in agent from the repo into the
// extensions folder.
func copyBuiltinAgent(t *testing.T, p paths.Paths, id string) {
	t.Helper()
	for _, name := range []string{"extension.json", "mark.svg"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "extensions", id, name))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(p.ExtensionsDir, id, name), string(data), 0o644)
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func setAgentsOff(t *testing.T, p paths.Paths, ids ...string) {
	t.Helper()
	cfg := config.Default()
	cfg.AgentsOff = ids
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
}

func fixedNow() int64 { return 1_760_000_000_000 }

func runHook(t *testing.T, p paths.Paths, payload string, args ...string) (string, int) {
	t.Helper()
	var stdout bytes.Buffer
	code := Hook(args, strings.NewReader(payload), &stdout, p, fixedNow)
	return stdout.String(), code
}

func errorLog(p paths.Paths) string {
	data, _ := os.ReadFile(filepath.Join(p.DataDir, errorLogName))
	return string(data)
}

func TestHookWritesTheMappedEvent(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	copyBuiltinAgent(t, p, "builtin.claude")
	payload := `{"session_id":"s1","hook_event_name":"UserPromptSubmit","cwd":"/work/app","prompt":"Fix the login bug"}`

	output, code := runHook(t, p, payload, "--tool", "claude")

	events := spool.Drain(p.SpoolDir)
	if code != 0 || output != "" || len(events) != 1 {
		t.Fatalf("code %d, output %q, events %+v, log %s", code, output, events, errorLog(p))
	}
	want := event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindStarted, AtMs: fixedNow(),
		Project: "app", Title: "Fix the login bug", Cwd: "/work/app"}
	if !reflect.DeepEqual(events[0], want) {
		t.Errorf("event = %+v, want %+v", events[0], want)
	}
}

func TestHookUnknownToolIsLogged(t *testing.T) {
	p := paths.ForHome(t.TempDir())

	output, code := runHook(t, p, `{}`, "--tool", "nobody")

	if code != 0 || output != "" || !strings.Contains(errorLog(p), `unknown tool "nobody"`) {
		t.Errorf("code %d, output %q, log %q", code, output, errorLog(p))
	}
}

func TestReply(t *testing.T) {
	sleeper := "#!/bin/sh\nsleep 2\n"
	tests := []struct {
		name       string
		builtin    string // a built-in agent of the repo instead of hooks
		hooks      string
		script     string
		off        bool
		fast       bool // the hook must return within 100 ms
		payload    string
		args       []string
		wantOutput string
		wantEvents int
	}{
		{name: "reply none prints nothing", hooks: `{"event":"e","session":"id","rules":[{"kind":"finished"}]}`,
			payload: `{"e":"stop","id":"s1"}`, wantOutput: "", wantEvents: 1},
		{name: "empty_object prints {}", hooks: cursorLike, payload: `{"conversation_id":"s1"}`,
			args: []string{"--event", "stop"}, wantOutput: "{}\n", wantEvents: 1},
		{name: "per-event ask prints ask for that event", hooks: cursorLike,
			payload: `{"conversation_id":"s1","command":"rm -rf build"}`, args: []string{"--event", "beforeShellExecution"},
			wantOutput: `{"permission":"ask"}` + "\n", wantEvents: 1},
		{name: "the default for an event without its own reply", hooks: cursorLike, payload: `{"conversation_id":"s1"}`,
			args: []string{"--event", "afterShellExecution"}, wantOutput: "{}\n"},
		{name: "reply comes from --event even when the payload is invalid JSON", hooks: cursorLike, payload: `{oops`,
			args: []string{"--event", "beforeShellExecution"}, wantOutput: `{"permission":"ask"}` + "\n"},
		{name: "reply printed when the agent is off", hooks: cursorLike, off: true,
			payload: `{"conversation_id":"s1"}`, args: []string{"--event", "beforeShellExecution"},
			wantOutput: `{"permission":"ask"}` + "\n"},
		{name: "reply printed when the converter times out", script: sleeper,
			hooks:   `{"event":{"arg":"event"},"reply":{"default":"ask"},"converter":{"run":"bin/convert","timeout_ms":100}}`,
			payload: `{}`, args: []string{"--event", "x"}, wantOutput: `{"permission":"ask"}` + "\n"},
		{name: "cursor beforeShellExecution prints ask", builtin: "builtin.cursor",
			payload: `{"conversation_id":"c1","hook_event_name":"beforeShellExecution","command":"npm run migrate"}`,
			args:    []string{"--event", "beforeShellExecution", "--reply", "ask"}, wantOutput: `{"permission":"ask"}` + "\n", wantEvents: 1},
		{name: "cursor beforeMCPExecution prints ask", builtin: "builtin.cursor",
			payload: `{"conversation_id":"c1","hook_event_name":"beforeMCPExecution","tool_name":"create_issue"}`,
			args:    []string{"--event", "beforeMCPExecution", "--reply", "ask"}, wantOutput: `{"permission":"ask"}` + "\n", wantEvents: 1},
		{name: "cursor stop prints the default (nothing)", builtin: "builtin.cursor",
			payload:    `{"conversation_id":"c1","hook_event_name":"stop","status":"completed"}`,
			wantOutput: "", wantEvents: 1},
		{name: "cursor permission event with garbage stdin still prints ask fast", builtin: "builtin.cursor", fast: true,
			payload: "\x00\xff not json {{{", args: []string{"--event", "beforeShellExecution", "--reply", "ask"},
			wantOutput: `{"permission":"ask"}` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := paths.ForHome(t.TempDir())
			tool := "acme"
			if tt.builtin != "" {
				copyBuiltinAgent(t, p, tt.builtin)
				tool = strings.TrimPrefix(tt.builtin, "builtin.")
			} else {
				writeAgent(t, p, "acme.cursorish", "acme", tt.hooks, tt.script)
			}
			if tt.off {
				setAgentsOff(t, p, "acme.cursorish")
			}

			started := time.Now()
			output, code := runHook(t, p, tt.payload, append([]string{"--tool", tool}, tt.args...)...)

			if elapsed := time.Since(started); tt.fast && elapsed > 100*time.Millisecond {
				t.Errorf("hook took %v, want under 100 ms", elapsed)
			}
			if code != 0 || output != tt.wantOutput {
				t.Errorf("code %d, output %q, want %q (log %s)", code, output, tt.wantOutput, errorLog(p))
			}
			if events := spool.Drain(p.SpoolDir); len(events) != tt.wantEvents {
				t.Errorf("events = %+v, want %d", events, tt.wantEvents)
			}
		})
	}
}

func TestHookReplyWhenTheManifestCannotBeRead(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"ask from the command", []string{"--event", "beforeShellExecution", "--reply", "ask"}, `{"permission":"ask"}` + "\n"},
		{"empty_object from the command", []string{"--reply", "empty_object"}, "{}\n"},
		{"no reply in the command prints nothing", nil, ""},
		{"a reply that is not one of ours prints nothing", []string{"--reply", "allow"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := paths.ForHome(t.TempDir())
			writeAgent(t, p, "acme.cursorish", "acme", cursorLike, "")
			writeTestFile(t, filepath.Join(p.ExtensionsDir, "acme.cursorish", "extension.json"), "{broken", 0o644)

			output, code := runHook(t, p, `{"conversation_id":"s1"}`, append([]string{"--tool", "acme"}, tt.args...)...)

			if code != 0 || output != tt.want {
				t.Errorf("code %d, output %q, want %q", code, output, tt.want)
			}
		})
	}
}

func TestHookOfAnAgentThatIsOffWritesNothing(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	copyBuiltinAgent(t, p, "builtin.claude")
	setAgentsOff(t, p, "builtin.claude")

	_, code := runHook(t, p, `{"session_id":"s1","hook_event_name":"Stop"}`, "--tool", "claude")

	if events := spool.Drain(p.SpoolDir); code != 0 || len(events) != 0 {
		t.Errorf("code %d, events %+v", code, events)
	}
}

func TestHookFastWithManyExtensions(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	copyBuiltinAgent(t, p, "builtin.claude")
	for index := range 40 {
		dir := filepath.Join(p.ExtensionsDir, fmt.Sprintf("acme.background-%d", index))
		writeTestFile(t, filepath.Join(dir, "extension.json"), fmt.Sprintf(
			`{"id":"acme.background-%d","name":"Rain","version":"1.0.0","type":"background","api":1,"entry":"main.js",
			"options":{"speed":{"type":"number","default":1,"min":0,"max":5}}}`, index), 0o644)
		writeTestFile(t, filepath.Join(dir, "main.js"), "// rain", 0o644)
	}
	for index := range 9 {
		writeAgent(t, p, fmt.Sprintf("acme.agent-%d", index), fmt.Sprintf("agent-%d", index), cursorLike, "")
	}
	payload, err := json.Marshal(map[string]any{"session_id": "s1", "hook_event_name": "UserPromptSubmit",
		"cwd": "/work/app", "prompt": strings.Repeat("Fix the login bug. ", 200)})
	if err != nil {
		t.Fatal(err)
	}
	runHook(t, p, string(payload), "--tool", "claude") // warm the file cache, as a real Mac has it
	start := time.Now()

	_, code := runHook(t, p, string(payload), "--tool", "claude")

	elapsed := time.Since(start)
	t.Logf("hook with 50 extensions took %v", elapsed)
	if code != 0 || elapsed > 100*time.Millisecond {
		t.Errorf("code %d, took %v, want under 100 ms", code, elapsed)
	}
	if events := spool.Drain(p.SpoolDir); len(events) != 2 {
		t.Errorf("events = %d, want 2 (log %s)", len(events), errorLog(p))
	}
}

func TestHookConverterTimeoutStillFast(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	writeAgent(t, p, "acme.slow", "acme", `{"event":{"arg":"event"},"converter":{"run":"bin/convert"}}`, "#!/bin/sh\nsleep 2\n")
	start := time.Now()

	_, code := runHook(t, p, `{}`, "--tool", "acme", "--event", "x")

	elapsed := time.Since(start)
	t.Logf("hook with a sleeping converter took %v", elapsed)
	if code != 0 || elapsed > 600*time.Millisecond {
		t.Errorf("code %d, took %v, want under 600 ms", code, elapsed)
	}
	if !strings.Contains(errorLog(p), "timed out") {
		t.Errorf("log = %q", errorLog(p))
	}
	if events := spool.Drain(p.SpoolDir); len(events) != 0 {
		t.Errorf("events = %+v", events)
	}
}

func TestRecoverToZero(t *testing.T) {
	p := paths.ForHome(t.TempDir())
	panics := func() (code int) {
		defer recoverToZero(p, fixedNow, &code)
		panic("boom")
	}

	if code := panics(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(errorLog(p), "panic: boom") {
		t.Errorf("error log = %q", errorLog(p))
	}
}
