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
	// Name is the conversation name; see names for where it comes from.
	Name string `json:"name,omitempty"`
	// Model is the short model name, for example "Opus 5.5".
	Model string `json:"model,omitempty"`
	// Branch is the git branch checked out in the session's folder.
	Branch string `json:"branch,omitempty"`
	// ContextPercent is how full the context window is, 0 to 100.
	ContextPercent *float64 `json:"context_percent,omitempty"`
	// Cwd and Transcript tell the service where to read more details. They
	// stay out of state.json.
	Cwd        string `json:"-"`
	Transcript string `json:"-"`

	names names
	// contextFromEvent is true once an event (the status line) said how
	// full the context is. It is fresher than the transcript, so a value
	// read from the files no longer replaces it.
	contextFromEvent bool
}

// names holds the conversation name from each place it can come from. The
// status line is the freshest (it shows a /rename at once), the files the
// service reads come next, and a name in a hook payload is the last resort.
type names struct {
	status, file, hook string
}

func (n names) best() string {
	for _, name := range []string{n.status, n.file, n.hook} {
		if name != "" {
			return name
		}
	}
	return ""
}

// Details are what the service reads from the session's own files. Empty
// fields mean "not found" and keep what the card has.
type Details struct {
	Name           string
	Model          string
	ContextPercent *float64
	Branch         string
}

// Limit is the latest usage reading of one source and window.
type Limit struct {
	Source      string       `json:"source"`
	Window      event.Window `json:"window"`
	UsedPercent float64      `json:"used_percent"`
	ResetsAt    *int64       `json:"resets_at,omitempty"`
	UpdatedAtMs int64        `json:"updated_at_ms"`
	// Estimated is true when the percent is a local estimate.
	Estimated bool `json:"estimated,omitempty"`
}

// Source is what the service knows of one event source.
type Source struct {
	// LastEventMs is the time of the source's latest valid event. It proves
	// the source's hooks work, so events of ignored projects count too.
	LastEventMs int64 `json:"last_event_ms"`
}

// State is everything the screen saver shows. Cards keep insertion order so
// the screen does not shuffle between updates.
type State struct {
	Cards   []Card
	Limits  []Limit
	Sources map[string]Source
}

// NoteEvent records that a valid event of source arrived. The latest time
// wins, since hooks can deliver out of order.
func (s *State) NoteEvent(source string, atMs int64) {
	if s.Sources == nil {
		s.Sources = map[string]Source{}
	}
	if atMs > s.Sources[source].LastEventMs {
		s.Sources[source] = Source{LastEventMs: atMs}
	}
}

// Apply folds one event into the state.
func (s *State) Apply(incoming event.Event) {
	if incoming.Kind == event.KindLimits {
		s.applyLimits(incoming)
		return
	}
	index := s.findCard(incoming.Source, incoming.Session)
	if incoming.Kind == event.KindInfo {
		// Details never make a card: the status line also runs for
		// sessions no hook has told us about.
		if index >= 0 {
			applyDetails(&s.Cards[index], incoming, &s.Cards[index].names.status)
		}
		return
	}
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
		applyDetails(card, incoming, &card.names.hook)
	}
	if incoming.Project != "" && (!late || card.Project == "") {
		card.Project = incoming.Project
	}
	card.UpdatedAtMs = max(card.UpdatedAtMs, incoming.AtMs)
}

// Card returns a copy of the card of one session.
func (s *State) Card(source, session string) (Card, bool) {
	index := s.findCard(source, session)
	if index < 0 {
		return Card{}, false
	}
	return s.Cards[index], true
}

// SetDetails folds in what the service read from the session's files. It
// reports whether the card as shown changed.
func (s *State) SetDetails(source, session string, found Details) bool {
	index := s.findCard(source, session)
	if index < 0 {
		return false
	}
	card := &s.Cards[index]
	before := *card
	if found.Name != "" {
		card.names.file = found.Name
		card.Name = card.names.best()
	}
	if found.ContextPercent != nil && !card.contextFromEvent {
		card.ContextPercent = copyFloat64(found.ContextPercent)
	}
	if found.Branch != "" {
		card.Branch = found.Branch
	}
	if found.Model != "" {
		card.Model = found.Model
	}
	return card.Name != before.Name || card.Branch != before.Branch || card.Model != before.Model ||
		!equalFloat64(card.ContextPercent, before.ContextPercent)
}

// applyDetails copies the optional details of an event. A missing field
// keeps what the card has. nameSlot says which source the name came from.
func applyDetails(card *Card, incoming event.Event, nameSlot *string) {
	if incoming.Name != "" {
		*nameSlot = incoming.Name
		card.Name = card.names.best()
	}
	if incoming.Model != "" {
		card.Model = incoming.Model
	}
	if incoming.ContextPercent != nil {
		card.ContextPercent = copyFloat64(incoming.ContextPercent)
		card.contextFromEvent = true
	}
	if incoming.Cwd != "" {
		card.Cwd = incoming.Cwd
	}
	if incoming.Transcript != "" {
		card.Transcript = incoming.Transcript
	}
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
		limit.Estimated = reading.Estimated
		limit.UpdatedAtMs = max(limit.UpdatedAtMs, incoming.AtMs)
	}
}

// ReplaceLimit sets one limit of a source exactly as given, time included.
// The service uses it for a source whose readings one estimator keeps:
// that reading is the newest truth, even when it was taken before the
// estimate it replaces.
func (s *State) ReplaceLimit(source string, reading event.LimitReading, atMs int64) {
	index := slices.IndexFunc(s.Limits, func(limit Limit) bool {
		return limit.Source == source && limit.Window == reading.Window
	})
	replaced := Limit{Source: source, Window: reading.Window, UsedPercent: reading.UsedPercent,
		ResetsAt: copyInt64(reading.ResetsAt), UpdatedAtMs: atMs, Estimated: reading.Estimated}
	if index < 0 {
		s.Limits = append(s.Limits, replaced)
		return
	}
	s.Limits[index] = replaced
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

func copyFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func equalFloat64(a, b *float64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
