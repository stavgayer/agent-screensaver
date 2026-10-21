// Package cli holds the commands that coding tools run on every event. They
// must always exit 0, stay fast and print nothing that could disturb the tool.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"agent-screensaver/internal/agents"
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

// Hook runs `agent-screensaver hook --tool <name>`. It always returns 0.
func Hook(args []string, stdin io.Reader, p paths.Paths, now func() int64) (code int) {
	defer recoverToZero(p, now, &code)
	flags := flag.NewFlagSet("hook", flag.ContinueOnError)
	flags.SetOutput(io.Discard) // stdout and stderr must stay silent
	tool := flags.String("tool", "", "tool name")
	if err := flags.Parse(args); err != nil {
		logError(p, now, fmt.Errorf("hook flags: %w", err))
		drain(stdin)
		return 0
	}
	adapter, ok := agents.ByName(*tool)
	if !ok {
		logError(p, now, fmt.Errorf("unknown tool %q", *tool))
		drain(stdin)
		return 0
	}
	return RunHook(adapter, stdin, p, now)
}

// RunHook maps one hook payload to an event and writes it to the spool. It
// always returns 0 and prints nothing.
func RunHook(adapter agents.Adapter, stdin io.Reader, p paths.Paths, now func() int64) (code int) {
	defer recoverToZero(p, now, &code)
	input, err := io.ReadAll(io.LimitReader(stdin, maxHookInputBytes))
	if err != nil {
		logError(p, now, fmt.Errorf("read hook input: %w", err))
	}
	drain(stdin) // never leave the writer blocked on a full pipe
	atMs := now()
	if os.Getenv(recordPayloadsEnv) == "1" {
		recordPayload(p, now, adapter.Name(), hookEventName(input), atMs, input)
	}
	mapped, ok := adapter.Map(input, atMs)
	if !ok {
		return 0 // an ignored event is normal, not an error
	}
	if err := spool.Write(p.SpoolDir, mapped); err != nil {
		logError(p, now, fmt.Errorf("write event: %w", err))
	}
	return 0
}

func drain(reader io.Reader) {
	_, _ = io.Copy(io.Discard, reader)
}

// hookEventName reads the event name for the payload file name.
func hookEventName(input []byte) string {
	var named struct {
		Name string `json:"hook_event_name"`
	}
	if json.Unmarshal(input, &named) != nil || named.Name == "" {
		return "unknown"
	}
	return filepath.Base(named.Name) // keep it a single file name part
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
