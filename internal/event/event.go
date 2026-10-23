// Package event defines the versioned JSON event that hooks, the status
// line and outside source programs write into the spool folder.
package event

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Version is the only event format version this package accepts.
const Version = 1

const (
	// MaxInputBytes caps one event file so a bad writer cannot exhaust memory.
	MaxInputBytes = 64 * 1024
	// MaxIdentifierLength caps, in characters, the source and session fields.
	MaxIdentifierLength = 128
)

// Window names the period a usage limit covers, or "plan" for a usage-based
// limit that has no period.
type Window string

const (
	FiveHour Window = "5h"
	Week     Window = "week"
	// Plan is a usage-based plan limit that has no fixed period.
	Plan Window = "plan"
)

// LimitReading is one usage-limit measurement.
type LimitReading struct {
	Window      Window  `json:"window"`
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    *int64  `json:"resets_at,omitempty"` // Unix seconds
	// Estimated is true when the percent was worked out locally between
	// exact readings, not read from the tool.
	Estimated bool `json:"estimated,omitempty"`
}

// Kind says what happened.
type Kind string

const (
	KindStarted    Kind = "started"
	KindToolIntent Kind = "tool_intent"
	KindApproval   Kind = "approval"
	KindResumed    Kind = "resumed"
	KindFinished   Kind = "finished"
	KindEnded      Kind = "ended"
	KindLimits     Kind = "limits"
	// KindInfo carries details of a session (name, model, context) and
	// nothing else: it never makes a card and never changes its state.
	KindInfo Kind = "info"
)

var knownKinds = map[Kind]bool{
	KindStarted:    true,
	KindToolIntent: true,
	KindApproval:   true,
	KindResumed:    true,
	KindFinished:   true,
	KindEnded:      true,
	KindLimits:     true,
	KindInfo:       true,
}

// Event is one message from a source to the service.
type Event struct {
	V       int            `json:"v"`
	Source  string         `json:"source"`
	Session string         `json:"session"`
	Kind    Kind           `json:"kind"`
	AtMs    int64          `json:"at_ms"`
	Project string         `json:"project,omitempty"`
	Title   string         `json:"title,omitempty"`
	Limits  []LimitReading `json:"limits,omitempty"`
	// Background is how many background jobs (shells, subagents) the session
	// still runs. nil means the source did not say; 0 means none are left.
	Background *int `json:"background,omitempty"`
	// Name is the conversation name the tool shows, for example one set
	// with /rename. It is preferred to the title made from the prompt.
	Name string `json:"name,omitempty"`
	// Model is the short model name, for example "Opus 5.5".
	Model string `json:"model,omitempty"`
	// ContextPercent is how full the context window is, 0 to 100. nil
	// means the source did not say.
	ContextPercent *float64 `json:"context_percent,omitempty"`
	// Cwd and Transcript tell the service where to look for more details
	// (the git branch, the conversation name). They are never shown.
	Cwd        string `json:"cwd,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

// Parse decodes and validates one event. It checks the size first so that
// oversized input is rejected before any decoding work.
func Parse(data []byte) (Event, error) {
	if len(data) > MaxInputBytes {
		return Event{}, fmt.Errorf("event is %d bytes, limit is %d", len(data), MaxInputBytes)
	}
	var parsed Event
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Event{}, fmt.Errorf("decode event: %w", err)
	}
	if err := parsed.validate(); err != nil {
		return Event{}, err
	}
	return parsed, nil
}

func (e Event) validate() error {
	if e.V != Version {
		return fmt.Errorf("unsupported event version %d, want %d", e.V, Version)
	}
	if !knownKinds[e.Kind] {
		return fmt.Errorf("unknown event kind %q", e.Kind)
	}
	if e.Background != nil && *e.Background < 0 {
		return fmt.Errorf("background is %d, must not be negative", *e.Background)
	}
	if e.ContextPercent != nil && (*e.ContextPercent < 0 || *e.ContextPercent > 100) {
		return fmt.Errorf("context_percent is %v, must be 0 to 100", *e.ContextPercent)
	}
	if err := validateIdentifier("source", e.Source); err != nil {
		return err
	}
	return validateIdentifier("session", e.Session)
}

func validateIdentifier(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if length := utf8.RuneCountInString(value); length > MaxIdentifierLength {
		return fmt.Errorf("%s is %d characters, limit is %d", field, length, MaxIdentifierLength)
	}
	return nil
}
