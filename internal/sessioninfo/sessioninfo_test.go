package sessioninfo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/state"
)

const (
	claudeSession = "11111111-1111-4111-8111-111111111111"
	codexSession  = "33333333-3333-4333-8333-333333333333"
)

func fixture(name string) string { return filepath.Join("testdata", name) }

func TestClaudeName(t *testing.T) {
	tests := []struct {
		name       string
		transcript string
		session    string
		want       string
	}{
		{"latest ai title wins over an older one", fixture("claude_transcript.jsonl"), claudeSession, "Fix the login session expiry"},
		{"a custom title beats a later ai title", fixture("claude_transcript_renamed.jsonl"), claudeSession, "Login fix"},
		{"titles of other sessions and quoted text are ignored", fixture("claude_transcript_untitled.jsonl"), claudeSession, ""},
		{"missing file", fixture("missing.jsonl"), claudeSession, ""},
		{"no path", "", claudeSession, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClaudeName(tt.transcript, tt.session); got != tt.want {
				t.Errorf("ClaudeName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodexName(t *testing.T) {
	tests := []struct {
		name    string
		index   string
		session string
		want    string
	}{
		{"the last name of the thread", fixture("codex_session_index.jsonl"), codexSession, "Parser refactor, part two"},
		{"another thread", fixture("codex_session_index.jsonl"), "44444444-4444-4444-8444-444444444444", "Another thread"},
		{"unknown thread", fixture("codex_session_index.jsonl"), "99999999-9999-4999-8999-999999999999", ""},
		{"missing index", fixture("missing.jsonl"), codexSession, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CodexName(tt.index, tt.session); got != tt.want {
				t.Errorf("CodexName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodexContext(t *testing.T) {
	tests := []struct {
		name    string
		rollout string
		want    *float64
	}{
		// 199,867 of 258,400 is 77.3%: the last call, not the running total.
		{"last call against the window", fixture("codex_rollout.jsonl"), percent(77)},
		{"no window size means unknown", fixture("codex_rollout_no_window.jsonl"), nil},
		{"missing rollout", fixture("missing.jsonl"), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CodexContext(tt.rollout)
			if !equalPercent(got, tt.want) {
				t.Errorf("CodexContext = %v, want %v", show(got), show(tt.want))
			}
		})
	}
}

func TestBranch(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "repo", ".git", "HEAD"), "ref: refs/heads/feat/v1\n")
	writeFile(t, filepath.Join(root, "repo", "web", "src", "keep"), "")
	writeFile(t, filepath.Join(root, "detached", ".git", "HEAD"), "8781131d0c2a4e5f6a7b8c9d0e1f2a3b4c5d6e7f\n")
	writeFile(t, filepath.Join(root, "main-repo", ".git", "worktrees", "wt", "HEAD"), "ref: refs/heads/task-6.3\n")
	writeFile(t, filepath.Join(root, "wt", ".git"), "gitdir: "+filepath.Join(root, "main-repo", ".git", "worktrees", "wt")+"\n")
	writeFile(t, filepath.Join(root, "relative", ".git"), "gitdir: ../main-repo/.git/worktrees/wt\n")
	writeFile(t, filepath.Join(root, "broken", ".git", "HEAD"), "nonsense\n")
	writeFile(t, filepath.Join(root, "plain", "keep"), "")

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"branch of the repo", filepath.Join(root, "repo"), "feat/v1"},
		{"a folder inside the repo", filepath.Join(root, "repo", "web", "src"), "feat/v1"},
		{"detached head shows the short commit", filepath.Join(root, "detached"), "8781131"},
		{"worktree with an absolute gitdir", filepath.Join(root, "wt"), "task-6.3"},
		{"worktree with a relative gitdir", filepath.Join(root, "relative"), "task-6.3"},
		{"unreadable head", filepath.Join(root, "broken"), ""},
		{"not a repo", filepath.Join(root, "plain"), ""},
		{"no folder", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Branch(tt.cwd, root); got != tt.want {
				t.Errorf("Branch(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

// TestReadsOnlyTheTail checks the bound: a title written before the last
// TailBytes is not seen, and a huge file is read as fast as a small one.
func TestReadsOnlyTheTail(t *testing.T) {
	dir := t.TempDir()
	title := `{"type":"ai-title","aiTitle":"Early title","sessionId":"` + claudeSession + `"}` + "\n"
	filler := bytes.Repeat([]byte(`{"type":"assistant","message":{"content":"`+strings.Repeat("x", 1000)+`"}}`+"\n"), 5_000)
	early := filepath.Join(dir, "early.jsonl")
	writeFile(t, early, title+string(filler))
	late := filepath.Join(dir, "late.jsonl")
	writeFile(t, late, string(filler)+title)

	if got := ClaudeName(early, claudeSession); got != "" {
		t.Errorf("a title before the tail was found: %q", got)
	}
	started := time.Now()
	if got := ClaudeName(late, claudeSession); got != "Early title" {
		t.Errorf("ClaudeName = %q, want the title at the end", got)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Errorf("reading a 5 MB file took %v", elapsed)
	}
}

func TestReaderPicksBySource(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "repo", ".git", "HEAD"), "ref: refs/heads/main\n")
	cwd := filepath.Join(root, "repo")
	reader := Reader{CodexSessionIndex: fixture("codex_session_index.jsonl"), Home: root}

	tests := []struct {
		name string
		card state.Card
		want state.Details
	}{
		{"claude: name from the transcript, context from the status line only",
			state.Card{Source: "claude", Session: claudeSession, Cwd: cwd, Transcript: fixture("claude_transcript.jsonl")},
			state.Details{Name: "Fix the login session expiry", Branch: "main"}},
		{"codex: name from the index, context from the rollout",
			state.Card{Source: "codex", Session: codexSession, Cwd: cwd, Transcript: fixture("codex_rollout.jsonl")},
			state.Details{Name: "Parser refactor, part two", ContextPercent: percent(77), Branch: "main"}},
		{"another source gets only the branch",
			state.Card{Source: "cursor", Session: "c1", Cwd: cwd, Transcript: fixture("claude_transcript.jsonl")},
			state.Details{Branch: "main"}},
		{"nothing known", state.Card{Source: "claude", Session: "s1"}, state.Details{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reader.Read(tt.card)
			if got.Name != tt.want.Name || got.Branch != tt.want.Branch || !equalPercent(got.ContextPercent, tt.want.ContextPercent) {
				t.Errorf("Read = %+v (context %v), want %+v (context %v)", got, show(got.ContextPercent), tt.want, show(tt.want.ContextPercent))
			}
		})
	}
}

func percent(value float64) *float64 { return &value }

func equalPercent(a, b *float64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func show(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
