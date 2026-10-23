package daemon

import (
	"testing"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
)

// fakeDetails answers every read with the same details and counts the reads.
type fakeDetails struct {
	answer state.Details
	reads  []state.Card
}

func (f *fakeDetails) Read(card state.Card) state.Details {
	f.reads = append(f.reads, card)
	return f.answer
}

func TestDetailsAreReadOnTurnEdgesAndAtMostEvery30s(t *testing.T) {
	type step struct {
		kind  event.Kind
		atMs  int64
		reads int // reads so far after this step
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"a new turn reads at once", []step{{event.KindStarted, 1_000, 1}}},
		{"tool steps inside 30 s do not read again", []step{
			{event.KindStarted, 1_000, 1}, {event.KindToolIntent, 5_000, 1}, {event.KindToolIntent, 30_999, 1},
		}},
		{"a tool step 30 s after the last read reads", []step{
			{event.KindStarted, 1_000, 1}, {event.KindToolIntent, 31_000, 2},
		}},
		{"the end of a turn always reads", []step{
			{event.KindStarted, 1_000, 1}, {event.KindFinished, 2_000, 2},
		}},
		{"status line details never read", []step{
			{event.KindStarted, 1_000, 1}, {event.KindInfo, 60_000, 1},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, p := newTestDaemon(t)
			reader := &fakeDetails{}
			d.Details = reader
			for _, current := range tt.steps {
				spoolEvent(t, p, "codex", "s1", current.kind, current.atMs)
				d.Step(current.atMs, false)
				if len(reader.reads) != current.reads {
					t.Fatalf("after %s at %d: %d reads, want %d", current.kind, current.atMs, len(reader.reads), current.reads)
				}
			}
		})
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
