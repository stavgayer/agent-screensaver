package agents

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
)

const (
	testNowMs  = 1_760_000_000_000
	testSource = "acme"
)

func intPointer(value int) *int           { return &value }
func floatPointer(value float64) *float64 { return &value }
func int64Pointer(value int64) *int64     { return &value }
func eventOf(kind event.Kind) event.Event { return withSession(kind, "s1") }
func withSession(kind event.Kind, session string) event.Event {
	return event.Event{V: event.Version, Source: testSource, Session: session, Kind: kind, AtMs: testNowMs}
}

// hookMapping builds a hook mapping with the usual event and session paths
// around the given rules (a JSON array) and common fields (a JSON object).
func hookMapping(t *testing.T, common, rules string) Mapping {
	t.Helper()
	if common == "" {
		common = "{}"
	}
	raw := `{"event":"hook_event_name","session":"session_id","common":` + common + `,"rules":` + rules + `}`
	mapping, err := parseMapping([]byte(raw), testSource, "", hookMappingKind)
	if err != nil {
		t.Fatalf("parse mapping: %v", err)
	}
	return mapping
}

func TestMapping(t *testing.T) {
	withTitle := func(kind event.Kind, title string) event.Event {
		mapped := eventOf(kind)
		mapped.Title = title
		return mapped
	}
	tests := []struct {
		name     string
		common   string
		rules    string
		payload  string
		eventArg string
		want     []event.Event // nil means the payload is ignored
	}{
		{name: "path", rules: `[{"on":"E","kind":"started","set":{"title":"tool_input.command"}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","tool_input":{"command":"ls -la"}}`,
			want:    []event.Event{withTitle(event.KindStarted, "ls -la")}},
		{name: "array index path", rules: `[{"on":"E","kind":"started","set":{"project":{"path":"workspace_roots.0","as":"basename"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","workspace_roots":["/work/first","/work/second"]}`,
			want: []event.Event{func() event.Event {
				mapped := eventOf(event.KindStarted)
				mapped.Project = "first"
				return mapped
			}()}},
		{name: "first non-empty of paths", rules: `[{"on":"E","kind":"started","set":{"title":{"path":["missing","empty","nothing","name"]}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","empty":"","nothing":null,"name":"third"}`,
			want:    []event.Event{withTitle(event.KindStarted, "third")}},
		{name: "const", rules: `[{"on":"E","kind":"started","set":{"title":{"const":"Working"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1"}`,
			want:    []event.Event{withTitle(event.KindStarted, "Working")}},
		{name: "prompt_title skips a system-reminder block", rules: `[{"on":"E","kind":"started","set":{"title":{"path":"prompt","as":"prompt_title"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","prompt":"<system-reminder>be nice</system-reminder>\nFix the login bug"}`,
			want:    []event.Event{withTitle(event.KindStarted, "Fix the login bug")}},
		{name: "short_title keeps the first line", rules: `[{"on":"E","kind":"started","set":{"title":{"path":"text","as":"short_title"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","text":"first   line\nsecond line"}`,
			want:    []event.Event{withTitle(event.KindStarted, "first line")}},
		{name: "model_name", rules: `[{"on":"E","kind":"started","set":{"model":{"path":"model","as":"model_name"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","model":"claude-opus-5-5[1m]"}`,
			want: []event.Event{func() event.Event {
				mapped := eventOf(event.KindStarted)
				mapped.Model = "Opus 5.5"
				return mapped
			}()}},
		{name: "count of background_tasks", rules: `[{"on":"E","kind":"finished","set":{"background":{"path":"background_tasks","as":"count"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","background_tasks":[{"id":"a"},{"id":"b"}]}`,
			want: []event.Event{func() event.Event {
				mapped := eventOf(event.KindFinished)
				mapped.Background = intPointer(2)
				return mapped
			}()}},
		{name: "count of no array gives no value", rules: `[{"on":"E","kind":"finished","set":{"background":{"path":"background_tasks","as":"count"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","background_tasks":"many"}`,
			want:    []event.Event{eventOf(event.KindFinished)}},
		{name: "percent clamps 130 to 100", rules: `[{"on":"E","kind":"info","set":{"context_percent":{"path":"used","as":"percent"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","used":130}`,
			want: []event.Event{func() event.Event {
				mapped := eventOf(event.KindInfo)
				mapped.ContextPercent = floatPointer(100)
				return mapped
			}()}},
		{name: "text joins a list", rules: `[{"on":"E","kind":"tool_intent","set":{"title":{"path":"command","as":"text"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","command":["git","status"]}`,
			want:    []event.Event{withTitle(event.KindToolIntent, "git status")}},
		{name: "tool value", rules: `[{"on":"E","kind":"approval","set":{"title":{"tool":"tool_name","input":"tool_input"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","tool_name":"Bash","tool_input":{"command":"npm run migrate"}}`,
			want:    []event.Event{withTitle(event.KindApproval, "Bash: npm run migrate")}},
		{name: "equals", rules: `[{"on":"E","if":{"path":"type","equals":"permission_prompt"},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","type":"permission_prompt"}`,
			want:    []event.Event{eventOf(event.KindApproval)}},
		{name: "equals a number", rules: `[{"on":"E","if":{"path":"count","equals":2},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","count":2}`,
			want:    []event.Event{eventOf(event.KindApproval)}},
		{name: "equals null when the key is null", rules: `[{"if":{"path":"transcript_path","equals":null},"kind":"ignore"},{"kind":"finished"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","transcript_path":null}`},
		{name: "equals null is false when the key is missing", rules: `[{"if":{"path":"transcript_path","equals":null},"kind":"ignore"},{"kind":"finished"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1"}`,
			want:    []event.Event{eventOf(event.KindFinished)}},
		{name: "in", rules: `[{"on":"E","if":{"path":"tool","in":["Bash","Shell"]},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","tool":"Shell"}`,
			want:    []event.Event{eventOf(event.KindApproval)}},
		{name: "in without a match", rules: `[{"on":"E","if":{"path":"tool","in":["Bash","Shell"]},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","tool":"Edit"}`},
		{name: "contains ignores case", rules: `[{"on":"E","if":{"path":"message","contains":"permission"},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","message":"Claude needs your PERMISSION"}`,
			want:    []event.Event{eventOf(event.KindApproval)}},
		{name: "present true", rules: `[{"on":"E","if":{"path":"type","present":true},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","type":""}`,
			want:    []event.Event{eventOf(event.KindApproval)}},
		{name: "present true is false for null", rules: `[{"on":"E","if":{"path":"type","present":true},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","type":null}`},
		{name: "present false", rules: `[{"on":"E","if":{"path":"type","present":false},"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1"}`,
			want:    []event.Event{eventOf(event.KindApproval)}},
		{name: "all conditions of a list must hold", rules: `[{"on":"E","if":[{"path":"a","equals":1},{"path":"b","equals":2}],"kind":"approval"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","a":1,"b":3}`},
		{name: "first matching rule wins", rules: `[{"on":"E","kind":"started"},{"on":"E","kind":"finished"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1"}`,
			want:    []event.Event{eventOf(event.KindStarted)}},
		{name: "on with a list of names", rules: `[{"on":["A","E"],"kind":"resumed"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1"}`,
			want:    []event.Event{eventOf(event.KindResumed)}},
		{name: "no rule matches", rules: `[{"on":"Other","kind":"started"}]`,
			payload: `{"hook_event_name":"E","session_id":"s1"}`},
		{name: "missing session", rules: `[{"on":"E","kind":"started"}]`,
			payload: `{"hook_event_name":"E"}`},
		{name: "invalid json", rules: `[{"kind":"started"}]`, payload: `{nope`},
		{name: "rule without on matches every event", rules: `[{"kind":"resumed"}]`,
			payload: `{"hook_event_name":"Anything","session_id":"s1"}`,
			want:    []event.Event{eventOf(event.KindResumed)}},
		{name: "common fields come first and the rule's own set wins", common: `{"title":{"const":"common"},"cwd":"cwd"}`,
			rules:   `[{"on":"E","kind":"started","set":{"title":{"const":"own"}}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","cwd":"/work/app"}`,
			want: []event.Event{func() event.Event {
				mapped := withTitle(event.KindStarted, "own")
				mapped.Cwd = "/work/app"
				return mapped
			}()}},
		{name: "secret in title is masked even without a transform", rules: `[{"on":"E","kind":"started","set":{"title":"text"}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","text":"export API_TOKEN=hunter2hunter2"}`,
			want:    []event.Event{withTitle(event.KindStarted, "export API_TOKEN=•••")}},
		{name: "info with nothing to say is dropped", rules: `[{"on":"E","kind":"info","set":{"name":"name"}}]`,
			payload: `{"hook_event_name":"E","session_id":"s1","cwd":"/a"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := hookMapping(t, tt.common, tt.rules)

			got, ok := mapping.Map([]byte(tt.payload), tt.eventArg, testNowMs)

			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v, events %+v", ok, got)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("events:\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}

func TestMappingEventFromArg(t *testing.T) {
	raw := `{"event":{"arg":"event"},"session":"id","rules":[{"on":"session.idle","kind":"finished"}]}`
	mapping, err := parseMapping([]byte(raw), testSource, "", hookMappingKind)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := mapping.Map([]byte(`{"id":"s1"}`), "session.idle", testNowMs)

	if !ok || len(got) != 1 || got[0].Kind != event.KindFinished {
		t.Errorf("got %+v, %v", got, ok)
	}
	if !mapping.UsesEventArg() {
		t.Error("UsesEventArg = false")
	}
}

func TestStatuslineMapping(t *testing.T) {
	raw := `{"session":"session_id","common":{"project":{"path":"cwd","as":"basename"}},"rules":[
		{"kind":"limits","set":{"limits":[
			{"window":"5h","used_percent":{"path":"five.used","as":"percent"},"resets_at":"five.resets"},
			{"window":"week","used_percent":{"path":"week.used","as":"percent"},"resets_at":"week.resets"}]}},
		{"kind":"info","set":{"name":"name"}}]}`
	mapping, err := parseMapping([]byte(raw), testSource, "", statuslineMappingKind)
	if err != nil {
		t.Fatal(err)
	}
	limits := func(readings ...event.LimitReading) event.Event {
		mapped := eventOf(event.KindLimits)
		mapped.Project = "app"
		mapped.Limits = readings
		return mapped
	}
	info := eventOf(event.KindInfo)
	info.Project, info.Name = "app", "Login fix"
	tests := []struct {
		name    string
		payload string
		want    []event.Event
	}{
		{"limits and info from one payload",
			`{"session_id":"s1","cwd":"/w/app","name":"Login fix","five":{"used":20,"resets":1738425600},"week":{"used":130}}`,
			[]event.Event{limits(
				event.LimitReading{Window: event.FiveHour, UsedPercent: 20, ResetsAt: int64Pointer(1738425600)},
				event.LimitReading{Window: event.Week, UsedPercent: 100}), info}},
		{"limits reading without used_percent dropped",
			`{"session_id":"s1","cwd":"/w/app","five":{"resets":1},"week":{"used":10}}`,
			[]event.Event{limits(event.LimitReading{Window: event.Week, UsedPercent: 10})}},
		{"limits without a session use the limits session",
			`{"cwd":"/w/app","five":{"used":1}}`,
			[]event.Event{func() event.Event {
				mapped := limits(event.LimitReading{Window: event.FiveHour, UsedPercent: 1})
				mapped.Session = LimitsSession
				return mapped
			}()}},
		{"nothing to say", `{"session_id":"s1","cwd":"/w/app"}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mapping.Map([]byte(tt.payload), "", testNowMs)
			if ok != (tt.want != nil) || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("events (ok %v):\n got  %+v\n want %+v", ok, got, tt.want)
			}
		})
	}
}

