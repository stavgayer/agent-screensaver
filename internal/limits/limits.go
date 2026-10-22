// Package limits reads usage limits from tools that expose them locally.
// Adding a tool means one new file with a Provider and one line in All.
package limits

import (
	"context"
	"path/filepath"
	"slices"
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

// ToolDirs lists folders where command line tools are usually installed but
// which a LaunchAgent does not have on its PATH (launchd only gives it
// /usr/bin:/bin:/usr/sbin:/sbin). Codex installed with npm lives under nvm,
// for example. glob is filepath.Glob; tests pass a fake.
func ToolDirs(home string, glob func(string) ([]string, error)) []string {
	dirs := []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		filepath.Join(home, ".local/bin"),
		filepath.Join(home, ".npm-global/bin"),
		filepath.Join(home, ".volta/bin"),
		filepath.Join(home, ".bun/bin"),
	}
	// Newest node version first, so an upgrade wins over an old install.
	if nvm, err := glob(filepath.Join(home, ".nvm/versions/node/*/bin")); err == nil {
		slices.Sort(nvm)
		slices.Reverse(nvm)
		dirs = append(dirs, nvm...)
	}
	return dirs
}

// WithFallbackDirs returns a lookPath that tries PATH first and then each of
// dirs in order.
func WithFallbackDirs(lookPath func(string) (string, error), dirs []string) func(string) (string, error) {
	return func(name string) (string, error) {
		found, err := lookPath(name)
		if err == nil {
			return found, nil
		}
		for _, dir := range dirs {
			if candidate, candidateErr := lookPath(filepath.Join(dir, name)); candidateErr == nil {
				return candidate, nil
			}
		}
		return "", err
	}
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
