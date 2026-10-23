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

// maxDetailReads bounds the reads that run at once, off the step: a details
// converter may take up to 2 s, which must never hold up the events.
const maxDetailReads = 4

// detailsResult is one finished read, merged into the state on the loop.
type detailsResult struct {
	key     cardKey
	details state.Details
}

// readDueDetails merges the reads that finished, then starts reads for the
// marked cards whose last read is at least detailsEveryMs old, one at a time
// per card and at most maxDetailReads at once. The others stay marked for a
// later step. It reports whether a shown card changed.
func (d *Daemon) readDueDetails(nowMs int64) bool {
	for key := range d.detailsDue {
		card, ok := d.state.Card(key.source, key.session)
		if !ok {
			delete(d.detailsDue, key)
			continue
		}
		lastMs, readBefore := d.detailsReadMs[key]
		if d.detailsInFlight[key] || len(d.detailsInFlight) >= maxDetailReads || readBefore && nowMs-lastMs < detailsEveryMs {
			continue
		}
		delete(d.detailsDue, key)
		d.detailsReadMs[key] = nowMs
		d.detailsInFlight[key] = true
		reader, details := d.Details, d.detailsOf(key.source)
		d.DetailsRunner(func() { d.detailsResults <- detailsResult{key, reader.Read(details, card)} })
	}
	changed := d.mergeDetails()
	// Forget the sessions whose cards are gone.
	for key := range d.detailsReadMs {
		if _, ok := d.state.Card(key.source, key.session); !ok {
			delete(d.detailsReadMs, key)
		}
	}
	return changed
}

// mergeDetails folds the finished reads into the state. It never waits.
func (d *Daemon) mergeDetails() bool {
	changed := false
	for {
		select {
		case result := <-d.detailsResults:
			delete(d.detailsInFlight, result.key)
			changed = d.state.SetDetails(result.key.source, result.key.session, result.details) || changed
		default:
			return changed
		}
	}
}