func TestReplyText(t *testing.T) {
	reply := Reply{Default: ReplyNone, Events: map[string]string{"beforeShellExecution": ReplyEmptyObject}}
	tests := []struct {
		name     string
		reply    Reply
		eventArg string
		want     string
	}{
		{"none prints nothing", Reply{}, "", ""},
		{"empty_object prints {}", Reply{Default: ReplyEmptyObject}, "", "{}\n"},
		{"per-event empty_object for that event", reply, "beforeShellExecution", "{}\n"},
		{"the default for other events", reply, "stop", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.reply.Text(tt.eventArg); got != tt.want {
				t.Errorf("Text(%q) = %q, want %q", tt.eventArg, got, tt.want)
			}
		})
	}
	if !reply.HasOwn("beforeShellExecution") || reply.HasOwn("stop") {
		t.Error("HasOwn does not match the events list")
	}
	if strings.Contains(reply.Text("x"), "allow") {
		t.Error("a reply allowed an action")
	}
	for _, old := range []string{"ask", "allow", "deny"} {
		if text, valid := ReplyText(old); valid || text != "" {
			t.Errorf("ReplyText(%q) = %q, %v; want nothing and invalid", old, text, valid)
		}
	}
}

// TestMappingCursor maps the recorded Cursor payloads with the built-in
// Cursor manifest.
func TestMappingCursor(t *testing.T) {
	const session = "22222222-2222-4222-8222-222222222222"
	// Cursor sends a transcript path only on stop, and the full model name
	// (with the effort) only on beforeSubmitPrompt and stop.
	transcript := "/Users/example/.cursor/projects/Users-example-work-app/agent-transcripts/" + session + "/" + session + ".jsonl"
	finished := func(e *event.Event) { e.Model, e.Transcript = "grok-4.7-high", transcript }
	cursorEvent := func(kind event.Kind, change func(*event.Event)) *event.Event {
		mapped := event.Event{V: event.Version, Source: "cursor", Session: session, Kind: kind, AtMs: testNowMs, Project: "app"}
		if change != nil {
			change(&mapped)
		}
		return &mapped
	}
	tests := []struct {
		name    string
		fixture string
		want    *event.Event // nil means the payload is ignored
	}{
		{"beforeSubmitPrompt is started with the prompt as title", "before_submit_prompt.json",
			cursorEvent(event.KindStarted, func(e *event.Event) { e.Title, e.Model = "Fix the login bug", "grok-4.7-high" })},
		// Owner decision 24/10: Cursor sends these before every command, also
		// when it will not ask, so they are working steps, never approvals.
		{"beforeShellExecution is a working step with the command as title", "before_shell_execution.json",
			cursorEvent(event.KindToolIntent, func(e *event.Event) { e.Title = "npm run migrate" })},
		{"beforeMCPExecution is a working step with the tool name as title", "before_mcp_execution.json",
			cursorEvent(event.KindToolIntent, func(e *event.Event) { e.Title = "create_issue" })},
		{"afterShellExecution is resumed", "after_shell_execution.json", cursorEvent(event.KindResumed, nil)},
		{"afterMCPExecution is resumed", "after_mcp_execution.json", cursorEvent(event.KindResumed, nil)},
		{"postToolUse is resumed", "post_tool_use.json", cursorEvent(event.KindResumed, nil)},
		{"stop is finished", "stop.json", cursorEvent(event.KindFinished, finished)},
		{"stop with status aborted is finished", "stop_aborted.json", cursorEvent(event.KindFinished, finished)},
		{"sessionEnd is ended", "session_end.json", cursorEvent(event.KindEnded, nil)},
		{"preCompact is info with context_percent", "pre_compact.json",
			cursorEvent(event.KindInfo, func(e *event.Event) { e.ContextPercent = floatPointer(87) })},
		{"afterAgentResponse is ignored", "after_agent_response.json", nil},
		{"missing conversation_id is ignored", "missing_conversation.json", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mapHook(t, "cursor", readTestFile(t, filepath.Join("testdata", "cursor", tt.fixture)))

			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v, event %+v", ok, got)
			}
			if tt.want != nil && !reflect.DeepEqual(got, *tt.want) {
				t.Errorf("event:\n got  %+v\n want %+v", got, *tt.want)
			}
		})
	}
}

