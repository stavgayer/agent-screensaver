package daemon

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
)

// fakeDetails answers every read with the same details and counts the reads.
type fakeDetails struct {
	answer  state.Details
	reads   []state.Card
	details []*agents.Details
}

func (f *fakeDetails) Read(details *agents.Details, card state.Card) state.Details {
	f.reads = append(f.reads, card)
	f.details = append(f.details, details)
	return f.answer
}

// installBuiltinAgents copies the repo's built-in agents into the
// extensions folder, as install does.
func installBuiltinAgents(t *testing.T, p paths.Paths) {
	t.Helper()
	for _, id := range []string{"builtin.claude", "builtin.codex"} {
		for _, name := range []string{"extension.json", "mark.svg"} {
			data, err := os.ReadFile(filepath.Join("..", "..", "extensions", id, name))
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(p.ExtensionsDir, id, name), data)
		}
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetailsAreReadOnTheirKindsAtMostEvery5s(t *testing.T) {
	type step struct {
		kind  event.Kind // "" steps the daemon with no new event
		atMs  int64
		reads int // reads so far after this step
	}
	tests := []struct {
		name   string
		source string
		steps  []step
	}{
		{"a new turn reads at once", "claude", []step{{event.KindStarted, 1_000, 1}}},
		{"claude reads on started, finished and ended only", "claude", []step{
			{event.KindStarted, 1_000, 1}, {event.KindToolIntent, 10_000, 1}, {event.KindFinished, 20_000, 2},
		}},
		{"codex also reads on tool steps (its read_on)", "codex", []step{
			{event.KindStarted, 1_000, 1}, {event.KindToolIntent, 7_000, 2},
		}},
		{"a read inside 5 s waits, and is not lost", "claude", []step{
			{event.KindStarted, 1_000, 1}, {event.KindFinished, 3_000, 1}, {"", 5_999, 1}, {"", 6_000, 2}, {"", 20_000, 2},
		}},
		{"status line details never read", "claude", []step{
			{event.KindStarted, 1_000, 1}, {event.KindInfo, 60_000, 1},
		}},
		{"a source without an agent reads on the default kinds", "jira", []step{
			{event.KindStarted, 1_000, 1}, {event.KindToolIntent, 60_000, 1},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, p := newTestDaemon(t)
			installBuiltinAgents(t, p)
			reader := &fakeDetails{}
			d.Details = reader
			for _, current := range tt.steps {
				if current.kind != "" {
					spoolEvent(t, p, tt.source, "s1", current.kind, current.atMs)
				}
				d.Step(current.atMs, false)
				if len(reader.reads) != current.reads {
					t.Fatalf("after %q at %d: %d reads, want %d", current.kind, current.atMs, len(reader.reads), current.reads)
				}
			}
			if wantDetails := tt.source != "jira"; (reader.details[0] != nil) != wantDetails {
				t.Errorf("details given = %v, want %v", reader.details[0] != nil, wantDetails)
			}
		})
	}
}

func TestEventNameWinsOverDetailsName(t *testing.T) {
	d, p := newTestDaemon(t)
	d.Details = &fakeDetails{answer: state.Details{Name: "From the transcript"}}
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 1_000)
	info := event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindInfo, AtMs: 1_100, Name: "From the status line"}
	if err := spool.Write(p.SpoolDir, info); err != nil {
		t.Fatal(err)
	}

	d.Step(1_100, false)

	if cards := readStateFile(t, p).Cards; len(cards) != 1 || cards[0].Name != "From the status line" {
		t.Errorf("cards = %+v", cards)
	}
}

