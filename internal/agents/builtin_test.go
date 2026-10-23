package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/event"
)

const (
	claudeSession = "11111111-1111-4111-8111-111111111111"
	codexSession  = "33333333-3333-4333-8333-333333333333"
)

// builtinAgent is a built-in agent of the repo's extensions folder.
func builtinAgent(t *testing.T, source string) Agent {
	t.Helper()
	loaded, _ := Load(builtinDir)
	agent, ok := BySource(loaded, source)
	if !ok {
		t.Fatalf("no built-in agent for %s", source)
	}
	return agent
}

// mapHook maps a hook payload with a built-in manifest. A hook payload
// gives at most one event.
func mapHook(t *testing.T, source string, input []byte) (event.Event, bool) {
	t.Helper()
	events, ok := builtinAgent(t, source).Hooks.Map(input, "", testNowMs)
	if len(events) > 1 {
		t.Fatalf("one payload gave %d events", len(events))
	}
	if !ok {
		return event.Event{}, false
	}
	return events[0], true
}

func TestMap(t *testing.T) {
	tests := []struct {
		name        string
		adapter     string
		fixture     string
		wantOK      bool
		wantKind    event.Kind
		wantSession string
		wantProject string
		wantTitle   string
	}{
		{"claude started", "claude", "user_prompt_submit.json", true, event.KindStarted, claudeSession, "workspace", "Fix the login bug and add a regression…"},
		{"claude tool intent", "claude", "pre_tool_use.json", true, event.KindToolIntent, claudeSession, "workspace", "Bash: npm run migrate"},
		{"claude permission notification", "claude", "notification_permission.json", true, event.KindApproval, "s1", "my-project", ""},
		{"claude idle notification is ignored", "claude", "notification_idle.json", false, "", "", "", ""},
		{"claude permission request", "claude", "permission_request.json", true, event.KindApproval, claudeSession, "workspace", "Bash: printf 'hi\\n' > /tmp/hello.txt &&…"},
		{"claude edit permission request", "claude", "permission_request_edit.json", true, event.KindApproval, "s1", "my-project", "Edit: /home/user/my-project/main.go"},
		{"claude resumed", "claude", "post_tool_use.json", true, event.KindResumed, claudeSession, "workspace", ""},
		{"claude finished", "claude", "stop.json", true, event.KindFinished, claudeSession, "workspace", ""},
		{"claude ended", "claude", "session_end.json", true, event.KindEnded, claudeSession, "workspace", ""},
		{"claude missing session is ignored", "claude", "missing_session.json", false, "", "", "", ""},
		{"claude unknown event is ignored", "claude", "unknown_event.json", false, "", "", "", ""},
		{"claude invalid json is ignored", "claude", "invalid.json", false, "", "", "", ""},
		{"codex started", "codex", "user_prompt_submit.json", true, event.KindStarted, codexSession, "workspace", "Refactor the parser"},
		{"codex tool intent", "codex", "pre_tool_use.json", true, event.KindToolIntent, codexSession, "workspace", "Bash: git status"},
		{"codex tool intent joins command array", "codex", "pre_tool_use_command_array.json", true, event.KindToolIntent, "thr_1", "codex-app", "shell: git status"},
		{"codex permission request", "codex", "permission_request.json", true, event.KindApproval, codexSession, "workspace", "Bash: touch /tmp/hello3.txt"},
		{"codex apply_patch permission request", "codex", "permission_request_apply_patch.json", true, event.KindApproval, "thr_1", "codex-app", "apply_patch: src/parser.go"},
		{"codex resumed", "codex", "post_tool_use.json", true, event.KindResumed, codexSession, "workspace", ""},
		{"codex resumed after mcp tool", "codex", "post_tool_use_mcp.json", true, event.KindResumed, codexSession, "workspace", ""},
		{"codex finished", "codex", "stop.json", true, event.KindFinished, codexSession, "workspace", ""},
		{"codex ended", "codex", "session_end.json", true, event.KindEnded, codexSession, "workspace", ""},
		{"codex session from thread_id", "codex", "thread_id_only.json", true, event.KindFinished, "th9", "codex-app", ""},
		{"codex helper prompt is ignored", "codex", "helper_user_prompt_submit.json", false, "", "", "", ""},
		{"codex helper stop is ignored", "codex", "helper_stop.json", false, "", "", "", ""},
		{"codex helper session end is ignored", "codex", "helper_session_end.json", false, "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", tt.adapter, tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := mapHook(t, tt.adapter, input)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			want := event.Event{V: event.Version, Source: tt.adapter, Session: tt.wantSession, Kind: tt.wantKind, AtMs: testNowMs, Project: tt.wantProject, Title: tt.wantTitle}
			if got.V != want.V || got.Source != want.Source || got.Session != want.Session || got.Kind != want.Kind ||
				got.AtMs != want.AtMs || got.Project != want.Project || got.Title != want.Title {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

func TestClaudeNotificationWithoutType(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantOK  bool
	}{
		{"message mentions permission", `Claude needs your Permission to use Bash`, true},
		{"message is unrelated", `Claude is waiting for your input`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"session_id":"s1","hook_event_name":"Notification","message":"` + tt.message + `"}`
			_, ok := mapHook(t, "claude", []byte(input))
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestDescribeTool(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  string
		check func(string) bool
	}{
		{name: "shell command", tool: "Bash", input: map[string]any{"command": "npm run migrate"}, want: "Bash: npm run migrate"},
		{name: "edit file path", tool: "Edit", input: map[string]any{"file_path": "a.go"}, want: "Edit: a.go"},
		{name: "write falls back to path", tool: "Write", input: map[string]any{"path": "b.go"}, want: "Write: b.go"},
		{name: "unknown tool is name only", tool: "WebSearch", input: map[string]any{"query": "x"}, want: "WebSearch"},
		{name: "nil input", tool: "Bash", input: nil, want: "Bash"},
		{name: "secret is masked", tool: "Bash", input: map[string]any{"command": "export API_TOKEN=hunter2hunter2"},
			check: func(got string) bool { return !strings.Contains(got, "hunter2") && strings.HasPrefix(got, "Bash: ") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DescribeTool(tt.tool, tt.input)
			if tt.check != nil {
				if !tt.check(got) {
					t.Errorf("DescribeTool = %q", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("DescribeTool = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPromptWithOnlyInjectedTextHasNoTitle(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{"tag block", "<task-notification>", ""},
		{"tag block then prompt", "<system-reminder>x</system-reminder>\nFix it", "Fix it"},
		{"attached files", `@"/Users/me/IMG_7324.HEIC"`, "IMG_7324.HEIC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"hook_event_name": "UserPromptSubmit", "session_id": "s1", "prompt": tt.prompt})
			if err != nil {
				t.Fatal(err)
			}
			got, ok := mapHook(t, "claude", input)
			if !ok || got.Title != tt.want {
				t.Errorf("got %q (ok=%v), want %q", got.Title, ok, tt.want)
			}
		})
	}
}

func TestBackgroundCount(t *testing.T) {
	two, zero := 2, 0
	tests := []struct {
		name    string
		adapter string
		fixture string
		want    *int
	}{
		{"claude stop with two background tasks", "claude", "stop_background.json", &two},
		{"claude stop with none in flight", "claude", "stop.json", &zero},
		{"claude stop with a broken list has no count", "claude", "stop_background_invalid.json", nil},
		{"claude payload without the field has no count", "claude", "post_tool_use.json", nil},
		{"claude prompt has no count", "claude", "user_prompt_submit.json", nil},
		{"codex never has a count", "codex", "stop.json", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", tt.adapter, tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := mapHook(t, tt.adapter, input)
			if !ok {
				t.Fatal("event was ignored")
			}
			switch {
			case tt.want == nil && got.Background != nil:
				t.Errorf("Background = %d, want none", *got.Background)
			case tt.want != nil && (got.Background == nil || *got.Background != *tt.want):
				t.Errorf("Background = %v, want %d", got.Background, *tt.want)
			}
		})
	}
}

func TestSessionDetails(t *testing.T) {
	const claudeTranscript = "/Users/example/.claude/projects/-Users-example-workspace/11111111-1111-4111-8111-111111111111.jsonl"
	const codexRollout = "/Users/example/.codex/sessions/2026/10/22/rollout-2026-10-22T18-09-57-33333333-3333-4333-8333-333333333333.jsonl"
	tests := []struct {
		name    string
		adapter string
		input   string // a fixture file name, or inline JSON when it starts with {
		want    event.Event
	}{
		{"claude prompt carries places", "claude", "user_prompt_submit.json",
			event.Event{Cwd: "/Users/example/workspace", Transcript: claudeTranscript}},
		{"claude session title is the name", "claude",
			`{"session_id":"s1","hook_event_name":"UserPromptSubmit","prompt":"hi","session_title":"Login   fix"}`,
			event.Event{Name: "Login fix"}},
		{"claude model is shortened", "claude",
			`{"session_id":"s1","hook_event_name":"Stop","model":"claude-opus-5-5[1m]"}`,
			event.Event{Model: "Opus 5.5"}},
		{"codex prompt carries places and model", "codex", "user_prompt_submit.json",
			event.Event{Cwd: "/Users/example/workspace", Transcript: codexRollout, Model: "gpt-example"}},
		{"codex stop carries the model", "codex", "stop.json",
			event.Event{Cwd: "/Users/example/workspace", Transcript: codexRollout, Model: "gpt-example"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(tt.input)
			if !strings.HasPrefix(tt.input, "{") {
				var err error
				if input, err = os.ReadFile(filepath.Join("testdata", tt.adapter, tt.input)); err != nil {
					t.Fatal(err)
				}
			}
			got, ok := mapHook(t, tt.adapter, input)
			if !ok {
				t.Fatal("event was ignored")
			}
			if got.Name != tt.want.Name || got.Model != tt.want.Model || got.Cwd != tt.want.Cwd || got.Transcript != tt.want.Transcript {
				t.Errorf("got name %q model %q cwd %q transcript %q, want %+v", got.Name, got.Model, got.Cwd, got.Transcript, tt.want)
			}
		})
	}
}