// TestCursorNeverAsksForAttention: no Cursor payload makes an approval card,
// because Cursor does not say whether it will really ask the user.
func TestCursorNeverAsksForAttention(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("testdata", "cursor", "*.json"))
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no Cursor fixtures (%v)", err)
	}
	for _, fixture := range fixtures {
		if got, ok := mapHook(t, "cursor", readTestFile(t, fixture)); ok && got.Kind == event.KindApproval {
			t.Errorf("%s maps to an approval", filepath.Base(fixture))
		}
	}
}

// TestCursorShellCommandMakesAWorkingCard follows a Cursor turn into the
// card state: the command shows on a working card, which never asks for
// attention.
func TestCursorShellCommandMakesAWorkingCard(t *testing.T) {
	var cards state.State
	for _, fixture := range []string{"before_submit_prompt.json", "before_shell_execution.json"} {
		mapped, ok := mapHook(t, "cursor", readTestFile(t, filepath.Join("testdata", "cursor", fixture)))
		if !ok {
			t.Fatalf("%s gave no event", fixture)
		}
		cards.Apply(mapped)
	}

	if len(cards.Cards) != 1 {
		t.Fatalf("cards = %+v, want one", cards.Cards)
	}
	card := cards.Cards[0]
	if card.Kind != state.KindRunning || card.Request != "npm run migrate" || cards.NeedsAttention() {
		t.Errorf("card = %+v, needs attention %v; want running with the command and no attention", card, cards.NeedsAttention())
	}
}

