package state

import (
	"reflect"
	"testing"

	"agent-screensaver/internal/event"
)

const minuteMs = 60_000

func ptr(value int64) *int64 { return &value }

// sameCards treats nil and empty as equal; callers only care about content.
func sameCards(got, want []Card) bool {
	return (len(got) == 0 && len(want) == 0) || reflect.DeepEqual(got, want)
}

func claudeEvent(session string, kind event.Kind, atMs int64, project, title string) event.Event {
	return event.Event{V: 1, Source: "claude", Session: session, Kind: kind, AtMs: atMs, Project: project, Title: title}
}

func TestApply(t *testing.T) {
	tests := []struct {
		name   string
		events []event.Event
		want   []Card
	}{
		{
			name:   "started creates running card",
			events: []event.Event{claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug")},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", StartedAtMs: 1000, UpdatedAtMs: 1000}},
		},
		{
			name: "two sessions same project two cards",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "One"),
				claudeEvent("s2", event.KindStarted, 2000, "api", "Two"),
			},
			want: []Card{
				{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api", Title: "One", StartedAtMs: 1000, UpdatedAtMs: 1000},
				{Source: "claude", Session: "s2", Kind: KindRunning, Project: "api", Title: "Two", StartedAtMs: 2000, UpdatedAtMs: 2000},
			},
		},
		{
			name: "same session different source two cards",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "", "One"),
				{V: 1, Source: "codex", Session: "s1", Kind: event.KindStarted, AtMs: 2000, Title: "Two"},
			},
			want: []Card{
				{Source: "claude", Session: "s1", Kind: KindRunning, Title: "One", StartedAtMs: 1000, UpdatedAtMs: 1000},
				{Source: "codex", Session: "s1", Kind: KindRunning, Title: "Two", StartedAtMs: 2000, UpdatedAtMs: 2000},
			},
		},
		{
			name: "approval then resumed returns to running",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindApproval, 2000, "", ""),
				claudeEvent("s1", event.KindResumed, 3000, "", ""),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", StartedAtMs: 1000, UpdatedAtMs: 3000}},
		},
		{
			name: "started with an empty title keeps the previous title",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindStarted, 2000, "api", ""),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", StartedAtMs: 2000, UpdatedAtMs: 2000}},
		},
		{
			name: "approval keeps prompt title and sets request",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindApproval, 2000, "", "Run rm -rf build"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindApproval, Project: "api",
				Title: "Fix bug", Request: "Run rm -rf build", StartedAtMs: 1000, UpdatedAtMs: 2000,
				ApprovalSinceMs: ptr(2000)}},
		},
		{
			name: "tool intent fills request for titleless approval",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindToolIntent, 1500, "", "Edit main.go"),
				claudeEvent("s1", event.KindApproval, 2000, "", ""),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindApproval, Project: "api",
				Title: "Fix bug", Request: "Edit main.go", StartedAtMs: 1000, UpdatedAtMs: 2000,
				ApprovalSinceMs: ptr(2000)}},
		},
		{
			name: "tool intent creates running card when none exists",
			events: []event.Event{
				claudeEvent("s1", event.KindToolIntent, 1000, "api", "Edit main.go"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Request: "Edit main.go", StartedAtMs: 1000, UpdatedAtMs: 1000}},
		},
		{
			name: "started again resets card",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "First"),
				claudeEvent("s1", event.KindToolIntent, 1500, "", "Edit main.go"),
				claudeEvent("s1", event.KindApproval, 2000, "", ""),
				claudeEvent("s1", event.KindFinished, 2500, "", ""),
				claudeEvent("s1", event.KindStarted, 3000, "api", "Second"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Second", StartedAtMs: 3000, UpdatedAtMs: 3000}},
		},
		{
			name: "finished marks card finished",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindFinished, 2000, "", ""),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindFinished, Project: "api",
				Title: "Fix bug", StartedAtMs: 1000, UpdatedAtMs: 2000}},
		},
		{
			name: "ended removes running card",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindEnded, 2000, "", ""),
			},
			want: nil,
		},
		{
			name: "ended keeps finished card without bumping updated at",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindFinished, 2000, "", ""),
				claudeEvent("s1", event.KindEnded, 3000, "", ""),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindFinished, Project: "api",
				Title: "Fix bug", StartedAtMs: 1000, UpdatedAtMs: 2000}},
		},
		{
			name:   "ended for unknown session does nothing",
			events: []event.Event{claudeEvent("ghost", event.KindEnded, 1000, "", "")},
			want:   nil,
		},
		{
			name: "removing a card keeps the order of the others",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "", "One"),
				claudeEvent("s2", event.KindStarted, 1100, "", "Two"),
				claudeEvent("s3", event.KindStarted, 1200, "", "Three"),
				claudeEvent("s2", event.KindEnded, 1300, "", ""),
			},
			want: []Card{
				{Source: "claude", Session: "s1", Kind: KindRunning, Title: "One", StartedAtMs: 1000, UpdatedAtMs: 1000},
				{Source: "claude", Session: "s3", Kind: KindRunning, Title: "Three", StartedAtMs: 1200, UpdatedAtMs: 1200},
			},
		},
		{
			name: "late event does not rewind updated at",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 5000, "api", "Fix bug"),
				claudeEvent("s1", event.KindToolIntent, 2000, "", "Old intent"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", StartedAtMs: 5000, UpdatedAtMs: 5000}},
		},
		{
			name: "approval then tool intent returns to running",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindApproval, 2000, "", ""),
				claudeEvent("s1", event.KindToolIntent, 3000, "", "Edit b.go"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", Request: "Edit b.go", StartedAtMs: 1000, UpdatedAtMs: 3000}},
		},
		{
			name: "late approval does not flip running card",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindResumed, 3000, "", ""),
				claudeEvent("s1", event.KindApproval, 2000, "", "Old prompt"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", StartedAtMs: 1000, UpdatedAtMs: 3000}},
		},
		{
			name: "late tool intent does not clear approval",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindApproval, 3000, "", "Run it"),
				claudeEvent("s1", event.KindToolIntent, 2000, "", "Older intent"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindApproval, Project: "api",
				Title: "Fix bug", Request: "Run it", StartedAtMs: 1000, UpdatedAtMs: 3000,
				ApprovalSinceMs: ptr(3000)}},
		},
		{
			name: "late event may fill an empty project only",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 5000, "", "Fix bug"),
				claudeEvent("s1", event.KindToolIntent, 2000, "api", "Old"),
				claudeEvent("s1", event.KindToolIntent, 1000, "other", "Older"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindRunning, Project: "api",
				Title: "Fix bug", StartedAtMs: 5000, UpdatedAtMs: 5000}},
		},
		{
			name: "tool intent keeps finished card finished",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindFinished, 2000, "", ""),
				claudeEvent("s1", event.KindToolIntent, 3000, "", "Edit"),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindFinished, Project: "api",
				Title: "Fix bug", Request: "Edit", StartedAtMs: 1000, UpdatedAtMs: 3000}},
		},
		{
			name: "finished clears approval time",
			events: []event.Event{
				claudeEvent("s1", event.KindStarted, 1000, "api", "Fix bug"),
				claudeEvent("s1", event.KindApproval, 2000, "", ""),
				claudeEvent("s1", event.KindFinished, 3000, "", ""),
			},
			want: []Card{{Source: "claude", Session: "s1", Kind: KindFinished, Project: "api",
				Title: "Fix bug", StartedAtMs: 1000, UpdatedAtMs: 3000}},
		},
		{
			name: "limits event never creates a card",
			events: []event.Event{
				{V: 1, Source: "claude", Session: "statusline", Kind: event.KindLimits, AtMs: 1000,
					Limits: []event.LimitReading{{Window: event.FiveHour, UsedPercent: 10}}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var current State
			for _, incoming := range tt.events {
				current.Apply(incoming)
			}
			if !sameCards(current.Cards, tt.want) {
				t.Fatalf("cards:\n got  %+v\n want %+v", current.Cards, tt.want)
			}
		})
	}
}

