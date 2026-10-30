package daemon

import (
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/demo"
	"agent-screensaver/internal/nowplaying"
	"agent-screensaver/internal/state"
)

// demoExpiryMs is how long an armed demo waits for the saver to show.
const demoExpiryMs = 5 * 60_000

// demoFutureSlackMs is how far in the future an armed time may be and still
// play. A step reads the clock before it reads the config, so a demo armed in
// that gap looks a little in the future.
const demoFutureSlackMs = 5_000

// demoRun follows the scripted demo. The daemon never writes the config (the
// CLI and the settings window do), so it never clears the armed fields: it
// remembers the armed value it has played or let expire, and ignores that
// value after.
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
	track  nowplaying.Track
}

func (f demoFrame) needsAttention() bool {
	scene := state.State{Cards: f.cards}
	return scene.NeedsAttention()
}

// next returns the frame to show at nowMs, or nil when the real state shows.
// The demo starts when the saver shows. It ends after its seconds, when the
// saver hides (so the menu bar never shows it while the user works), or at
// once on a real approval, so a real agent never waits unseen. A clock that
// jumps back, or an armed time well in the future, ends it too.
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
		if nowMs-armed >= demoExpiryMs || armed > nowMs+demoFutureSlackMs {
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
	if realApproval || !saverActive || elapsedMs < 0 || elapsedMs >= totalMs {
		r.spend()
		return nil
	}
	cards, limits := demo.Scene(elapsedMs, totalMs, nowMs)
	return &demoFrame{cards: cards, limits: limits, track: demo.Track(elapsedMs, nowMs)}
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
