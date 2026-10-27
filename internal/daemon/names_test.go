package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/sessioninfo"
	"agent-screensaver/internal/spool"
)

// claudeTurn spools a started event of Claude session s1 with its
// transcript.
func claudeTurn(t *testing.T, p paths.Paths, transcript string, atMs int64) {
	t.Helper()
	started := event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindStarted, AtMs: atMs,
		Title: "the prompt", Transcript: transcript}
	if err := spool.Write(p.SpoolDir, started); err != nil {
		t.Fatal(err)
	}
}

func appendFiller(t *testing.T, path string, bytes int) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	line := `{"type":"user","sessionId":"s1","message":{"content":"` + strings.Repeat("x", 200) + `"}}` + "\n"
	for written := 0; written < bytes; written += len(line) {
		if _, err := file.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
}

func TestARestartKeepsANameThatIsOnlyBeyondTheTranscriptTail(t *testing.T) {
	// Given a Claude session whose name was read once
	first, p := newTestDaemon(t)
	installBuiltinAgents(t, p)
	first.Details = sessioninfo.Reader{Home: p.Home}
	transcript := filepath.Join(p.Home, ".claude", "projects", "app", "s1.jsonl")
	writeTestFile(t, transcript, []byte(`{"type":"ai-title","aiTitle":"Parser refactor","sessionId":"s1"}`+"\n"))
	claudeTurn(t, p, transcript, 1_000)
	first.Step(1_000, false)
	if cards := readStateFile(t, p).Cards; len(cards) != 1 || cards[0].Name != "Parser refactor" {
		t.Fatalf("first run cards = %+v", cards)
	}

	// When the transcript grows past the 256 KB tail and the service restarts
	appendFiller(t, transcript, 300<<10)
	second := New(p)
	second.Logf = t.Logf
	second.DetailsRunner = func(read func()) { read() }
	second.Details = sessioninfo.Reader{Home: p.Home}
	claudeTurn(t, p, transcript, 60_000)
	second.Step(60_000, false)

	// Then the card still has the name
	if cards := readStateFile(t, p).Cards; len(cards) != 1 || cards[0].Name != "Parser refactor" {
		t.Errorf("after restart cards = %+v, want the remembered name", cards)
	}
}

func TestPrivacyKeepsRememberedNamesOutOfTheStateFile(t *testing.T) {
	d, p := newTestDaemon(t)
	installBuiltinAgents(t, p)
	cfg := config.Default()
	cfg.Privacy = true
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	d.Details = sessioninfo.Reader{Home: p.Home}
	transcript := filepath.Join(p.Home, ".claude", "projects", "app", "s1.jsonl")
	writeTestFile(t, transcript, []byte(`{"type":"ai-title","aiTitle":"Secret plan","sessionId":"s1"}`+"\n"))
	claudeTurn(t, p, transcript, 1_000)

	d.Step(1_000, false)

	data, err := os.ReadFile(p.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Secret plan") {
		t.Errorf("state.json holds the name with privacy on:\n%s", data)
	}
}
