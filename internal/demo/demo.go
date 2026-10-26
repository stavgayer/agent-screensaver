// Package demo is the scripted scene the screen saver plays for a screen
// recording. Everything in it is made up: no real names, prompts or paths.
// The scene is a pure function of time, so tests need no clock.
package demo

import (
	"time"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
)

// Loop is the length of one pass of the scripted scene.
const Loop = 30 * time.Second

// Moments inside one loop, in milliseconds.
const (
	codexFinishesMs    = 6_000
	approvalStartsMs   = 10_000
	approvalAnsweredMs = 18_000
	claudeFinishesMs   = 24_000
)

// The Claude 5h ring rises once over the whole demo, not once per loop.
const (
	claudeFiveHourFrom = 42.0
	claudeFiveHourTo   = 61.0
)

const (
	minuteMs = 60_000
	hourSec  = 3_600
)

const approvalRequest = "Bash: npm run migrate"

// script is the fixed part of one card.
type script struct {
	source, session, title, branch string
	runningForMs                   int64
	contextPercent                 *float64
}

func percent(value float64) *float64 { return &value }

var scripts = []script{
	{"claude", "demo-claude", "refactor auth middleware", "feat/auth", 10 * minuteMs, percent(34)},
	{"codex", "demo-codex", "add rate limit tests", "", 4 * minuteMs, nil},
	{"cursor", "demo-cursor", "update dashboard styles", "", 7 * minuteMs, nil},
}

// Scene returns the cards and limits to show elapsedMs into a demo of
// totalMs, at wall time nowMs. Reset moments stay fixed from the demo start
// so the countdowns tick down.
func Scene(elapsedMs, totalMs, nowMs int64) ([]state.Card, []state.Limit) {
	loopMs := elapsedMs % Loop.Milliseconds()
	demoStartMs := nowMs - elapsedMs
	cards := make([]state.Card, 0, len(scripts))
	for _, item := range scripts {
		cards = append(cards, buildCard(item, loopMs, demoStartMs, nowMs))
	}
	return cards, buildLimits(elapsedMs, totalMs, demoStartMs, nowMs)
}

func buildCard(item script, loopMs, demoStartMs, nowMs int64) state.Card {
	card := state.Card{
		Source:         item.source,
		Session:        item.session,
		Kind:           kindAt(item.source, loopMs),
		Title:          item.title,
		Branch:         item.branch,
		ContextPercent: item.contextPercent,
		StartedAtMs:    demoStartMs - item.runningForMs,
		UpdatedAtMs:    nowMs,
	}
	if card.Kind == state.KindApproval {
		card.Request = approvalRequest
		since := nowMs - (loopMs - approvalStartsMs)
		card.ApprovalSinceMs = &since
	}
	return card
}

// kindAt is the script. Cursor never asks for approval.
func kindAt(source string, loopMs int64) state.CardKind {
	switch source {
	case "codex":
		if loopMs >= codexFinishesMs {
			return state.KindFinished
		}
	case "claude":
		switch {
		case loopMs >= claudeFinishesMs:
			return state.KindFinished
		case loopMs >= approvalStartsMs && loopMs < approvalAnsweredMs:
			return state.KindApproval
		}
	case "cursor":
		if loopMs >= claudeFinishesMs {
			return state.KindFinished
		}
	}
	return state.KindRunning
}

func buildLimits(elapsedMs, totalMs, demoStartMs, nowMs int64) []state.Limit {
	demoStartSec := demoStartMs / 1000
	limit := func(source string, window event.Window, used float64, resetsInSec int64) state.Limit {
		resetsAt := demoStartSec + resetsInSec
		return state.Limit{Source: source, Window: window, UsedPercent: used, ResetsAt: &resetsAt, UpdatedAtMs: nowMs}
	}
	return []state.Limit{
		limit("claude", event.FiveHour, claudeFiveHourAt(elapsedMs, totalMs), 2*hourSec),
		limit("claude", event.Week, 28, 3*24*hourSec),
		limit("codex", event.FiveHour, 17, 3*hourSec),
		limit("codex", event.Week, 9, 4*24*hourSec),
	}
}

// claudeFiveHourAt rises in a straight line over the demo and stays at the
// end value after it.
func claudeFiveHourAt(elapsedMs, totalMs int64) float64 {
	if totalMs <= 0 || elapsedMs >= totalMs {
		return claudeFiveHourTo
	}
	progress := float64(max(elapsedMs, 0)) / float64(totalMs)
	return claudeFiveHourFrom + (claudeFiveHourTo-claudeFiveHourFrom)*progress
}
