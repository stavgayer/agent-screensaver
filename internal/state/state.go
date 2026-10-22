// Package state turns the stream of events into the cards and usage limits
// the screen saver shows. It holds the business rules and nothing else: no
// clock, no files, no JSON transport.
package state

import (
	"slices"

	"agent-screensaver/internal/event"
)

// Timing rules, all measured from a card's last event.
const (
	FinishedTTLMs          = 30 * 60_000
	NoActivityAfterMs      = 60 * 60_000
	RemoveAfterMs          = 12 * 3_600_000
	ApprovalUnknownAfterMs = 10 * 60_000
)

// CardKind is what a card shows.
type CardKind string

const (
	KindApproval   CardKind = "approval"
	KindRunning    CardKind = "running"
	KindFinished   CardKind = "finished"
	KindUnknown    CardKind = "unknown"
	KindNoActivity CardKind = "no_activity"
)

// Card is one agent session on screen.
type Card struct {
	Source          string   `json:"source"`
	Session         string   `json:"session"`
	Kind            CardKind `json:"kind"`
	Project         string   `json:"project,omitempty"`
	Title           string   `json:"title,omitempty"`
	Request         string   `json:"request,omitempty"`
	StartedAtMs     int64    `json:"started_at_ms"`
	UpdatedAtMs     int64    `json:"updated_at_ms"`
	ApprovalSinceMs *int64   `json:"approval_since_ms,omitempty"`
	// Background is how many background jobs the session still runs. A
	// finished card with jobs left stays until they are done.
	Background int `json:"background,omitempty"`
}

// Limit is the latest usage reading of one source and window.
type Limit struct {
	Source      string       `json:"source"`
	Window      event.Window `json:"window"`
	UsedPercent float64      `json:"used_percent"`
	ResetsAt    *int64       `json:"resets_at,omitempty"`
	UpdatedAtMs int64        `json:"updated_at_ms"`
}

// State is everything the screen saver shows. Cards keep insertion order so
// the screen does not shuffle between updates.
type State struct {
	Cards  []Card
	Limits []Limit
}

// Apply folds one event into the state.
func (s *State) Apply(incoming event.Event) {
	if incoming.Kind == event.KindLimits {
		s.applyLimits(incoming)
		return
	}
	index := s.findCard(incoming.Source, incoming.Session)
	if incoming.Kind == event.KindEnded {
		s.applyEnded(index)
		return
	}
	if index < 0 {
		s.Cards = append(s.Cards, newCard(incoming))
		index = len(s.Cards) - 1
	}
	card := &s.Cards[index]
	// Hooks are separate processes, so an old event can arrive after a newer
	// one. It may only fill a missing project, never rewrite the card.
	late := incoming.AtMs < card.UpdatedAtMs
	if !late {
		applyToCard(card, incoming)
	}
	if incoming.Project != "" && (!late || card.Project == "") {
		card.Project = incoming.Project
	}
	card.UpdatedAtMs = max(card.UpdatedAtMs, incoming.AtMs)
}

// applyEnded drops a card, except a finished one: it stays so its result
// remains visible until the TTL, which counts from the finish. Background
// jobs end with their session, so its count goes to zero.
func (s *State) applyEnded(index int) {
	if index < 0 {
		return
	}
	if s.Cards[index].Kind != KindFinished {
		s.Cards = slices.Delete(s.Cards, index, index+1)
		return
	}
	s.Cards[index].Background = 0
}

// Tick ages the cards. Limits are readings, not sessions, so they stay.
func (s *State) Tick(nowMs int64) {
	s.Cards = slices.DeleteFunc(s.Cards, func(card Card) bool {
		return isExpired(card, nowMs)
	})
	for index := range s.Cards {
		s.Cards[index].Kind = agedKind(s.Cards[index], nowMs)
	}
}

// NeedsAttention reports whether any card waits for the user to approve.
func (s *State) NeedsAttention() bool {
	return slices.ContainsFunc(s.Cards, func(card Card) bool {
		return card.Kind == KindApproval
	})
}

func newCard(incoming event.Event) Card {
	return Card{
		Source:      incoming.Source,
		Session:     incoming.Session,
		Kind:        KindRunning,
		StartedAtMs: incoming.AtMs,
	}
}

func applyToCard(card *Card, incoming event.Event) {
	if incoming.Background != nil {
		card.Background = *incoming.Background
	}
	switch incoming.Kind {
	case event.KindStarted:
		card.Kind = KindRunning
		// An empty title means the prompt had only machine text: keep the
		// title the card already has.
		if incoming.Title != "" {
			card.Title = incoming.Title
		}
		card.StartedAtMs = incoming.AtMs
		card.Request = ""
		card.ApprovalSinceMs = nil
	case event.KindToolIntent:
		card.Request = incoming.Title
		if card.Kind != KindFinished {
			// A new tool attempt means the approval was answered or denied.
			card.Kind = KindRunning
			card.ApprovalSinceMs = nil
		}
	case event.KindApproval:
		card.Kind = KindApproval
		since := incoming.AtMs
		card.ApprovalSinceMs = &since
		if incoming.Title != "" {
			card.Request = incoming.Title
		}
	case event.KindResumed:
		card.Kind = KindRunning
		card.ApprovalSinceMs = nil
	case event.KindFinished:
		card.Kind = KindFinished
		card.ApprovalSinceMs = nil
	}
}

func (s *State) applyLimits(incoming event.Event) {
	for _, reading := range incoming.Limits {
		index := slices.IndexFunc(s.Limits, func(limit Limit) bool {
			return limit.Source == incoming.Source && limit.Window == reading.Window
		})
		if index < 0 {
			s.Limits = append(s.Limits, Limit{Source: incoming.Source, Window: reading.Window})
			index = len(s.Limits) - 1
		}
		limit := &s.Limits[index]
		limit.UsedPercent = reading.UsedPercent
		limit.ResetsAt = copyInt64(reading.ResetsAt)
		limit.UpdatedAtMs = max(limit.UpdatedAtMs, incoming.AtMs)
	}
}

func (s *State) findCard(source, session string) int {
	return slices.IndexFunc(s.Cards, func(card Card) bool {
		return card.Source == source && card.Session == session
	})
}

func isExpired(card Card, nowMs int64) bool {
	idleMs := nowMs - card.UpdatedAtMs
	if idleMs > RemoveAfterMs {
		return true
	}
	// A done card whose background jobs still run is not done yet.
	return card.Kind == KindFinished && card.Background == 0 && idleMs > FinishedTTLMs
}

// agedKind downgrades a silent card: an agent that stops reporting while
// waiting is probably no longer waiting, and one that stops while running is
// probably gone.
func agedKind(card Card, nowMs int64) CardKind {
	idleMs := nowMs - card.UpdatedAtMs
	switch {
	case card.Kind == KindApproval && idleMs > ApprovalUnknownAfterMs:
		return KindUnknown
	case card.Kind == KindRunning && idleMs > NoActivityAfterMs:
		return KindNoActivity
	default:
		return card.Kind
	}
}

func copyInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
