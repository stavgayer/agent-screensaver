// Package sessioninfo reads the details a card shows from a session's own
// files: the conversation name, how full the context window is, and the git
// branch. The service calls it, never a hook, because hooks must exit fast.
// Every read is bounded: only the last TailBytes of a log are looked at.
package sessioninfo

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"agent-screensaver/internal/state"
	"agent-screensaver/internal/text"
)

// TailBytes is how much of the end of a log is read. Claude re-writes its
// titles and Codex its token counts often, so the latest copy sits near the
// end; 256 KB holds hundreds of lines of a real rollout.
const TailBytes = 256 << 10

// headBytes bounds the read of a small git file such as HEAD.
const headBytes = 4 << 10

// Reader reads the details of a card by its source.
type Reader struct {
	// CodexSessionIndex is Codex's session_index.jsonl, where thread names
	// are written.
	CodexSessionIndex string
	// Home is where the search for a git folder stops going up.
	Home string
}

// Read returns what the session's files say. Empty fields mean "not found".
func (r Reader) Read(card state.Card) state.Details {
	details := state.Details{Branch: Branch(card.Cwd, r.Home)}
	switch card.Source {
	case "claude":
		// The context fullness of Claude comes from the status line: the
		// transcript does not have the window size.
		details.Name = ClaudeName(card.Transcript, card.Session)
	case "codex":
		details.Name = CodexName(r.CodexSessionIndex, card.Session)
		details.ContextPercent = CodexContext(card.Transcript)
	}
	return details
}

// ClaudeName returns the conversation name in a Claude transcript: the last
// custom title (set with /rename or in the desktop app), else the last title
// Claude made up. Titles of other sessions are skipped: a resumed transcript
// can carry them.
func ClaudeName(transcript, session string) string {
	var custom, generated string
	eachLastLine(transcript, func(line []byte) bool {
		if !bytes.Contains(line, []byte(`-title"`)) {
			return true
		}
		var entry struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AITitle     string `json:"aiTitle"`
			SessionID   string `json:"sessionId"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.SessionID != session {
			return true
		}
		switch {
		case entry.Type == "custom-title" && entry.CustomTitle != "":
			custom = entry.CustomTitle
			return false
		case entry.Type == "ai-title" && generated == "":
			generated = entry.AITitle
		}
		return true
	})
	if custom != "" {
		return text.ShortTitle(custom)
	}
	return text.ShortTitle(generated)
}

// CodexName returns the last name Codex wrote for a thread in its session
// index. Codex writes it about five seconds into the first turn.
func CodexName(index, session string) string {
	var name string
	eachLastLine(index, func(line []byte) bool {
		if !bytes.Contains(line, []byte(session)) {
			return true
		}
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.ID != session || entry.ThreadName == "" {
			return true
		}
		name = entry.ThreadName
		return false
	})
	return text.ShortTitle(name)
}

// CodexContext returns how full the context window is, in whole percent,
// from the last token count in a Codex rollout. It uses the last call's
// tokens: the running total counts every call of the thread and is far
// larger than the window. nil means unknown.
func CodexContext(rollout string) *float64 {
	var found *float64
	eachLastLine(rollout, func(line []byte) bool {
		if !bytes.Contains(line, []byte(`"token_count"`)) {
			return true
		}
		var entry struct {
			Payload struct {
				Type string `json:"type"`
				Info *struct {
					LastTokenUsage struct {
						TotalTokens float64 `json:"total_tokens"`
					} `json:"last_token_usage"`
					ModelContextWindow float64 `json:"model_context_window"`
				} `json:"info"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Payload.Type != "token_count" || entry.Payload.Info == nil {
			return true
		}
		info := entry.Payload.Info
		if info.ModelContextWindow > 0 {
			used := math.Round(min(max(info.LastTokenUsage.TotalTokens/info.ModelContextWindow*100, 0), 100))
			found = &used
		}
		// The latest count decides, even when it has no window size.
		return false
	})
	return found
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

// eachLastLine calls visit with the complete lines of the last TailBytes of
// a file, newest first, until visit returns false. A missing or unreadable
// file has no lines.
func eachLastLine(path string, visit func(line []byte) bool) {
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
	offset := max(info.Size()-TailBytes, 0)
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
