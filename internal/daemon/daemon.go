// Package daemon is the always-running service. It drains the spool, keeps
// the state, writes state.json for the screen saver, polls usage limits and
// asks for the attention sound.
package daemon

import (
	"context"
	"log"
	"os"
	"reflect"
	"slices"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/limits"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
	"agent-screensaver/internal/view"
)

const (
	// rewriteEveryMs refreshes generated_at_ms, so the saver can tell a live
	// daemon from a dead one even when nothing changed.
	rewriteEveryMs = 30_000
	readTimeout    = 10 * time.Second
)

// tickInterval is how often Run calls Step. Polling the spool twice a second
// costs nothing measurable; switch to FSEvents only if profiling says so.
// It is a variable so tests can shorten it.
var tickInterval = 500 * time.Millisecond

// ShouldPlaySound is true only when a card starts waiting for approval while
// the saver is showing and the user wants the sound.
func ShouldPlaySound(prevAttention, nowAttention, saverActive, soundEnabled bool) bool {
	return !prevAttention && nowAttention && saverActive && soundEnabled
}

// StepResult tells Run what to do after one step.
type StepResult struct {
	StateWritten bool
	PlaySound    bool
	PollSources  []string // sources whose limits are due now
	WriteErr     error    // the state file could not be written; Step retries
}

// fileStamp identifies a version of a file or folder by its modification time.
type fileStamp struct {
	modNs  int64
	exists bool
}

func stampOf(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{modNs: info.ModTime().UnixNano(), exists: true}
}

// Daemon holds the state between steps. Step and SetLimits must be called
// from one goroutine, which is how Run uses them.
type Daemon struct {
	// Logf reports problems. It defaults to the standard logger (stderr).
	Logf func(format string, args ...any)

	paths paths.Paths
	state state.State
	cfg   config.Config
	view  view.View

	configStamp, extensionsStamp fileStamp
	viewLoaded                   bool

	attention    bool
	written      bool
	dirty        bool // a write failed or limits arrived; write on the next step
	lastWriteMs  int64
	pendingLimit []event.Event

	sources  []string
	lastPoll map[string]int64
	finished map[string]bool // source finished a task since its last poll
}

// New returns a daemon that keeps its files under p.
func New(p paths.Paths) *Daemon {
	return &Daemon{
		Logf:     log.Printf,
		paths:    p,
		lastPoll: map[string]int64{},
		finished: map[string]bool{},
	}
}

// SetSources names the tools whose limits are polled.
func (d *Daemon) SetSources(sources ...string) {
	d.sources = slices.Clone(sources)
}

// SoundFile is the sound to play when Step asks for it.
func (d *Daemon) SoundFile() string { return d.cfg.SoundFile }

// SetLimits queues a provider reading; the next Step folds it into the state.
func (d *Daemon) SetLimits(source string, readings []event.LimitReading, nowMs int64) {
	d.pendingLimit = append(d.pendingLimit, event.Event{
		V: event.Version, Source: source, Kind: event.KindLimits, AtMs: nowMs, Limits: readings,
	})
}

// Step does one round of work: no goroutines, no sleeping, only file IO.
func (d *Daemon) Step(nowMs int64, saverActive bool) StepResult {
	changed := d.applyPendingLimits()
	changed = d.drainSpool() || changed
	changed = d.refreshView() || changed
	changed = d.tick(nowMs) || changed

	attention := d.state.NeedsAttention()
	result := StepResult{
		PlaySound:   ShouldPlaySound(d.attention, attention, saverActive, d.cfg.Sound),
		PollSources: d.duePolls(nowMs, saverActive),
	}
	d.attention = attention

	if changed || d.dirty || !d.written || nowMs-d.lastWriteMs >= rewriteEveryMs {
		d.writeState(nowMs, &result)
	}
	return result
}

func (d *Daemon) applyPendingLimits() bool {
	for _, pending := range d.pendingLimit {
		d.state.Apply(pending)
	}
	changed := len(d.pendingLimit) > 0
	d.pendingLimit = nil
	return changed
}

func (d *Daemon) drainSpool() bool {
	events := spool.Drain(d.paths.SpoolDir)
	for _, incoming := range events {
		d.state.Apply(incoming)
		if incoming.Kind == event.KindFinished && slices.Contains(d.sources, incoming.Source) {
			d.finished[incoming.Source] = true
		}
	}
	return len(events) > 0
}

