// Package sessioninfo reads the details a card shows from a session's own
// files: the conversation name, how full the context window is, the model
// and the git branch. Where to read is data in each agent extension
// (agent.details, docs/agents.md); this package is the one reader for every
// agent. The service calls it, never a hook, because hooks must exit fast.
// Every read is bounded and only reads files under the user's home.
package sessioninfo

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/state"
)

// headBytes bounds the read of a small git file such as HEAD.
const headBytes = 4 << 10

// Reader reads the details of a card.
type Reader struct {
	// Home is the user's home: every file read must be inside it, and the
	// search for a git folder stops there.
	Home string
	// ContextSizes holds the window sizes learned from the status line;
	// nil means none.
	ContextSizes *ContextSizes
}

// Read returns what the session's files say, by the agent's details (nil
// when the agent declares none). The branch is read for every card. Empty
// fields mean "not found".
func (r Reader) Read(details *agents.Details, card state.Card) state.Details {
	found := state.Details{Branch: Branch(card.Cwd, r.Home)}
	if details == nil {
		return found
	}
	read := detailsRead{reader: r, card: card, converted: map[string]converterOutput{}}
	if value, ok := read.first(details.Name, "name"); ok {
		found.Name, _ = value.(string)
	}
	if value, ok := read.first(details.Model, "model"); ok {
		found.Model, _ = value.(string)
	}
	if value, ok := read.first(details.ContextPercent, "context_percent"); ok {
		if percent, isNumber := value.(float64); isNumber {
			clamped := min(max(percent, 0), 100)
			found.ContextPercent = &clamped
		}
	}
	found.Name = cleanName(found.Name)
	found.Model = cleanModel(found.Model)
	return found
}

// defaultTailBytes is how much of a transcript the Claude context
// provider reads: the latest turn sits near the end.
const defaultTailBytes = agents.DefaultTailKB << 10

// Claude context window sizes, in tokens. The transcript does not say the
// window size. In order: a "[1m]" or "(1M context)" marker in the model
// name means the 1M window; a size learned from the status line for the
// model (ContextSizes); a turn larger than the default window can only fit
// the 1M window; a model that has a 1M choice is unknown (better no bar
// than one five times too full); any other Claude model has the default.
const (
	defaultClaudeContext = 200_000
	millionContext       = 1_000_000
)

// claudeModelPrefix marks the Claude models the default size is known for.
// "<synthetic>" lines (local error messages) and other names are unknown.
const claudeModelPrefix = "claude-"

var millionMarkers = []string{"[1m]", "(1M context)"}

// millionChoiceModels are the model families that can run with the 1M
// window, so the default size cannot be assumed for them.
var millionChoiceModels = []string{"claude-opus-", "claude-sonnet-"}

// ClaudeContext returns how full the context window is, in whole percent,
// from the last answer in a Claude transcript: the input, cache read and
// cache write tokens of that turn over the window size. nil means unknown,
// for example when the model is not a known Claude model. sizes may be nil.
func ClaudeContext(transcript string, sizes *ContextSizes) *float64 {
	var found *float64
	eachLastLine(transcript, defaultTailBytes, func(line []byte) bool {
		if !bytes.Contains(line, []byte(`"usage"`)) {
			return true
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
				Usage *struct {
					InputTokens              float64 `json:"input_tokens"`
					CacheCreationInputTokens float64 `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     float64 `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" || entry.Message.Usage == nil {
			return true
		}
		usage := entry.Message.Usage
		used := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
		if used == 0 {
			// Not a real call (a local error line): look further back.
			return true
		}
		if window, ok := contextWindow(entry.Message.Model, used, sizes); ok {
			percent := math.Round(min(used/window*100, 100))
			found = &percent
		}
		return false
	})
	return found
}

// contextWindow returns the window size of a model for a turn of used
// tokens, and false when it is not known.
func contextWindow(model string, used float64, sizes *ContextSizes) (float64, bool) {
	for _, marker := range millionMarkers {
		if strings.Contains(model, marker) {
			return millionContext, true
		}
	}
	if !strings.HasPrefix(model, claudeModelPrefix) {
		return 0, false
	}
	if used > defaultClaudeContext {
		return millionContext, true
	}
	if learned, ok := sizes.size(model); ok {
		return float64(learned), true
	}
	for _, family := range millionChoiceModels {
		if strings.HasPrefix(model, family) {
			return 0, false
		}
	}
	return defaultClaudeContext, true
}

// Branch returns the git branch checked out in cwd or a folder above it,
// the short commit when the head is detached, or "" when cwd is not in a
// repository. It does not go above stop (usually the home folder).
func Branch(cwd, stop string) string {
	if cwd == "" {
		return ""
	}
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		if gitDir, ok := findGitDir(dir); ok {
			return headBranch(filepath.Join(gitDir, "HEAD"))
		}
		if dir == stop || dir == filepath.Dir(dir) {
			return ""
		}
	}
}

// findGitDir returns the git folder of a repository whose top is dir. A
// worktree has a .git file that points to its git folder.
func findGitDir(dir string) (string, bool) {
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Stat(dotGit)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return dotGit, true
	}
	pointer, ok := readSmall(dotGit)
	if !ok {
		return "", false
	}
	target, found := strings.CutPrefix(pointer, "gitdir: ")
	if !found {
		return "", false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	return target, true
}

func headBranch(head string) string {
	content, ok := readSmall(head)
	if !ok {
		return ""
	}
	if ref, found := strings.CutPrefix(content, "ref: "); found {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	if len(content) == 40 && strings.Trim(content, "0123456789abcdef") == "" {
		return content[:7]
	}
	return ""
}

// readSmall reads the first line of a small file, trimmed.
func readSmall(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, headBytes))
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(string(content), "\n")
	return strings.TrimSpace(line), true
}

// eachLastLine calls visit with the complete lines of the last tailBytes of
// a file, newest first, until visit returns false. A missing or unreadable
// file has no lines.
func eachLastLine(path string, tailBytes int64, visit func(line []byte) bool) {
	if path == "" {
		return
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return
	}
	offset := max(info.Size()-tailBytes, 0)
	tail := make([]byte, info.Size()-offset)
	if _, err := file.ReadAt(tail, offset); err != nil && err != io.EOF {
		return
	}
	if offset > 0 {
		// The first line was cut by the offset.
		if newline := bytes.IndexByte(tail, '\n'); newline >= 0 {
			tail = tail[newline+1:]
		} else {
			return
		}
	}
	lines := bytes.Split(tail, []byte("\n"))
	for index := len(lines) - 1; index >= 0; index-- {
		if len(lines[index]) > 0 && !visit(lines[index]) {
			return
		}
	}
}