// TestCursorHookNeverAnswersForTheUser: our hook prints nothing to Cursor.
// On 24/10/2026 (Cursor 3.27.12, run mode "Run Everything") our
// {"permission":"ask"} on beforeShellExecution made Cursor wait 49 minutes
// for an approval it would never have asked for, while the card said
// Working. With no output Cursor decides as it would without us.
func TestCursorHookNeverAnswersForTheUser(t *testing.T) {
	agent := builtinAgent(t, "cursor")
	for _, name := range append(slices.Clone(agent.Install.Hooks.Events), "") {
		if got := agent.Hooks.Reply.Text(name); got != "" {
			t.Errorf("reply to %q = %q, want nothing", name, got)
		}
		if agent.Hooks.Reply.HasOwn(name) {
			t.Errorf("%q has its own reply, so its command would carry --reply", name)
		}
	}
}

// TestCursorTurnWithAShellCommandEndsDone replays the hook order Cursor
// logged for a turn with one shell command (24/10/2026): the card is done
// after stop, and Cursor's later steps of the same turn come before it.
func TestCursorTurnWithAShellCommandEndsDone(t *testing.T) {
	var cards state.State
	steps := []string{"before_submit_prompt.json", "before_shell_execution.json", "after_shell_execution.json", "post_tool_use.json", "stop.json"}
	for index, fixture := range steps {
		mapped, ok := mapHook(t, "cursor", readTestFile(t, filepath.Join("testdata", "cursor", fixture)))
		if !ok {
			t.Fatalf("%s gave no event", fixture)
		}
		mapped.AtMs = testNowMs + int64(index)*1000
		cards.Apply(mapped)
	}
	if len(cards.Cards) != 1 || cards.Cards[0].Kind != state.KindFinished {
		t.Fatalf("cards = %+v, want one finished card", cards.Cards)
	}
}
