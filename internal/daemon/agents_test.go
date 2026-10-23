package daemon

import (
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
)

func TestStateCarriesAgentMarks(t *testing.T) {
	d, p := newTestDaemon(t)
	installBuiltinAgents(t, p)
	// The user replaces the Codex mark with an icon of their own; a broken
	// one for Claude is ignored.
	writeTestFile(t, filepath.Join(p.MarksDir, "codex.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`))
	writeTestFile(t, filepath.Join(p.MarksDir, "claude.svg"), []byte(`<svg onload="x()"></svg>`))

	d.Step(1_000, false)

	agents := readStateFile(t, p).View.Agents
	claude, codex := agents["claude"], agents["codex"]
	if claude.Name != "Claude Code" || !strings.HasSuffix(claude.MarkURL, "/extensions/builtin.claude/mark.svg") {
		t.Errorf("claude = %+v", claude)
	}
	if codex.Name != "Codex" || !strings.HasSuffix(codex.MarkURL, "/marks/codex.svg") {
		t.Errorf("codex = %+v, want the user's own mark", codex)
	}
	if !strings.HasPrefix(claude.MarkURL, "file://") {
		t.Errorf("mark url %q is not a file URL", claude.MarkURL)
	}
}

func TestLastEventPerSource(t *testing.T) {
	d, p := newTestDaemon(t)
	cfg := config.Default()
	cfg.IgnoredProjects = []string{"secret-project"}
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 1_000)
	ignored := event.Event{V: event.Version, Source: "codex", Session: "s2", Kind: event.KindStarted, AtMs: 2_000, Project: "secret-project"}
	if err := spool.Write(p.SpoolDir, ignored); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(p.SpoolDir, "broken.json"), []byte(`{"v":1,"source":"jira","kind":"nope"}`))

	d.Step(3_000, false)

	sources := readStateFile(t, p).Sources
	want := map[string]state.Source{"claude": {LastEventMs: 1_000}, "codex": {LastEventMs: 2_000}}
	if len(sources) != 2 || sources["claude"] != want["claude"] || sources["codex"] != want["codex"] {
		t.Errorf("sources = %+v, want %+v (an ignored project counts, an invalid file does not)", sources, want)
	}

	restarted := New(p)
	restarted.Logf = t.Logf
	restarted.Step(4_000, false)

	if got := readStateFile(t, p).Sources; got["claude"] != want["claude"] || got["codex"] != want["codex"] {
		t.Errorf("after a restart sources = %+v", got)
	}
}

func TestEventsOfAnAgentThatIsOffAreDropped(t *testing.T) {
	d, p := newTestDaemon(t)
	installBuiltinAgents(t, p)
	cfg := config.Default()
	cfg.AgentsOff = []string{"builtin.codex"}
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	spoolEvent(t, p, "codex", "s1", event.KindApproval, 1_000)
	spoolEvent(t, p, "claude", "s2", event.KindStarted, 1_000)

	d.Step(1_000, false)

	file := readStateFile(t, p)
	if len(file.Cards) != 1 || file.Cards[0].Source != "claude" {
		t.Errorf("cards = %+v, want only the claude card", file.Cards)
	}
}
