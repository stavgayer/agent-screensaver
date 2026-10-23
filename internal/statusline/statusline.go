// Package statusline turns the JSON that Claude Code pipes to its status
// line command into a limits event and an info event.
package statusline

import (
	"encoding/json"
	"path/filepath"
	"unicode/utf8"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/text"
)

const sourceClaude = "claude"

// FallbackSession names events whose input has no usable session id. Limits
// are account-wide, so one constant session loses nothing.
const FallbackSession = "statusline"

// window is one rate-limit window. The percentage is a pointer so that a
// window Claude Code left out can be told apart from one that is at 0%.
type window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}

type input struct {
	Cwd            string `json:"cwd"`
	SessionID      string `json:"session_id"`
	SessionName    string `json:"session_name"`
	TranscriptPath string `json:"transcript_path"`
	Model          struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	ContextWindow struct {
		// UsedPercentage is null before the first message.
		UsedPercentage    *float64 `json:"used_percentage"`
		ContextWindowSize int      `json:"context_window_size"`
	} `json:"context_window"`
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
		Session: sessionName(parsed.SessionID),
		Kind:    event.KindLimits,
		AtMs:    nowMs,
		Project: projectName(parsed),
		Limits:  readings,
	}, true
}

// InfoEvent builds an info event with the session's name, model and context
// fullness. It reports false when the input is not valid JSON, has no
// session id, or carries none of those details.
func InfoEvent(data []byte, nowMs int64) (event.Event, bool) {
	var parsed input
	if err := json.Unmarshal(data, &parsed); err != nil || sessionName(parsed.SessionID) == FallbackSession {
		return event.Event{}, false
	}
	model := parsed.Model.DisplayName
	if model == "" {
		model = parsed.Model.ID
	}
	info := event.Event{
		V:          event.Version,
		Source:     sourceClaude,
		Session:    parsed.SessionID,
		Kind:       event.KindInfo,
		AtMs:       nowMs,
		Project:    projectName(parsed),
		Name:       text.ShortTitle(parsed.SessionName),
		Model:      text.ShortModel(model),
		ModelID:    parsed.Model.ID,
		Cwd:        currentDir(parsed),
		Transcript: parsed.TranscriptPath,
	}
	if size := parsed.ContextWindow.ContextWindowSize; size > 0 && info.ModelID != "" {
		info.ContextWindow = size
	}
	if used := parsed.ContextWindow.UsedPercentage; used != nil {
		clamped := min(max(*used, 0), 100)
		info.ContextPercent = &clamped
	}
	if info.Name == "" && info.Model == "" && info.ContextPercent == nil {
		return event.Event{}, false
	}
	return info, true
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
	if dir := currentDir(parsed); dir != "" {
		return filepath.Base(dir)
	}
	return ""
}

func currentDir(parsed input) string {
	if parsed.Workspace.CurrentDir != "" {
		return parsed.Workspace.CurrentDir
	}
	return parsed.Cwd
}

// sessionName keeps the event valid: event.Parse rejects empty or oversized
// session ids, which would silently drop a good reading downstream.
func sessionName(sessionID string) string {
	if sessionID == "" || utf8.RuneCountInString(sessionID) > event.MaxIdentifierLength {
		return FallbackSession
	}
	return sessionID
}
