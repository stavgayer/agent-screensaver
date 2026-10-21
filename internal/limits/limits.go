// Package limits reads usage limits from tools that expose them locally.
// Adding a tool means one new file with a Provider and one line in All.
package limits

import (
	"context"
	"time"

	"agent-screensaver/internal/event"
)

// pollInterval is how long a reading stays fresh while nothing finishes.
const pollInterval = 5 * time.Minute

// Provider reads the current usage limits of one tool.
type Provider interface {
	// Source is the tool name, for example "codex". It matches the
	// agents.Adapter name.
	Source() string
	Read(ctx context.Context) ([]event.LimitReading, error)
}

// All is the one registry of providers. A tool is listed only when its
// program can be found, so a missing tool is not an error.
func All(lookPath func(string) (string, error)) []Provider {
	var providers []Provider
	if bin, err := lookPath(codexBinary); err == nil {
		providers = append(providers, Codex{Bin: bin})
	}
	return providers
}

// PollDue says whether the limits should be read again. Reading costs a
// process start, so it only happens while the saver is on screen: when
// nothing was read yet, when the reading is old, or when the tool finished
// a task (which is when usage changes).
func PollDue(lastPollMs *int64, saverActive, sourceFinishedSincePoll bool, nowMs int64) bool {
	if !saverActive {
		return false
	}
	if lastPollMs == nil || sourceFinishedSincePoll {
		return true
	}
	return nowMs-*lastPollMs >= pollInterval.Milliseconds()
}
