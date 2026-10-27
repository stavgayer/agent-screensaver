package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/limits"
)

// logRecorder keeps the daemon's log lines; Run logs from its goroutine.
type logRecorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *logRecorder) logf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, fmt.Sprintf(format, args...))
}

func (r *logRecorder) has(line string) bool { return r.count(line) > 0 }

func (r *logRecorder) count(line string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(slices.DeleteFunc(slices.Clone(r.lines), func(logged string) bool { return logged != line }))
}

func TestLimitProvidersFollowTheInstalledAgents(t *testing.T) {
	previous := tickInterval
	tickInterval = 5 * time.Millisecond
	t.Cleanup(func() { tickInterval = previous })
	d, p := newTestDaemon(t)
	logs := &logRecorder{}
	d.Logf = logs.logf
	if err := os.MkdirAll(p.ExtensionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	provider := &fakeProvider{}
	d.LimitsProviders = func(installed []agents.Agent) []limits.Provider {
		var providers []limits.Provider
		for _, agent := range installed {
			if agent.Limits == agents.LimitsCodexAppServer {
				providers = append(providers, provider)
			}
		}
		return providers
	}
	watcher := &fakeWatcher{}
	watcher.active.Store(true) // limits are read only while the saver shows
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, d, watcher, &fakeSound{}, nil) }()
	eventually(t, "the first provider list", func() bool { return logs.has("limit providers: []") })

	// When an agent with a limits provider is installed, it is polled
	installBuiltinAgents(t, p)
	eventually(t, "the new provider", func() bool { return logs.has("limit providers: [codex]") })
	eventually(t, "a read of the new provider", func() bool { return provider.reads.Load() > 0 })

	// When it is removed, its provider goes
	if err := os.RemoveAll(filepath.Join(p.ExtensionsDir, "builtin.codex")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the provider to go", func() bool { return logs.count("limit providers: []") == 2 })
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
}
