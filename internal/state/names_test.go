package state

import (
	"testing"

	"agent-screensaver/internal/event"
)

func startedCard(s *State, session string, atMs int64) {
	s.Apply(event.Event{V: event.Version, Source: "claude", Session: session, Kind: event.KindStarted, AtMs: atMs, Title: "the prompt"})
}

func TestARememberedNameFillsAReadWithoutOne(t *testing.T) {
	var s State
	s.RememberNames([]SessionName{{Source: "claude", Session: "s1", Name: "Parser refactor", SeenAtMs: 1_000}})
	startedCard(&s, "s1", 2_000)

	s.SetDetails("claude", "s1", Details{})

	if card, _ := s.Card("claude", "s1"); card.Name != "Parser refactor" {
		t.Errorf("name = %q, want the remembered one", card.Name)
	}
}

func TestANameReadFromTheFilesWinsOverTheRememberedOne(t *testing.T) {
	var s State
	s.RememberNames([]SessionName{{Source: "claude", Session: "s1", Name: "Old", SeenAtMs: 1_000}})
	startedCard(&s, "s1", 2_000)

	s.SetDetails("claude", "s1", Details{Name: "New"})

	if card, _ := s.Card("claude", "s1"); card.Name != "New" {
		t.Errorf("name = %q", card.Name)
	}
	if names := s.SessionNames(3_000); len(names) != 1 || names[0].Name != "New" {
		t.Errorf("session names = %+v, want the new name only", names)
	}
}

func TestSessionNamesOutliveTheCardAndExpire(t *testing.T) {
	var s State
	startedCard(&s, "s1", 1_000)
	s.SetDetails("claude", "s1", Details{Name: "Parser refactor"})
	s.Cards = nil // the card went away

	if names := s.SessionNames(2_000); len(names) != 1 || names[0].Name != "Parser refactor" {
		t.Fatalf("session names = %+v", names)
	}
	if names := s.SessionNames(1_000 + SessionNameTTLMs + 1); len(names) != 0 {
		t.Errorf("session names after the TTL = %+v, want none", names)
	}
}

func TestSessionNamesAreCapped(t *testing.T) {
	var s State
	var many []SessionName
	for index := range MaxSessionNames + 5 {
		many = append(many, SessionName{Source: "claude", Session: string(rune('a'+index%26)) + string(rune('A'+index/26)), Name: "n", SeenAtMs: int64(index)})
	}
	s.RememberNames(many)

	names := s.SessionNames(1_000)

	if len(names) != MaxSessionNames {
		t.Fatalf("kept %d names, want %d", len(names), MaxSessionNames)
	}
	if names[0].SeenAtMs != int64(MaxSessionNames+4) {
		t.Errorf("first kept = %+v, want the newest first", names[0])
	}
}
