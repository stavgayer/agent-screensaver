// Package cli holds the commands that coding tools run on every event. They
// must always exit 0, stay fast and print nothing that could disturb the tool.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
)

const (
	// maxHookInputBytes bounds what a hook reads, so a bad writer cannot
	// make the hook allocate without limit.
	maxHookInputBytes = 1 << 20
	maxErrorLogBytes  = 1 << 20
	errorLogName      = "hook-errors.log"
	recordPayloadsEnv = "AGS_RECORD_PAYLOADS"
)

// Hook runs `agent-screensaver hook --tool <source> [--event <name>]`. It
// prints the agent's reply first, so the tool gets a valid answer whatever
// happens next, then maps the payload and writes the events. It always
// returns 0.
func Hook(args []string, stdin io.Reader, stdout io.Writer, p paths.Paths, now func() int64) (code int) {
	defer recoverToZero(p, now, &code)
	defer drain(stdin) // never leave the writer blocked on a full pipe
	flags := flag.NewFlagSet("hook", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // stdout and stderr must stay silent
	tool := flags.String("tool", "", "agent source")
	eventArg := flags.String("event", "", "event name, for tools that do not send it")
	if err := flags.Parse(args); err != nil {
		logError(p, now, fmt.Errorf("hook flags: %w", err))
		return 0
	}
	agent, ok := findAgent(p, now, *tool)
	if !ok {
		return 0
	}
	_, _ = io.WriteString(stdout, agent.Hooks.Reply.Text(*eventArg))
	if config.Load(p.ConfigFile).AgentOff(agent.ID) {
		return 0
	}
	return RunHook(agent, *eventArg, stdin, p, now)
}

// findAgent loads the agent extensions and picks the one for source. The
// manifests are read on every call: a hook is a short process, and reading
// fifty small files stays well inside the time budget.
func findAgent(p paths.Paths, now func() int64, source string) (agents.Agent, bool) {
	loaded, err := agents.Load(p.ExtensionsDir)
	agent, ok := agents.BySource(loaded, source)
	if !ok {
		if err != nil {
			logError(p, now, err)
		}
		logError(p, now, fmt.Errorf("unknown tool %q", source))
	}
	return agent, ok
}

// RunHook maps one hook payload to events and writes them to the spool. It
// always returns 0 and prints nothing.
func RunHook(agent agents.Agent, eventArg string, stdin io.Reader, p paths.Paths, now func() int64) (code int) {
	defer recoverToZero(p, now, &code)
	input, err := io.ReadAll(io.LimitReader(stdin, maxHookInputBytes))
	if err != nil {
		logError(p, now, fmt.Errorf("read hook input: %w", err))
	}
	drain(stdin)
	atMs := now()
	if os.Getenv(recordPayloadsEnv) == "1" {
		recordPayload(p, now, agent.Source, payloadName(input, eventArg), atMs, input)
	}
	env := agents.ConverterEnv(p.Home, os.Getenv("PATH"), agent.Source, eventArg)
	events, err := agent.Hooks.Events(context.Background(), input, eventArg, env, atMs)
	if err != nil {
		logError(p, now, fmt.Errorf("%s hook: %w", agent.ID, err))
	}
	for _, mapped := range events {
		if err := spool.Write(p.SpoolDir, mapped); err != nil {
			logError(p, now, fmt.Errorf("write event: %w", err))
		}
	}
	return 0
}

func drain(reader io.Reader) {
	_, _ = io.Copy(io.Discard, reader)
}

// payloadName names a recorded payload file: the --event value, else the
// hook_event_name most tools send.
func payloadName(input []byte, eventArg string) string {
	name := eventArg
	if name == "" {
		var named struct {
			Name string `json:"hook_event_name"`
		}
		if json.Unmarshal(input, &named) == nil {
			name = named.Name
		}
	}
	if name == "" {
		return "unknown"
	}
	return filepath.Base(name) // keep it a single file name part
}

// recordPayload saves the raw input so real tool payloads can be captured
// for fixtures.
func recordPayload(p paths.Paths, now func() int64, tool, eventName string, atMs int64, input []byte) {
	dir := filepath.Join(p.DataDir, "payloads")
	name := fmt.Sprintf("%s-%s-%d.json", tool, eventName, atMs)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logError(p, now, fmt.Errorf("create payload dir: %w", err))
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), input, 0o600); err != nil {
		logError(p, now, fmt.Errorf("record payload: %w", err))
	}
}

// logError appends one line to the error log. Logging must never fail the
// hook, so its own errors are dropped.
func logError(p paths.Paths, now func() int64, cause error) {
	if err := os.MkdirAll(p.DataDir, 0o700); err != nil {
		return
	}
	path := filepath.Join(p.DataDir, errorLogName)
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > maxErrorLogBytes {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC // bound the log size
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	stamp := time.UnixMilli(now()).UTC().Format(time.RFC3339)
	_, _ = fmt.Fprintf(file, "%s %v\n", stamp, cause)
}

// recoverToZero must be deferred directly. A Go panic exits with status 2,
// which can block the tool call that ran us, so a panic is logged and turned
// into exit code 0.
func recoverToZero(p paths.Paths, now func() int64, code *int) {
	if recovered := recover(); recovered != nil {
		logError(p, now, fmt.Errorf("panic: %v", recovered))
		*code = 0
	}
}