// refreshView resolves the view again only when the config file or the
// extensions folder changed. A folder's modification time changes when an
// extension is added or removed.
func (d *Daemon) refreshView() bool {
	configStamp, extensionsStamp := stampOf(d.paths.ConfigFile), stampOf(d.paths.ExtensionsDir)
	if d.viewLoaded && configStamp == d.configStamp && extensionsStamp == d.extensionsStamp {
		return false
	}
	d.configStamp, d.extensionsStamp, d.viewLoaded = configStamp, extensionsStamp, true
	d.cfg = config.Load(d.paths.ConfigFile)
	d.view = view.Resolve(d.cfg, extensions.Scan(d.paths.ExtensionsDir))
	return true
}

// tick ages the cards and reports whether that changed anything.
func (d *Daemon) tick(nowMs int64) bool {
	before := slices.Clone(d.state.Cards)
	d.state.Tick(nowMs)
	return !reflect.DeepEqual(before, d.state.Cards)
}

func (d *Daemon) duePolls(nowMs int64, saverActive bool) []string {
	var due []string
	for _, source := range d.sources {
		var last *int64
		if polledAt, ok := d.lastPoll[source]; ok {
			last = &polledAt
		}
		if !limits.PollDue(last, saverActive, d.finished[source], nowMs) {
			continue
		}
		// Marked now, not when the read ends, so a failing tool is not
		// retried on every tick.
		d.lastPoll[source] = nowMs
		delete(d.finished, source)
		due = append(due, source)
	}
	return due
}

func (d *Daemon) writeState(nowMs int64, result *StepResult) {
	file := statefile.Build(d.state, d.view, nowMs)
	if err := statefile.WriteAtomic(d.paths.StateFile, file); err != nil {
		d.dirty = true
		result.WriteErr = err
		return
	}
	d.dirty, d.written, d.lastWriteMs = false, true, nowMs
	result.StateWritten = true
}

// Run steps the daemon until ctx is cancelled. Provider reads run in their
// own goroutines, one per provider at a time, and come back through a channel
// so only this goroutine touches the daemon.
func Run(ctx context.Context, d *Daemon, w platform.SaverWatcher, s platform.SoundPlayer, providers []limits.Provider) error {
	loop := newLoop(ctx, d, w, s, providers)
	loop.step()
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case read := <-loop.results:
			loop.finishRead(read)
		case <-ticker.C:
			loop.step()
		}
	}
}

type readResult struct {
	source   string
	readings []event.LimitReading
	err      error
}

type loop struct {
	ctx       context.Context
	daemon    *Daemon
	watcher   platform.SaverWatcher
	sound     platform.SoundPlayer
	providers map[string]limits.Provider
	inFlight  map[string]bool
	results   chan readResult // buffered for every provider, so a read never blocks
	saver     bool
	writeErr  string
}

func newLoop(ctx context.Context, d *Daemon, w platform.SaverWatcher, s platform.SoundPlayer, providers []limits.Provider) *loop {
	byName := make(map[string]limits.Provider, len(providers))
	sources := make([]string, 0, len(providers))
	for _, provider := range providers {
		byName[provider.Source()] = provider
		sources = append(sources, provider.Source())
	}
	d.SetSources(sources...)
	return &loop{
		ctx: ctx, daemon: d, watcher: w, sound: s, providers: byName,
		inFlight: map[string]bool{},
		results:  make(chan readResult, len(providers)),
	}
}

func (l *loop) step() {
	nowMs := time.Now().UnixMilli()
	active := l.watcher.Active()
	if active != l.saver {
		l.saver = active
		l.daemon.Logf("saver active: %t", active)
	}
	result := l.daemon.Step(nowMs, active)
	l.reportWriteError(result.WriteErr)
	if result.PlaySound {
		if err := l.sound.Play(l.daemon.SoundFile()); err != nil {
			l.daemon.Logf("play sound: %v", err)
		}
	}
	for _, source := range result.PollSources {
		l.startRead(source)
	}
}

// reportWriteError logs a failure once, not on every retry.
func (l *loop) reportWriteError(err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if message != l.writeErr && message != "" {
		l.daemon.Logf("write state file: %s", message)
	}
	l.writeErr = message
}

func (l *loop) startRead(source string) {
	if l.inFlight[source] {
		return
	}
	l.inFlight[source] = true
	provider := l.providers[source]
	go func() {
		ctx, cancel := context.WithTimeout(l.ctx, readTimeout)
		defer cancel()
		readings, err := provider.Read(ctx)
		l.results <- readResult{source: source, readings: readings, err: err}
	}()
}

func (l *loop) finishRead(read readResult) {
	l.inFlight[read.source] = false
	if read.err != nil {
		// The previous limits stay on screen.
		l.daemon.Logf("read %s limits: %v", read.source, read.err)
		return
	}
	l.daemon.SetLimits(read.source, read.readings, time.Now().UnixMilli())
}