func TestApplyCopiesApprovalTime(t *testing.T) {
	var current State
	current.Apply(claudeEvent("s1", event.KindApproval, 2000, "", ""))
	first := current.Cards[0].ApprovalSinceMs
	current.Apply(claudeEvent("s1", event.KindApproval, 9000, "", ""))
	if *first != 2000 {
		t.Fatalf("earlier pointer changed to %d; the card must own its value", *first)
	}
	if got := *current.Cards[0].ApprovalSinceMs; got != 9000 {
		t.Fatalf("approval since = %d, want 9000", got)
	}
}

func TestApplyLimits(t *testing.T) {
	limitsEvent := func(source string, atMs int64, readings ...event.LimitReading) event.Event {
		return event.Event{V: 1, Source: source, Session: "statusline", Kind: event.KindLimits, AtMs: atMs, Limits: readings}
	}
	resets := int64(1_800_000_000)
	tests := []struct {
		name   string
		events []event.Event
		want   []Limit
	}{
		{
			name: "limits upsert by source and window",
			events: []event.Event{
				limitsEvent("claude", 1000,
					event.LimitReading{Window: event.FiveHour, UsedPercent: 10},
					event.LimitReading{Window: event.Week, UsedPercent: 20, ResetsAt: &resets}),
				limitsEvent("codex", 1500, event.LimitReading{Window: event.FiveHour, UsedPercent: 5}),
				limitsEvent("claude", 2000, event.LimitReading{Window: event.FiveHour, UsedPercent: 42}),
			},
			want: []Limit{
				{Source: "claude", Window: event.FiveHour, UsedPercent: 42, UpdatedAtMs: 2000},
				{Source: "claude", Window: event.Week, UsedPercent: 20, ResetsAt: &resets, UpdatedAtMs: 1000},
				{Source: "codex", Window: event.FiveHour, UsedPercent: 5, UpdatedAtMs: 1500},
			},
		},
		{
			name: "late limits reading does not rewind updated at",
			events: []event.Event{
				limitsEvent("claude", 5000, event.LimitReading{Window: event.Week, UsedPercent: 30}),
				limitsEvent("claude", 3000, event.LimitReading{Window: event.Week, UsedPercent: 31}),
			},
			want: []Limit{{Source: "claude", Window: event.Week, UsedPercent: 31, UpdatedAtMs: 5000}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var current State
			for _, incoming := range tt.events {
				current.Apply(incoming)
			}
			if !reflect.DeepEqual(current.Limits, tt.want) {
				t.Fatalf("limits:\n got  %+v\n want %+v", current.Limits, tt.want)
			}
		})
	}
}

