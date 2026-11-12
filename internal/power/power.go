// Package power decides what the battery means for the screen saver. A
// laptop on battery that is nearly empty should say so, and say more when
// agents are still working, because stopping them loses their work. This
// package holds those rules only: reading the battery is done elsewhere.
package power

import "agent-screensaver/internal/state"

// LowAtPercent is the battery level at or below which the battery counts as
// low.
const LowAtPercent = 20

// Reading is one sample of the battery. MinutesLeft is negative when the
// system does not know the time left yet.
type Reading struct {
	HasBattery  bool
	OnBattery   bool
	Charging    bool
	Percent     int
	MinutesLeft int
}

// Status is what the screen saver shows about the battery. MinutesLeft is
// nil when the time is unknown. Warn means the battery is low and agents are
// still working, so the owner should save or stop them.
type Status struct {
	Percent     int  `json:"percent"`
	MinutesLeft *int `json:"minutes_left,omitempty"`
	Charging    bool `json:"charging"`
	Low         bool `json:"low"`
	Warn        bool `json:"warn"`
	Working     int  `json:"working"`
}

// Working counts the cards whose agent has not finished, plus the finished
// ones that still run background jobs.
func Working(cards []state.Card) int {
	count := 0
	for _, card := range cards {
		if isWorking(card) {
			count++
		}
	}
	return count
}

// Evaluate returns the battery status, or nil when there is nothing to show:
// no battery, or a battery that is charging from a power plug.
func Evaluate(r Reading, cards []state.Card) *Status {
	if !r.HasBattery || !r.OnBattery {
		return nil
	}
	working := Working(cards)
	low := r.Percent <= LowAtPercent
	status := &Status{
		Percent:  r.Percent,
		Charging: r.Charging,
		Low:      low,
		Warn:     low && working > 0,
		Working:  working,
	}
	if r.MinutesLeft >= 0 {
		minutes := r.MinutesLeft
		status.MinutesLeft = &minutes
	}
	return status
}

// Same reports whether two statuses look the same on screen. The daemon
// writes the status file again only when this is false. MinutesLeft and
// Working are not compared, so a change in them alone does not cause a write.
func Same(a, b *Status) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Percent == b.Percent && a.Low == b.Low && a.Warn == b.Warn && a.Charging == b.Charging
}

// isWorking is true for every card that is not finished, and for a finished
// card that still has background jobs.
func isWorking(card state.Card) bool {
	return card.Kind != state.KindFinished || card.Background > 0
}
