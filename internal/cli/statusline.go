package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
)

const (
	previousTimeout = 300 * time.Millisecond
	// killGrace bounds the wait for the output pipe after the kill, in case a
	// grandchild still holds it.
	killGrace       = 100 * time.Millisecond
	ownBinaryName   = "agent-screensaver"
	statuslineWord  = "statusline"
	inStatuslineEnv = "AGS_IN_STATUSLINE"
)

// RunStatusline runs `agent-screensaver statusline [--tool <source>]`. It
// writes the events the agent's status line mapping gives, then runs the
// status line command the user had before and prints its output. It always
// returns 0.
func RunStatusline(args []string, stdin io.Reader, stdout io.Writer, p paths.Paths, now func() int64) (code int) {
	defer recoverToZero(p, now, &code)
	defer drain(stdin)
	if os.Getenv(inStatuslineEnv) == "1" {
		return 0 // we were started by our own status line: stop the loop
	}
	flags := flag.NewFlagSet("statusline", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	tool := flags.String("tool", "", "agent source")
	if err := flags.Parse(args); err != nil {
		logError(p, now, fmt.Errorf("statusline flags: %w", err))
		return 0
	}
	input, err := io.ReadAll(io.LimitReader(stdin, maxHookInputBytes))
	if err != nil {
		logError(p, now, fmt.Errorf("read statusline input: %w", err))
	}
	drain(stdin)
	cfg := config.Load(p.ConfigFile)
	if agent, ok := statuslineAgent(p, now, *tool); ok && !cfg.AgentOff(agent.ID) {
		writeStatuslineEvents(agent, input, p, now)
	}
	previous := strings.TrimSpace(cfg.PreviousStatusline)
	if previous == "" || isOwnStatusline(previous) {
		return 0
	}
	if output, ok := RunPrevious(previous, input, previousTimeout); ok {
		_, _ = io.WriteString(stdout, output)
	}
	return 0
}

// statuslineAgent finds the agent whose status line this is. The command
// written before agent extensions had no --tool; it means the one agent
// that has a status line mapping.
func statuslineAgent(p paths.Paths, now func() int64, source string) (agents.Agent, bool) {
	loaded, err := agents.Load(p.ExtensionsDir)
	var withStatusline []agents.Agent
	for _, agent := range loaded {
		if agent.Statusline != nil && (source == "" || agent.Source == source) {
			withStatusline = append(withStatusline, agent)
		}
	}
	if len(withStatusline) != 1 {
		if err != nil {
			logError(p, now, err)
		}
		logError(p, now, fmt.Errorf("no agent with a status line for tool %q", source))
		return agents.Agent{}, false
	}
	return withStatusline[0], true
}

func writeStatuslineEvents(agent agents.Agent, input []byte, p paths.Paths, now func() int64) {
	env := agents.ConverterEnv(p.Home, os.Getenv("PATH"), agent.Source, "")
	events, err := agent.Statusline.Events(context.Background(), input, "", env, now())
	if err != nil {
		logError(p, now, fmt.Errorf("%s status line: %w", agent.ID, err))
	}
	for _, mapped := range events {
		if err := spool.Write(p.SpoolDir, mapped); err != nil {
			logError(p, now, fmt.Errorf("write %s event: %w", mapped.Kind, err))
		}
	}
}

// isOwnStatusline reports whether the command is our own statusline command,
// which would call itself without end.
func isOwnStatusline(command string) bool {
	words := strings.Fields(command)
	return len(words) > 0 &&
		filepath.Base(words[0]) == ownBinaryName &&
		slices.Contains(words, statuslineWord)
}

// RunPrevious runs cmd through sh with stdin and returns its stdout. It
// reports false on failure or timeout; on timeout the whole process group is
// killed so a slow child cannot linger.
func RunPrevious(cmd string, stdin []byte, timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	child := exec.CommandContext(ctx, "sh", "-c", cmd)
	child.Stdin = strings.NewReader(string(stdin))
	child.Env = append(os.Environ(), inStatuslineEnv+"=1")
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child.Cancel = func() error { return syscall.Kill(-child.Process.Pid, syscall.SIGKILL) }
	child.WaitDelay = killGrace
	var output strings.Builder
	child.Stdout = &output // stderr stays nil, so it is discarded
	err := child.Run()
	// A command that leaves a background process holding stdout still
	// succeeded: WaitDelay only cut the wait for the pipe.
	if err != nil && !(errors.Is(err, exec.ErrWaitDelay) && ctx.Err() == nil) {
		return "", false
	}
	return output.String(), true
}