func TestTick(t *testing.T) {
	card := func(kind CardKind, updatedAtMs int64) Card {
		return Card{Source: "claude", Session: "s1", Kind: kind, StartedAtMs: 0, UpdatedAtMs: updatedAtMs}
	}
	tests := []struct {
		name  string
		cards []Card
		now   int64
		want  []Card
	}{
		{"finished kept at 29 min", []Card{card(KindFinished, 0)}, 29 * minuteMs, []Card{card(KindFinished, 0)}},
		{"finished kept exactly at ttl", []Card{card(KindFinished, 0)}, FinishedTTLMs, []Card{card(KindFinished, 0)}},
		{"finished removed at 31 min", []Card{card(KindFinished, 0)}, 31 * minuteMs, nil},
		{"approval kept at 9 min", []Card{card(KindApproval, 0)}, 9 * minuteMs, []Card{card(KindApproval, 0)}},
		{"approval kept exactly at 10 min", []Card{card(KindApproval, 0)}, ApprovalUnknownAfterMs, []Card{card(KindApproval, 0)}},
		{"approval becomes unknown after 10 min", []Card{card(KindApproval, 0)}, 11 * minuteMs, []Card{card(KindUnknown, 0)}},
		{"running kept at 59 min", []Card{card(KindRunning, 0)}, 59 * minuteMs, []Card{card(KindRunning, 0)}},
		{"running kept exactly at 60 min", []Card{card(KindRunning, 0)}, NoActivityAfterMs, []Card{card(KindRunning, 0)}},
		{"running no activity at 60 min plus 1 ms", []Card{card(KindRunning, 0)}, NoActivityAfterMs + 1, []Card{card(KindNoActivity, 0)}},
		{"running becomes no activity after 60 min", []Card{card(KindRunning, 0)}, 61 * minuteMs, []Card{card(KindNoActivity, 0)}},
		{"unknown stays unknown at 5 h", []Card{card(KindUnknown, 0)}, 5 * 60 * minuteMs, []Card{card(KindUnknown, 0)}},
		{"any card removed after 12 h running", []Card{card(KindRunning, 0)}, RemoveAfterMs + 1, nil},
		{"any card removed after 12 h approval", []Card{card(KindApproval, 0)}, RemoveAfterMs + 1, nil},
		{"any card removed after 12 h no activity", []Card{card(KindNoActivity, 0)}, RemoveAfterMs + 1, nil},
		{"any card kept exactly at 12 h", []Card{card(KindNoActivity, 0)}, RemoveAfterMs, []Card{card(KindNoActivity, 0)}},
		{
			name:  "only stale cards change and order is kept",
			cards: []Card{card(KindRunning, 100*minuteMs), card(KindFinished, 0), card(KindApproval, 100*minuteMs)},
			now:   101 * minuteMs,
			want:  []Card{card(KindRunning, 100*minuteMs), card(KindApproval, 100*minuteMs)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := State{Cards: tt.cards}
			current.Tick(tt.now)
			if !sameCards(current.Cards, tt.want) {
				t.Fatalf("cards:\n got  %+v\n want %+v", current.Cards, tt.want)
			}
		})
	}
}

