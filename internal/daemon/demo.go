package daemon

import (
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/demo"
	"agent-screensaver/internal/state"
)

// demoExpiryMs is how long an armed demo waits for the saver to show.
const demoExpiryMs = 5 * 60_000

// demoRun follows the scripted demo. Only the CLI writes the config, so the
// daemon never clears the armed fields: it remembers the armed value it has
// played or let expire, and ignores that value after.
type demoRun struct {
	armedMs int64 // the armed value being followed
	started bool  // the saver has shown since it was armed
	startMs int64 // when the saver showed
	spentMs int64 // the last armed value that played or expired
}

// demoFrame is the scripted scene to write instead of the real cards and
// limits.
type demoFrame struct {
	cards  []state.Card
	limits []state.Limit
}

func (f demoFrame) needsAttention() bool {
	scene := state.State{Cards: f.cards}
	return scene.NeedsAttention()
}

// next returns the frame to show at nowMs, or nil when the real state shows.
// The demo starts when the saver shows and ends after its seconds, or at
// once on a real approval, so a real agent never waits unseen.
func (r *demoRun) next(cfg config.Config, realApproval, saverActive bool, nowMs int64) *demoFrame {
	armed := cfg.DemoArmedMs
	if armed == 0 || armed == r.spentMs {
		r.armedMs, r.started = 0, false
		return nil
	}
	if armed != r.armedMs {
		r.armedMs, r.started = armed, false
	}
	if !r.started {
		if nowMs-armed >= demoExpiryMs {
			r.spend()
			return nil
		}
		if !saverActive || realApproval {
			return nil
		}
		r.started, r.startMs = true, nowMs
	}
	totalMs := int64(demoSeconds(cfg)) * 1000
	elapsedMs := nowMs - r.startMs
	if realApproval || elapsedMs >= totalMs {
		r.spend()
		return nil
	}
	cards, limits := demo.Scene(elapsedMs, totalMs, nowMs)
	return &demoFrame{cards: cards, limits: limits}
}

func (r *demoRun) spend() {
	r.spentMs, r.armedMs, r.started = r.armedMs, 0, false
}

// demoSeconds guards against a hand-edited config with no length.
func demoSeconds(cfg config.Config) int {
	if cfg.DemoSeconds <= 0 {
		return config.DemoDefaultSeconds
	}
	return cfg.DemoSeconds
}
