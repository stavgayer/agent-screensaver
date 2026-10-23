package daemon

import (
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
)

// detailsEveryMs is the shortest time between two reads of one session's
// files while it works. The start and the end of a turn always read: the
// Codex thread name appears a few seconds into the first turn and the token
// count is final at the end.
const detailsEveryMs = 30_000

// DetailsReader reads a card's details from the session's own files.
type DetailsReader interface {
	Read(card state.Card) state.Details
}

// ContextLearner remembers each model's context window size, which the
// status line says, for sessions whose files do not.
type ContextLearner interface {
	Learn(modelID string, tokens int) error
}

type cardKey struct{ source, session string }

// learnContextWindow passes on the window size an info event carries.
func (d *Daemon) learnContextWindow(incoming event.Event) {
	if d.ContextSizes == nil || incoming.Kind != event.KindInfo || incoming.ModelID == "" || incoming.ContextWindow <= 0 {
		return
	}
	if err := d.ContextSizes.Learn(incoming.ModelID, incoming.ContextWindow); err != nil {
		d.Logf("learn context window: %v", err)
	}
}

// noteDetailsDue marks a card whose files should be read after this drain.
func (d *Daemon) noteDetailsDue(incoming event.Event, nowMs int64) {
	if d.Details == nil || incoming.Kind == event.KindInfo || incoming.Kind == event.KindLimits {
		return
	}
	key := cardKey{incoming.Source, incoming.Session}
	turnEdge := incoming.Kind == event.KindStarted || incoming.Kind == event.KindFinished
	lastMs, readBefore := d.detailsReadMs[key]
	if turnEdge || !readBefore || nowMs-lastMs >= detailsEveryMs {
		d.detailsDue[key] = true
	}
}

// readDueDetails reads the marked cards and reports whether a shown card
// changed. Each card is read once per drain, however many events it had.
func (d *Daemon) readDueDetails(nowMs int64) bool {
	changed := false
	for key := range d.detailsDue {
		delete(d.detailsDue, key)
		card, ok := d.state.Card(key.source, key.session)
		if !ok {
			continue
		}
		d.detailsReadMs[key] = nowMs
		changed = d.state.SetDetails(key.source, key.session, d.Details.Read(card)) || changed
	}
	// Forget the sessions whose cards are gone.
	for key := range d.detailsReadMs {
		if _, ok := d.state.Card(key.source, key.session); !ok {
			delete(d.detailsReadMs, key)
		}
	}
	return changed
}
