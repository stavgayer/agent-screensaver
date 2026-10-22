package limits

// Codex limits come from `codex app-server`, which speaks newline-delimited
// JSON-RPC (no "jsonrpc" field) on stdin and stdout. Checked against
// codex-cli 0.144.4 and 0.162.0, whose JSON schema comes from
// `codex app-server generate-json-schema`. The handshake, in order:
//
//  1. client request      {"id":1,"method":"initialize","params":{"clientInfo":{"name":"agent-screensaver","version":"1"}}}
//  2. server response     {"id":1,"result":{"userAgent":...,"codexHome":...}}
//  3. client notification {"method":"initialized"}
//  4. client request      {"id":2,"method":"account/rateLimits/read"}
//  5. server response     {"id":2,"result":{"rateLimits":{"primary":{...},"secondary":{...}},...}}
//
// The server may send notifications (lines without "id") in between; they
// are skipped. This only reads: no Codex state or config is written.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"agent-screensaver/internal/event"
)

const (
	codexBinary       = "codex"
	rateLimitsRequest = 2
	// Resets above this are milliseconds: as seconds it would be year 33658.
	millisecondsThreshold = int64(1e12)
	handshake             = `{"id":1,"method":"initialize","params":{"clientInfo":{"name":"agent-screensaver","version":"1"}}}` + "\n" +
		`{"method":"initialized"}` + "\n" +
		`{"id":2,"method":"account/rateLimits/read"}` + "\n"
)

// Codex reads limits through the Codex app-server.
type Codex struct {
	Bin string
	// Env is the environment the child starts with. The caller passes it in
	// (os.Environ() in the daemon) so tests never read the real one.
	Env []string
}

// Source implements Provider.
func (Codex) Source() string { return codexBinary }

// Read starts the app-server, asks for the rate limits and stops it again.
// The child runs in its own process group and the whole group is killed on
// return, so no Codex process is left behind.
func (codex Codex) Read(ctx context.Context) ([]event.LimitReading, error) {
	cmd := exec.CommandContext(ctx, codex.Bin, "app-server")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd) }
	// codex is a script with "#!/usr/bin/env node". Under launchd the PATH
	// has no node, so its own folder (where nvm keeps node) goes first.
	cmd.Env = codexEnv(codex.Env, filepath.Dir(codex.Bin))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	defer func() {
		_ = killGroup(cmd)
		_ = cmd.Wait()
	}()

	if _, err := io.WriteString(stdin, handshake); err != nil {
		return nil, fmt.Errorf("write codex handshake: %w", err)
	}
	result, err := awaitResult(stdout, rateLimitsRequest)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("read codex rate limits: %w", ctx.Err())
		}
		return nil, err
	}
	return ParseCodexRateLimits(result), nil
}

// codexEnv returns base with PATH replaced by binDir followed by the old
// PATH. An empty old PATH adds nothing: a trailing empty entry would mean
// the current folder.
func codexEnv(base []string, binDir string) []string {
	const prefix = "PATH="
	pathValue := ""
	env := make([]string, 0, len(base)+1)
	for _, entry := range base {
		if value, ok := strings.CutPrefix(entry, prefix); ok {
			pathValue = value
			continue
		}
		env = append(env, entry)
	}
	if pathValue != "" {
		binDir += string(os.PathListSeparator) + pathValue
	}
	return append(env, prefix+binDir)
}

func killGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

