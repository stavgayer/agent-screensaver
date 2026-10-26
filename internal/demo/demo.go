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

// script is one card and its timeline inside a loop. Times are
// milliseconds from the loop start. A card with no approval window has
// approvalStartsMs equal to approvalEndsMs.
type script struct {
	source, session, title, project, model, branch string
	runningForMs                                   int64
	contextPercent                                 *float64
	approvalStartsMs, approvalEndsMs               int64
	finishesMs                                     int64
}

func percent(value float64) *float64 { return &value }

// Cursor never asks for approval.
var scripts = []script{
	{"claude", "demo-claude", "refactor auth middleware", "web-app", "Opus 5.5", "feat/auth",
		10 * minuteMs, percent(34), 10_000, 18_000, 24_000},
	{"codex", "demo-codex", "add rate limit tests", "web-app", "gpt-5.6", "",
		4 * minuteMs, nil, 0, 0, 6_000},
	{"cursor", "demo-cursor", "update dashboard styles", "web-app", "", "",
		7 * minuteMs, nil, 0, 0, 24_000},
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
		Kind:           kindAt(item, loopMs),
		Title:          item.title,
		Project:        item.project,
		Model:          item.model,
		Branch:         item.branch,
		ContextPercent: item.contextPercent,
		StartedAtMs:    demoStartMs - item.runningForMs,
		UpdatedAtMs:    nowMs,
	}
	switch card.Kind {
	case state.KindApproval:
		card.Request = approvalRequest
		since := nowMs - (loopMs - item.approvalStartsMs)
		card.ApprovalSinceMs = &since
	case state.KindFinished:
		// The card reads "Done" for as long as it has been finished.
		card.UpdatedAtMs = nowMs - (loopMs - item.finishesMs)
	}
	return card
}

func kindAt(item script, loopMs int64) state.CardKind {
	switch {
	case loopMs >= item.finishesMs:
		return state.KindFinished
	case loopMs >= item.approvalStartsMs && loopMs < item.approvalEndsMs:
		return state.KindApproval
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
