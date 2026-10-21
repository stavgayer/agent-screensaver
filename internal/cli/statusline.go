package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/spool"
	"agent-screensaver/internal/statusline"
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

// RunStatusline writes the limits event, then runs the status line command
// the user had before and prints its output. It always returns 0.
func RunStatusline(stdin io.Reader, stdout io.Writer, p paths.Paths, now func() int64) (code int) {
	defer recoverToZero(p, now, &code)
	if os.Getenv(inStatuslineEnv) == "1" {
		return 0 // we were started by our own status line: stop the loop
	}
	input, err := io.ReadAll(io.LimitReader(stdin, maxHookInputBytes))
	if err != nil {
		logError(p, now, fmt.Errorf("read statusline input: %w", err))
	}
	drain(stdin)
	if limits, ok := statusline.LimitsEvent(input, now()); ok {
		if err := spool.Write(p.SpoolDir, limits); err != nil {
			logError(p, now, fmt.Errorf("write limits event: %w", err))
		}
	}
	previous := strings.TrimSpace(config.Load(p.ConfigFile).PreviousStatusline)
	if previous == "" || isOwnStatusline(previous) {
		return 0
	}
	if output, ok := RunPrevious(previous, input, previousTimeout); ok {
		_, _ = io.WriteString(stdout, output)
	}
	return 0
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
