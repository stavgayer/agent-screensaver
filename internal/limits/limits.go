// Package limits reads usage limits from tools that expose them locally.
// Adding a tool means one new file with a Provider and one line in All.
package limits

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"agent-screensaver/internal/event"
)

const (
	// pollInterval is how long a reading stays fresh while nothing finishes.
	pollInterval = 5 * time.Minute
	// retryAfterFailure is how soon a failed read is tried again while the
	// saver shows. Without it the owner would wait a whole pollInterval
	// for the ring after one slow first start.
	retryAfterFailure = 30 * time.Second
	// ReadTimeout is how long one provider read may take. Codex sets itself
	// up on its first start after an upgrade, and that took more than 10 s.
	ReadTimeout = 30 * time.Second
)

// Provider reads the current usage limits of one tool.
type Provider interface {
	// Source is the tool name, for example "codex". It matches the
	// agents.Adapter name.
	Source() string
	Read(ctx context.Context) ([]event.LimitReading, error)
}

// All is the one registry of providers. A tool is listed only when its
// program can be found, so a missing tool is not an error.
// environ is the base environment given to the tool programs.
func All(lookPath func(string) (string, error), environ []string) []Provider {
	var providers []Provider
	if bin, err := lookPath(codexBinary); err == nil {
		providers = append(providers, Codex{Bin: bin, Env: environ})
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
		slices.SortFunc(nvm, func(a, b string) int { return compareNodeVersions(b, a) })
		dirs = append(dirs, nvm...)
	}
	return dirs
}

// compareNodeVersions orders two ".../vMAJOR.MINOR.PATCH/bin" folders by
// version number, so v9 is older than v25 (a text sort gets that wrong).
// Names that are not versions sort by text, after every real version.
func compareNodeVersions(a, b string) int {
	versionA, okA := nodeVersion(a)
	versionB, okB := nodeVersion(b)
	switch {
	case okA && okB:
		if order := slices.Compare(versionA, versionB); order != 0 {
			return order
		}
	case okA:
		return 1
	case okB:
		return -1
	}
	return strings.Compare(a, b)
}

// nodeVersion reads the numbers of "v25.5.0" from the folder above "bin".
func nodeVersion(binDir string) ([]int, bool) {
	name := strings.TrimPrefix(filepath.Base(filepath.Dir(binDir)), "v")
	var numbers []int
	for _, part := range strings.Split(name, ".") {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		numbers = append(numbers, number)
	}
	return numbers, true
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
// a task (which is when usage changes). After a failed read the wait is
// only retryAfterFailure, so one bad read does not hide the limits for
// minutes. lastPollMs is when the last read ended or was started.
func PollDue(lastPollMs *int64, lastFailed, saverActive, sourceFinishedSincePoll bool, nowMs int64) bool {
	if !saverActive {
		return false
	}
	if lastPollMs == nil || sourceFinishedSincePoll {
		return true
	}
	wait := pollInterval
	if lastFailed {
		wait = retryAfterFailure
	}
	return nowMs-*lastPollMs >= wait.Milliseconds()
}