func TestDetailsReachTheStateFile(t *testing.T) {
	d, p := newTestDaemon(t)
	used := 77.0
	reader := &fakeDetails{answer: state.Details{Name: "Parser refactor", ContextPercent: &used, Branch: "main"}}
	d.Details = reader
	started := event.Event{V: event.Version, Source: "codex", Session: "s1", Kind: event.KindStarted, AtMs: 1_000,
		Title: "Refactor the parser", Cwd: "/w/app", Transcript: "/w/rollout.jsonl", Model: "gpt-5.6"}
	if err := spool.Write(p.SpoolDir, started); err != nil {
		t.Fatal(err)
	}

	d.Step(1_000, false)

	if len(reader.reads) != 1 || reader.reads[0].Cwd != "/w/app" || reader.reads[0].Transcript != "/w/rollout.jsonl" {
		t.Fatalf("reads = %+v, want one with the session's folder and log", reader.reads)
	}
	cards := readStateFile(t, p).Cards
	if len(cards) != 1 {
		t.Fatalf("cards = %+v", cards)
	}
	card := cards[0]
	if card.Name != "Parser refactor" || card.Branch != "main" || card.Model != "gpt-5.6" ||
		card.ContextPercent == nil || *card.ContextPercent != 77 {
		t.Errorf("card = %+v", card)
	}
	if card.Cwd != "" || card.Transcript != "" {
		t.Errorf("places leaked into state.json: %+v", card)
	}
}

func TestNoDetailsReaderReadsNothing(t *testing.T) {
	d, p := newTestDaemon(t)
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 1_000)
	if result := d.Step(1_000, false); !result.StateWritten {
		t.Fatal("state file not written")
	}
}

type fakeLearner struct{ learned map[string]int }

func (f *fakeLearner) Learn(modelID string, tokens int) error {
	f.learned[modelID] = tokens
	return nil
}

func TestStatusLineWindowSizesAreLearned(t *testing.T) {
	d, p := newTestDaemon(t)
	learner := &fakeLearner{learned: map[string]int{}}
	d.ContextSizes = learner
	info := event.Event{V: event.Version, Source: "claude", Session: "s1", Kind: event.KindInfo, AtMs: 1_000,
		Model: "Opus 5.5", ModelID: "claude-opus-5-5[1m]", ContextWindow: 1_000_000}
	noSize := event.Event{V: event.Version, Source: "claude", Session: "s2", Kind: event.KindInfo, AtMs: 1_000,
		Model: "Sonnet 5.5", ModelID: "claude-sonnet-5-5"}
	for _, incoming := range []event.Event{info, noSize} {
		if err := spool.Write(p.SpoolDir, incoming); err != nil {
			t.Fatal(err)
		}
	}
	d.Step(1_000, false)
	if want := map[string]int{"claude-opus-5-5[1m]": 1_000_000}; !reflect.DeepEqual(learner.learned, want) {
		t.Errorf("learned %v, want %v", learner.learned, want)
	}
}

// slowDetails blocks every read until release is closed, like a details
// converter near its 2 s limit.
type slowDetails struct {
	release chan struct{}
	answer  state.Details
}

func (s *slowDetails) Read(*agents.Details, state.Card) state.Details {
	<-s.release
	return s.answer
}

func TestASlowDetailsReadDoesNotDelayEvents(t *testing.T) {
	d, p := newTestDaemon(t)
	d.DetailsRunner = func(read func()) { go read() }
	reader := &slowDetails{release: make(chan struct{}), answer: state.Details{Name: "Read late"}}
	d.Details = reader
	spoolEvent(t, p, "claude", "s1", event.KindStarted, 1_000)
	d.Step(1_000, false) // starts the read, which now blocks

	spoolEvent(t, p, "claude", "s1", event.KindApproval, 2_000)
	started := time.Now()
	d.Step(2_000, false)

	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Errorf("a step took %v while a read was blocked", elapsed)
	}
	if cards := readStateFile(t, p).Cards; len(cards) != 1 || cards[0].Kind != state.KindApproval {
		t.Fatalf("cards = %+v, want the approval at once", cards)
	}
	close(reader.release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d.Step(3_000, false)
		if cards := readStateFile(t, p).Cards; cards[0].Name == "Read late" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("the finished read never reached the card")
}
