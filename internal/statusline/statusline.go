// Package statusline turns the JSON that Claude Code pipes to its status
// line command into a limits event.
package statusline

import (
	"encoding/json"
	"path/filepath"

	"agent-screensaver/internal/event"
)

const sourceClaude = "claude"

// window is one rate-limit window. The percentage is a pointer so that a
// window Claude Code left out can be told apart from one that is at 0%.
type window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}

type input struct {
	Cwd       string `json:"cwd"`
	SessionID string `json:"session_id"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	RateLimits struct {
		FiveHour *window `json:"five_hour"`
		SevenDay *window `json:"seven_day"`
	} `json:"rate_limits"`
}

// LimitsEvent builds a limits event from status line input. It reports false
// when the input is not valid JSON or carries no usable reading, which is
// normal for API-key users and early in a session.
func LimitsEvent(data []byte, nowMs int64) (event.Event, bool) {
	var parsed input
	if err := json.Unmarshal(data, &parsed); err != nil {
		return event.Event{}, false
	}

	var readings []event.LimitReading
	readings = appendReading(readings, event.FiveHour, parsed.RateLimits.FiveHour)
	readings = appendReading(readings, event.Week, parsed.RateLimits.SevenDay)
	if len(readings) == 0 {
		return event.Event{}, false
	}

	return event.Event{
		V:       event.Version,
		Source:  sourceClaude,
		Session: parsed.SessionID,
		Kind:    event.KindLimits,
		AtMs:    nowMs,
		Project: projectName(parsed),
		Limits:  readings,
	}, true
}

func appendReading(readings []event.LimitReading, name event.Window, found *window) []event.LimitReading {
	if found == nil || found.UsedPercentage == nil {
		return readings
	}
	return append(readings, event.LimitReading{
		Window:      name,
		UsedPercent: min(max(*found.UsedPercentage, 0), 100),
		ResetsAt:    found.ResetsAt,
	})
}

// projectName prefers workspace.current_dir, which the docs recommend, and
// falls back to cwd, which holds the same value.
func projectName(parsed input) string {
	dir := parsed.Workspace.CurrentDir
	if dir == "" {
		dir = parsed.Cwd
	}
	if dir == "" {
		return ""
	}
	return filepath.Base(dir)
}
