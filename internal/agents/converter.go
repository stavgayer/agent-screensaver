package agents

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/text"
)

// Converter limits (docs/agents.md "Converter programs").
const (
	DefaultConverterTimeout        = 200 * time.Millisecond
	MaxConverterTimeout            = 500 * time.Millisecond
	DefaultDetailsConverterTimeout = time.Second
	MaxDetailsConverterTimeout     = 2 * time.Second

	// MaxConverterInput is the most payload a converter is given.
	MaxConverterInput = 1 << 20
	// MaxConverterOutput caps what a converter may print, in total.
	MaxConverterOutput = 64 << 10
	// MaxConverterEvents is the most events one payload can give.
	MaxConverterEvents = 4

	// killGrace bounds the wait for the output pipe after the kill, in case
	// a grandchild still holds it.
	killGrace = 100 * time.Millisecond
)

// Converter is a program in the extension folder that maps a payload.
type Converter struct {
	// Run is the full path of the program.
	Run     string
	Timeout time.Duration
}

type converterManifest struct {
	Run       string `json:"run"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// parseConverter checks the program: a regular file with the execute bit,
// inside the extension folder, and a timeout within limit.
func parseConverter(form converterManifest, dir string, limit time.Duration) (Converter, error) {
	if !filepath.IsLocal(form.Run) {
		return Converter{}, fmt.Errorf("converter %q must be a path inside the extension folder", form.Run)
	}
	timeout := time.Duration(form.TimeoutMs) * time.Millisecond
	if form.TimeoutMs == 0 {
		timeout = DefaultConverterTimeout
	}
	if timeout < 0 || timeout > limit {
		return Converter{}, fmt.Errorf("converter timeout_ms is %d, allowed 1 to %d", form.TimeoutMs, limit.Milliseconds())
	}
	path := filepath.Join(dir, filepath.Clean(form.Run))
	info, err := os.Lstat(path)
	if err != nil {
		return Converter{}, fmt.Errorf("converter: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return Converter{}, fmt.Errorf("converter %s must be a regular file with the execute bit", form.Run)
	}
	return Converter{Run: path, Timeout: timeout}, nil
}

// ConverterEnv is the whole environment a converter gets.
func ConverterEnv(home, path, source, eventArg string) []string {
	return []string{"HOME=" + home, "PATH=" + path, "AGS_SOURCE=" + source, "AGS_EVENT=" + eventArg}
}

// RunConverter runs the program directly (no shell, no arguments) with the
// payload on stdin and returns the events it printed, one JSON object per
// line. On a timeout the whole process group is killed and nothing is
// returned. The caller forces the source and checks each event (Normalize).
func RunConverter(ctx context.Context, c Converter, payload []byte, env []string) ([]event.Event, error) {
	output, err := RunProgram(ctx, c, payload, env)
	if err != nil {
		return nil, err
	}
	return parseConverterOutput(output), nil
}

// RunProgram runs a converter and returns what it printed, at most
// MaxConverterOutput bytes. A timeout, a failure or more output is an error.
func RunProgram(ctx context.Context, c Converter, input []byte, env []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	child := exec.CommandContext(ctx, c.Run)
	child.Stdin = bytes.NewReader(input[:min(len(input), MaxConverterInput)])
	child.Env = env
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Cancel = func() error { return syscall.Kill(-child.Process.Pid, syscall.SIGKILL) }
	child.WaitDelay = killGrace
	output := &cappedBuffer{limit: MaxConverterOutput}
	child.Stdout = output // stderr stays nil, so it is discarded
	err := child.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("converter %s timed out after %v", c.Run, c.Timeout)
	}
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return nil, fmt.Errorf("converter %s: %w", c.Run, err)
	}
	if output.over {
		return nil, fmt.Errorf("converter %s printed more than 64 KiB", c.Run)
	}
	return output.data, nil
}

// parseConverterOutput keeps the first MaxConverterEvents lines that are
// JSON objects. Other lines are dropped.
func parseConverterOutput(output []byte) []event.Event {
	var events []event.Event
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 4096), MaxConverterOutput+1)
	for scanner.Scan() && len(events) < MaxConverterEvents {
		line := bytes.TrimSpace(scanner.Bytes())
		var parsed event.Event
		if len(line) == 0 || json.Unmarshal(line, &parsed) != nil {
			continue
		}
		events = append(events, parsed)
	}
	return events
}

// cappedBuffer keeps at most limit bytes and notes when more came. It
// does not embed bytes.Buffer: io.Copy would use its ReadFrom and skip the cap.
type cappedBuffer struct {
	data  []byte
	limit int
	over  bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	room := b.limit - len(b.data)
	if len(data) > room {
		b.over = true
		b.data = append(b.data, data[:max(room, 0)]...)
	} else {
		b.data = append(b.data, data...)
	}
	return len(data), nil // keep reading, so the program is never blocked on a full pipe
}

// Normalize makes an event safe to write: it forces the source, fills the
// version and time when missing, masks and cuts every text that can reach
// the screen, and checks the event with the normal event rules. An info
// event with no details and a limits event with no readings are refused,
// because they say nothing.
func Normalize(e event.Event, source string, nowMs int64) (event.Event, error) {
	e.Source = source
	if e.V == 0 {
		e.V = event.Version
	}
	if e.AtMs == 0 {
		e.AtMs = nowMs
	}
	e.Title = text.ShortTitle(e.Title)
	e.Name = text.ShortTitle(e.Name)
	e.Project = text.MaskSecrets(e.Project)
	e.Model = text.ShortModel(text.MaskSecrets(e.Model))
	if e.Kind == event.KindLimits {
		if err := event.ValidateIdentifier("session", e.Session); err != nil {
			e.Session = LimitsSession
		}
		for index := range e.Limits {
			e.Limits[index].UsedPercent = min(max(e.Limits[index].UsedPercent, 0), 100)
		}
	}
	if err := e.Validate(); err != nil {
		return event.Event{}, err
	}
	if !saysSomething(e) {
		return event.Event{}, fmt.Errorf("%s event carries nothing", e.Kind)
	}
	return e, nil
}

func saysSomething(e event.Event) bool {
	switch e.Kind {
	case event.KindLimits:
		return len(e.Limits) > 0
	case event.KindInfo:
		return e.Name != "" || e.Model != "" || e.ContextPercent != nil
	}
	return true
}
