package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/event"
)

const testNowMs = 1_760_000_000_000

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
		{"claude started", "claude", "user_prompt_submit.json", true, event.KindStarted, "s1", "my-project", "Fix the login bug and add a regression…"},
		{"claude tool intent", "claude", "pre_tool_use.json", true, event.KindToolIntent, "s1", "my-project", "Bash: npm run migrate"},
		{"claude permission notification", "claude", "notification_permission.json", true, event.KindApproval, "s1", "my-project", ""},
		{"claude idle notification is ignored", "claude", "notification_idle.json", false, "", "", "", ""},
		{"claude permission request", "claude", "permission_request.json", true, event.KindApproval, "s1", "my-project", "Edit: /home/user/my-project/main.go"},
		{"claude resumed", "claude", "post_tool_use.json", true, event.KindResumed, "s1", "my-project", ""},
		{"claude finished", "claude", "stop.json", true, event.KindFinished, "s1", "my-project", ""},
		{"claude ended", "claude", "session_end.json", true, event.KindEnded, "s1", "my-project", ""},
		{"claude missing session is ignored", "claude", "missing_session.json", false, "", "", "", ""},
		{"claude unknown event is ignored", "claude", "unknown_event.json", false, "", "", "", ""},
		{"claude invalid json is ignored", "claude", "invalid.json", false, "", "", "", ""},
		{"codex started", "codex", "user_prompt_submit.json", true, event.KindStarted, "thr_1", "codex-app", "Refactor the parser"},
		{"codex tool intent joins command array", "codex", "pre_tool_use.json", true, event.KindToolIntent, "thr_1", "codex-app", "shell: git status"},
		{"codex permission request", "codex", "permission_request.json", true, event.KindApproval, "thr_1", "codex-app", "apply_patch: src/parser.go"},
		{"codex resumed", "codex", "post_tool_use.json", true, event.KindResumed, "thr_1", "codex-app", ""},
		{"codex finished", "codex", "stop.json", true, event.KindFinished, "thr_1", "codex-app", ""},
		{"codex ended", "codex", "session_end.json", true, event.KindEnded, "thr_1", "codex-app", ""},
		{"codex session from thread_id", "codex", "thread_id_only.json", true, event.KindFinished, "th9", "codex-app", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, found := ByName(tt.adapter)
			if !found {
				t.Fatalf("adapter %q not registered", tt.adapter)
			}
			input, err := os.ReadFile(filepath.Join("testdata", tt.adapter, tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := adapter.Map(input, testNowMs)
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
			_, ok := Claude{}.Map([]byte(input), testNowMs)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			adapter, ok := ByName(name)
			if !ok || adapter.Name() != name {
				t.Errorf("ByName(%q) = %v, %v", name, adapter, ok)
			}
		})
	}
	t.Run("unknown", func(t *testing.T) {
		if _, ok := ByName("x"); ok {
			t.Error("ByName(\"x\") found an adapter")
		}
	})
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