func TestToolIntentRevivesDemotedCard(t *testing.T) {
	tests := []struct {
		name   string
		demote CardKind
		idleMs int64
	}{
		{"unknown then tool intent returns to running", KindUnknown, 11 * minuteMs},
		{"no activity then tool intent returns to running", KindNoActivity, 61 * minuteMs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var current State
			current.Apply(claudeEvent("s1", event.KindStarted, 0, "api", "Fix bug"))
			if tt.demote == KindUnknown {
				current.Apply(claudeEvent("s1", event.KindApproval, 0, "", ""))
			}
			current.Tick(tt.idleMs)
			if got := current.Cards[0].Kind; got != tt.demote {
				t.Fatalf("after tick kind = %s, want %s", got, tt.demote)
			}
			current.Apply(claudeEvent("s1", event.KindToolIntent, tt.idleMs, "", "Edit"))
			card := current.Cards[0]
			if card.Kind != KindRunning || card.ApprovalSinceMs != nil || card.Request != "Edit" {
				t.Fatalf("card = %+v, want running, no approval time, request Edit", card)
			}
		})
	}
}

func TestTickKeepsLimits(t *testing.T) {
	current := State{Limits: []Limit{{Source: "claude", Window: event.Week, UpdatedAtMs: 0}}}
	current.Tick(RemoveAfterMs * 2)
	if len(current.Limits) != 1 {
		t.Fatalf("limits = %+v, want the reading kept", current.Limits)
	}
}

func TestNeedsAttention(t *testing.T) {
	tests := []struct {
		name  string
		kinds []CardKind
		want  bool
	}{
		{"needs attention only with approval: none", nil, false},
		{"needs attention only with approval: running and finished", []CardKind{KindRunning, KindFinished}, false},
		{"needs attention only with approval: unknown is not approval", []CardKind{KindUnknown, KindNoActivity}, false},
		{"needs attention only with approval: one approval", []CardKind{KindRunning, KindApproval}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var current State
			for _, kind := range tt.kinds {
				current.Cards = append(current.Cards, Card{Kind: kind})
			}
			if got := current.NeedsAttention(); got != tt.want {
				t.Fatalf("NeedsAttention() = %v, want %v", got, tt.want)
			}
		})
	}
}
