package daemon

import (
	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
)

// detailsEveryMs is the shortest time between two reads of one card's
// files (spec §7d "Details"). A read that comes due sooner waits, so the
// read after the end of a turn is never lost.
const detailsEveryMs = 5_000

// DetailsReader reads a card's details from the session's own files, as
// the agent's details say (nil when it declares none).
type DetailsReader interface {
	Read(details *agents.Details, card state.Card) state.Details
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

// noteDetailsDue marks a card whose files should be read, after the event
// kinds the agent's details name (read_on).
func (d *Daemon) noteDetailsDue(incoming event.Event) {
	if d.Details == nil || incoming.Kind == event.KindInfo || incoming.Kind == event.KindLimits {
		return
	}
	if d.detailsOf(incoming.Source).ReadsOn(incoming.Kind) {
		d.detailsDue[cardKey{incoming.Source, incoming.Session}] = true
	}
}

// detailsOf is the details spec of a source's agent, nil for none.
func (d *Daemon) detailsOf(source string) *agents.Details {
	agent, ok := agents.BySource(d.agents, source)
	if !ok {
		return nil
	}
	return agent.Details
}

// readDueDetails reads the marked cards whose last read is at least
// detailsEveryMs old, and reports whether a shown card changed. The others
// stay marked for a later step.
func (d *Daemon) readDueDetails(nowMs int64) bool {
	changed := false
	for key := range d.detailsDue {
		card, ok := d.state.Card(key.source, key.session)
		if !ok {
			delete(d.detailsDue, key)
			continue
		}
		if lastMs, readBefore := d.detailsReadMs[key]; readBefore && nowMs-lastMs < detailsEveryMs {
			continue
		}
		delete(d.detailsDue, key)
		d.detailsReadMs[key] = nowMs
		changed = d.state.SetDetails(key.source, key.session, d.Details.Read(d.detailsOf(key.source), card)) || changed
	}
	// Forget the sessions whose cards are gone.
	for key := range d.detailsReadMs {
		if _, ok := d.state.Card(key.source, key.session); !ok {
			delete(d.detailsReadMs, key)
		}
	}
	return changed
}
