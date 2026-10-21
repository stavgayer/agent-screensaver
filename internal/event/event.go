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

// Window names the period a usage limit covers.
type Window string

const (
	FiveHour Window = "5h"
	Week     Window = "week"
)

// LimitReading is one usage-limit measurement.
type LimitReading struct {
	Window      Window  `json:"window"`
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    *int64  `json:"resets_at,omitempty"` // Unix seconds
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
)

var knownKinds = map[Kind]bool{
	KindStarted:    true,
	KindToolIntent: true,
	KindApproval:   true,
	KindResumed:    true,
	KindFinished:   true,
	KindEnded:      true,
	KindLimits:     true,
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
