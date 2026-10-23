package sessioninfo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/state"
)

// builtinDetails are the details of a built-in agent of the repo.
func builtinDetails(t *testing.T, source string) *agents.Details {
	t.Helper()
	loaded, _ := agents.Load(filepath.Join("..", "..", "extensions"))
	agent, ok := agents.BySource(loaded, source)
	if !ok {
		t.Fatalf("no built-in agent %s", source)
	}
	return agent.Details
}

// testDetails loads an agent whose details are the given JSON object.
// script, when set, becomes bin/details.
func testDetails(t *testing.T, details, script string) *agents.Details {
	t.Helper()
	extDir := t.TempDir()
	dir := filepath.Join(extDir, "acme.agent")
	writeFile(t, filepath.Join(dir, "mark.svg"), `<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	writeFile(t, filepath.Join(dir, "extension.json"), `{"id":"acme.agent","name":"Acme","version":"1.0.0","type":"agent","api":1,
		"agent":{"source":"acme","mark":"mark.svg","hooks":{"event":"e","session":"id","rules":[{"kind":"finished"}]},
		"install":{"hooks":{"file":"~/.acme/hooks.json","format":"json","shape":"flat","events":["stop"]}},
		"details":`+details+`}}`)
	if script != "" {
		writeFile(t, filepath.Join(dir, "bin", "details"), script)
		if err := os.Chmod(filepath.Join(dir, "bin", "details"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := agents.Load(extDir)
	if len(loaded) != 1 {
		t.Fatalf("test agent did not load: %v", err)
	}
	return loaded[0].Details
}

// homeFile copies a fixture into the temp home and returns its path.
func homeFile(t *testing.T, home, fixtureName, relative string) string {
	t.Helper()
	data, err := os.ReadFile(fixture(fixtureName))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, relative)
	writeFile(t, path, string(data))
	return path
}

func TestDetails(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	homeFile(t, home, "codex_session_index.jsonl", ".codex/session_index.jsonl")
	rollout := homeFile(t, home, "codex_rollout.jsonl", ".codex/sessions/rollout.jsonl")
	noWindow := homeFile(t, home, "codex_rollout_no_window.jsonl", ".codex/sessions/no-window.jsonl")
	transcript := homeFile(t, home, "claude_transcript.jsonl", ".claude/projects/p/t.jsonl")
	renamed := homeFile(t, home, "claude_transcript_renamed.jsonl", ".claude/projects/p/renamed.jsonl")
	untitled := homeFile(t, home, "claude_transcript_untitled.jsonl", ".claude/projects/p/untitled.jsonl")
	outsideFile := filepath.Join(outside, "t.jsonl")
	writeFile(t, outsideFile, `{"type":"ai-title","aiTitle":"Outside","sessionId":"`+claudeSession+`"}`+"\n")
	linked := filepath.Join(home, ".claude", "projects", "p", "linked.jsonl")
	if err := os.Symlink(outsideFile, linked); err != nil {
		t.Fatal(err)
	}
	// The first line ends with a fake title that a 1 KiB tail would start
	// with; the reader must drop that partial line.
	fake := `{"type":"ai-title","aiTitle":"Cut in half","sessionId":"s1"}`
	partial := filepath.Join(home, "partial.jsonl")
	rest := 1024 - len(fake) - 1 // the bytes after the fake title's line
	other := `{"type":"other"}` + "\n"
	writeFile(t, partial, strings.Repeat("x", 3000)+fake+"\n"+strings.Repeat(other, (rest-1)/len(other))+
		strings.Repeat(" ", (rest-1)%len(other))+"\n")
	tail := `{"name":[{"jsonl":"~/partial.jsonl","tail_kb":1,"last":{"path":"type","equals":"ai-title"},"value":"aiTitle"}]}`

	claude, codex := builtinDetails(t, "claude"), builtinDetails(t, "codex")
	sizes := &ContextSizes{sizes: map[string]int{"claude-opus-5-5": 1_000_000}}
	reader := Reader{Home: home, ContextSizes: sizes}
	tests := []struct {
		name    string
		details *agents.Details
		card    state.Card
		want    state.Details
	}{
		{"codex name from session_index last matching line", codex,
			state.Card{Source: "codex", Session: codexSession}, state.Details{Name: "Parser refactor, part two"}},
		{"codex context from rollout tail ratio", codex,
			state.Card{Source: "codex", Session: "none", Transcript: rollout}, state.Details{ContextPercent: percent(77)}},
		{"codex rollout without a window size has no context", codex,
			state.Card{Source: "codex", Session: "none", Transcript: noWindow}, state.Details{}},
		{"claude name from custom-title first", claude,
			state.Card{Source: "claude", Session: claudeSession, Transcript: renamed}, state.Details{Name: "Login fix", Model: "Opus 5.5"}},
		{"claude name from ai-title, model and context", claude,
			state.Card{Source: "claude", Session: claudeSession, Transcript: transcript},
			state.Details{Name: "Fix the login session expiry", Model: "Opus 5.5", ContextPercent: percent(2)}},
		{"titles of other sessions are ignored", claude,
			state.Card{Source: "claude", Session: claudeSession, Transcript: untitled}, state.Details{Model: "Opus 5.5"}},
		{"path outside home refused", claude,
			state.Card{Source: "claude", Session: claudeSession, Transcript: outsideFile}, state.Details{}},
		{"symlink out of home refused", claude,
			state.Card{Source: "claude", Session: claudeSession, Transcript: linked}, state.Details{}},
		{"no transcript, no file to read", claude, state.Card{Source: "claude", Session: claudeSession}, state.Details{}},
		{"jsonl first partial line dropped", testDetails(t, tail, ""), state.Card{Source: "acme", Session: "s1"}, state.Details{}},
		{"no details read nothing but the branch", nil, state.Card{Source: "acme", Session: "s1", Transcript: transcript}, state.Details{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reader.Read(tt.details, tt.card)
			if got.Name != tt.want.Name || got.Model != tt.want.Model || got.Branch != tt.want.Branch || !equalPercent(got.ContextPercent, tt.want.ContextPercent) {
				t.Errorf("Read = %+v (context %v), want %+v (context %v)", got, show(got.ContextPercent), tt.want, show(tt.want.ContextPercent))
			}
		})
	}
}

func TestDetailsReadOnlyTheTail(t *testing.T) {
	home := t.TempDir()
	title := `{"type":"ai-title","aiTitle":"Early title","sessionId":"` + claudeSession + `"}` + "\n"
	filler := bytes.Repeat([]byte(`{"type":"assistant","message":{"content":"`+strings.Repeat("x", 1000)+`"}}`+"\n"), 10_000)
	early := filepath.Join(home, "early.jsonl")
	writeFile(t, early, title+string(filler))
	late := filepath.Join(home, "late.jsonl")
	writeFile(t, late, string(filler)+title)
	reader := Reader{Home: home}
	claude := builtinDetails(t, "claude")

	if got := reader.Read(claude, state.Card{Source: "claude", Session: claudeSession, Transcript: early}); got.Name != "" {
		t.Errorf("a title before the tail was found: %q", got.Name)
	}
	started := time.Now()
	got := reader.Read(claude, state.Card{Source: "claude", Session: claudeSession, Transcript: late})
	elapsed := time.Since(started)
	t.Logf("reading the tail of a 10 MiB file took %v", elapsed)
	if got.Name != "Early title" {
		t.Errorf("name = %q, want the title at the end", got.Name)
	}
	if elapsed > 50*time.Millisecond {
		t.Errorf("reading a 10 MiB file took %v, want under 50 ms", elapsed)
	}
}

func TestDetailsConverter(t *testing.T) {
	home := t.TempDir()
	// The programs are new files, and macOS checks a new program on its
	// first start, which can take most of a second; the timeouts allow it.
	tests := []struct {
		name     string
		script   string
		timeout  int
		want     state.Details
		maxTaken time.Duration
	}{
		{"output used", "#!/bin/sh\ncat >/dev/null\necho '{\"name\":\"From the program\",\"context_percent\":12,\"model\":\"claude-opus-5-5\"}'\n",
			2000, state.Details{Name: "From the program", Model: "Opus 5.5", ContextPercent: percent(12)}, 2500 * time.Millisecond},
		{"stdin holds the card", "#!/bin/sh\nname=$(cat | tr -d '\"{}:, ')\necho \"{\\\"name\\\":\\\"$name\\\"}\"\n",
			2000, state.Details{Name: "sessions1cwdtranscript"}, 2500 * time.Millisecond},
		{"timeout respected", "#!/bin/sh\nsleep 2\necho '{\"name\":\"late\"}'\n", 100, state.Details{}, 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := testDetails(t, fmt.Sprintf(`{"name":[{"converter":{"run":"bin/details","timeout_ms":%d}}],
				"context_percent":[{"converter":{"run":"bin/details","timeout_ms":%d}}],
				"model":[{"converter":{"run":"bin/details","timeout_ms":%d}}]}`, tt.timeout, tt.timeout, tt.timeout), tt.script)
			started := time.Now()

			got := Reader{Home: home}.Read(details, state.Card{Source: "acme", Session: "s1"})

			if elapsed := time.Since(started); elapsed > tt.maxTaken {
				t.Errorf("took %v, want under %v", elapsed, tt.maxTaken)
			}
			if got.Name != tt.want.Name || got.Model != tt.want.Model || !equalPercent(got.ContextPercent, tt.want.ContextPercent) {
				t.Errorf("Read = %+v (context %v), want %+v", got, show(got.ContextPercent), tt.want)
			}
		})
	}
}
