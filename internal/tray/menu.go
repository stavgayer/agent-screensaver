// Package tray is the menu bar app: a pure model of the menu (this file), the
// text rules shared with the web page (labels.go) and the thin glue to the
// system tray (tray.go).
package tray

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
)

// Item IDs that the user can click. The glue maps each to an action.
const (
	IDPreview  = "preview"
	IDSettings = "settings"
	IDSound    = "sound"
	IDPrivacy  = "privacy"
	IDQuit     = "quit"
)

// Item is one line of the menu. An item with no ID is text only. A text item
// with no label is a separator line.
type Item struct {
	ID, Label        string
	Enabled, Checked bool
}

// Model is the whole menu and the icon to show with it.
type Model struct {
	Attention bool // an agent waits for approval: show the attention icon
	Items     []Item
}

const (
	// staleAfterMs is how old the state file may be before the daemon counts as
	// dead. Same value as STALE_AFTER_MS in web/src/plan.ts; change both.
	staleAfterMs = 120_000
	// limitStaleAfterMs is the age of a limit reading that gets an "updated
	// Nm ago" note. Same value as STALE_AFTER_MS in
	// web/src/builtins/limit-rings/logic.ts; change both.
	limitStaleAfterMs = 15 * 60_000

	serviceDownLine = "Service not running"
)

// BuildMenu turns the state file and the settings into the menu. It is pure:
// the clock is a parameter. A state file that is missing (the zero value),
// says the service is down, or is too old means the daemon is dead: the cards
// and limits in it are not trusted, so only the warning and the controls show.
func BuildMenu(f statefile.StateFile, cfg config.Config, nowMs int64) Model {
	if !f.ServiceOK || nowMs-f.GeneratedAtMs > staleAfterMs {
		return Model{Items: append([]Item{text(serviceDownLine)}, controls(cfg)...)}
	}
	waiting := cardsOfKind(f.Cards, state.KindApproval)
	running := cardsOfKind(f.Cards, state.KindRunning)

	items := []Item{text(summaryLine(len(waiting), len(running)))}
	for _, reading := range orderLimits(f.Limits) {
		items = append(items, text(limitLine(reading, nowMs)))
	}
	for _, card := range waiting {
		items = append(items, text(approvalLine(card, cfg.Privacy, nowMs)))
	}
	return Model{Attention: len(waiting) > 0, Items: append(items, controls(cfg)...)}
}

func controls(cfg config.Config) []Item {
	return []Item{
		separatorLine(),
		action(IDPreview, "Preview screen saver", false),
		action(IDSettings, "Settings…", false),
		separatorLine(),
		action(IDSound, "Sound", cfg.Sound),
		action(IDPrivacy, "Privacy mode", cfg.Privacy),
		separatorLine(),
		action(IDQuit, "Quit menu bar app", false),
	}
}

func text(label string) Item { return Item{Label: label} }
func separatorLine() Item    { return Item{} }
func action(id, label string, checked bool) Item {
	return Item{ID: id, Label: label, Enabled: true, Checked: checked}
}

func cardsOfKind(cards []state.Card, kind state.CardKind) []state.Card {
	var matching []state.Card
	for _, card := range cards {
		if card.Kind == kind {
			matching = append(matching, card)
		}
	}
	return matching
}

func summaryLine(waiting, running int) string {
	var parts []string
	if waiting > 0 {
		parts = append(parts, fmt.Sprintf("%d waiting for approval", waiting))
	}
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if len(parts) == 0 {
		return "All quiet"
	}
	return strings.Join(parts, " · ")
}

// approvalLine is "Codex · api-server · waiting 3m". Privacy mode leaves the
// project out, as it does on the screen.
func approvalLine(card state.Card, privacy bool, nowMs int64) string {
	since := card.UpdatedAtMs
	if card.ApprovalSinceMs != nil {
		since = *card.ApprovalSinceMs
	}
	parts := []string{DisplayName(card.Source)}
	if card.Project != "" && !privacy {
		parts = append(parts, card.Project)
	}
	parts = append(parts, "waiting "+FormatDuration(nowMs-since))
	return strings.Join(parts, " · ")
}

// limitLine is "Claude 62% · resets in 1h40m" for the 5 hour window (its name
// is left out because it is the one people mean), "Codex week 55%" and
// "Codex plan 55% · resets 3 Nov". The week shows no reset, as on the screen.
// A local estimate ends with "est.", as on the screen.
func limitLine(reading state.Limit, nowMs int64) string {
	name := DisplayName(reading.Source)
	if reading.Window != event.FiveHour {
		name += " " + string(reading.Window)
	}
	line := fmt.Sprintf("%s %d%%", name, wholePercent(reading.UsedPercent))
	if reset := resetText(reading, nowMs); reset != "" {
		line += " · " + reset
	}
	if age := nowMs - reading.UpdatedAtMs; age > limitStaleAfterMs {
		line += " · updated " + FormatDuration(age) + " ago"
	}
	if reading.Estimated {
		line += " · est."
	}
	return line
}

func resetText(reading state.Limit, nowMs int64) string {
	if reading.ResetsAt == nil {
		return ""
	}
	resetsAtS := *reading.ResetsAt
	var label string
	switch reading.Window {
	case event.FiveHour:
		label = FormatResetIn(resetsAtS, nowMs)
	case event.Plan:
		label = FormatResetLabel(resetsAtS, nowMs)
	default:
		return ""
	}
	// A countdown reads "resets in 1h40m". A date already says "resets", and
	// a reset in the past is a whole sentence.
	if resetsAtS*1000 <= nowMs || strings.HasPrefix(label, "resets") {
		return label
	}
	return "resets in " + label
}

// wholePercent rounds and keeps the value inside 0..100, like ringPercent in
// web/src/builtins/limit-rings/logic.ts.
func wholePercent(used float64) int {
	if math.IsNaN(used) {
		return 0
	}
	return int(math.Min(100, math.Max(0, math.Round(used))))
}

// orderLimits puts claude first, then codex, then other sources by name; each
// source lists 5h, week, plan. Same order as the limit rings.
func orderLimits(limits []state.Limit) []state.Limit {
	sorted := slices.Clone(limits)
	slices.SortStableFunc(sorted, func(a, b state.Limit) int {
		return cmp.Or(
			cmp.Compare(sourceRank(a.Source), sourceRank(b.Source)),
			cmp.Compare(a.Source, b.Source),
			cmp.Compare(windowRank(a.Window), windowRank(b.Window)),
		)
	})
	return sorted
}

var knownSources = []string{"claude", "codex"}

func sourceRank(source string) int {
	if rank := slices.Index(knownSources, source); rank >= 0 {
		return rank
	}
	return len(knownSources)
}

var windowOrder = []event.Window{event.FiveHour, event.Week, event.Plan}

func windowRank(window event.Window) int { return slices.Index(windowOrder, window) }