type rpcMessage struct {
	ID     *int            `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// awaitResult reads lines until the response to request id arrives.
func awaitResult(stdout io.Reader, id int) (json.RawMessage, error) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if message, ok := decodeResponse(line, id); ok {
			if message.Error != nil {
				return nil, fmt.Errorf("codex rate limits request failed: %s", message.Error.Message)
			}
			return message.Result, nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, errors.New("codex app-server closed before answering")
			}
			return nil, fmt.Errorf("read codex output: %w", err)
		}
	}
}

// decodeResponse accepts only valid JSON lines that carry the wanted id;
// logs and notifications fall through.
func decodeResponse(line []byte, id int) (rpcMessage, bool) {
	var message rpcMessage
	if json.Unmarshal(line, &message) != nil || message.Method != "" || message.ID == nil || *message.ID != id {
		return rpcMessage{}, false
	}
	return message, true
}

type rateLimitWindow struct {
	UsedPercent *float64 `json:"usedPercent"`
	ResetsAt    *int64   `json:"resetsAt"`
}

// flexNumber reads a JSON number or a numeric string; Codex sends both.
type flexNumber struct{ value *float64 }

func (number *flexNumber) UnmarshalJSON(data []byte) error {
	trimmed := strings.Trim(string(data), `"`)
	if parsed, err := strconv.ParseFloat(trimmed, 64); err == nil {
		number.value = &parsed
	}
	return nil
}

// individualLimit is the limit of usage-based plans, which have no windows.
type individualLimit struct {
	Limit            flexNumber `json:"limit"`
	Used             flexNumber `json:"used"`
	RemainingPercent flexNumber `json:"remainingPercent"`
	ResetsAt         *int64     `json:"resetsAt"`
}

type rateLimitsResult struct {
	RateLimits struct {
		Primary         *rateLimitWindow `json:"primary"`
		Secondary       *rateLimitWindow `json:"secondary"`
		IndividualLimit *individualLimit `json:"individualLimit"`
	} `json:"rateLimits"`
}

// ParseCodexRateLimits turns the result of `account/rateLimits/read` into
// readings: primary is the 5 hour window, secondary the weekly one. A window
// that is absent or has no percent is skipped. Usage-based plans have no
// windows; their individual limit becomes one "plan" reading.
func ParseCodexRateLimits(resp []byte) []event.LimitReading {
	var result rateLimitsResult
	if json.Unmarshal(resp, &result) != nil {
		return nil
	}
	var readings []event.LimitReading
	for _, entry := range []struct {
		window event.Window
		data   *rateLimitWindow
	}{
		{event.FiveHour, result.RateLimits.Primary},
		{event.Week, result.RateLimits.Secondary},
	} {
		if reading, ok := toReading(entry.window, entry.data); ok {
			readings = append(readings, reading)
		}
	}
	if reading, ok := planReading(result.RateLimits.IndividualLimit); ok {
		readings = append(readings, reading)
	}
	return readings
}

func planReading(limit *individualLimit) (event.LimitReading, bool) {
	if limit == nil {
		return event.LimitReading{}, false
	}
	percent, ok := usedPercent(limit)
	if !ok {
		return event.LimitReading{}, false
	}
	return event.LimitReading{Window: event.Plan, UsedPercent: clampPercent(percent), ResetsAt: toSeconds(limit.ResetsAt)}, true
}

// usedPercent prefers the remaining percent the server computed, and falls
// back to used divided by limit.
func usedPercent(limit *individualLimit) (float64, bool) {
	if remaining := limit.RemainingPercent.value; remaining != nil {
		return 100 - *remaining, true
	}
	if limit.Used.value != nil && limit.Limit.value != nil && *limit.Limit.value != 0 {
		return *limit.Used.value / *limit.Limit.value * 100, true
	}
	return 0, false
}

func clampPercent(percent float64) float64 { return min(max(percent, 0), 100) }

func toSeconds(resetsAt *int64) *int64 {
	if resetsAt == nil {
		return nil
	}
	seconds := *resetsAt
	if seconds > millisecondsThreshold {
		seconds /= 1000
	}
	return &seconds
}

func toReading(window event.Window, data *rateLimitWindow) (event.LimitReading, bool) {
	if data == nil || data.UsedPercent == nil {
		return event.LimitReading{}, false
	}
	reading := event.LimitReading{Window: window, UsedPercent: clampPercent(*data.UsedPercent), ResetsAt: toSeconds(data.ResetsAt)}
	return reading, true
}
