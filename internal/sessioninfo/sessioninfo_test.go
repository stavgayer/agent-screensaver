package sessioninfo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	claudeSession = "11111111-1111-4111-8111-111111111111"
	codexSession  = "33333333-3333-4333-8333-333333333333"
)

func fixture(name string) string { return filepath.Join("testdata", name) }

func TestClaudeContext(t *testing.T) {
	learned := &ContextSizes{sizes: map[string]int{"claude-opus-5-5": 1_000_000}}
	tests := []struct {
		name       string
		transcript string
		sizes      *ContextSizes
		want       *float64
	}{
		// 3 + 2,000 + 18,000 tokens of the last turn; the status line said
		// this model's window is 1,000,000.
		{"the learned window of the model", fixture("claude_transcript.jsonl"), learned, percent(2)},
		// 150,000 tokens in a 1,000,000 window.
		{"the 1M marker", fixture("claude_context_1m.jsonl"), nil, percent(15)},
		// 450,000 tokens cannot fit 200,000, so the window is the 1M one;
		// the synthetic error line after it is not a real turn.
		{"more than the default window means 1M", fixture("claude_context_over_default.jsonl"), nil, percent(45)},
		{"nothing learned and the model has a 1M choice: no bar", fixture("claude_transcript.jsonl"), nil, nil},
		{"unknown model hides the bar", fixture("claude_context_unknown_model.jsonl"), nil, nil},
		{"missing file", fixture("missing.jsonl"), nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClaudeContext(tt.transcript, tt.sizes)
			if !equalPercent(got, tt.want) {
				t.Errorf("ClaudeContext = %v, want %v", show(got), show(tt.want))
			}
		})
	}
}

func TestContextWindow(t *testing.T) {
	learned := &ContextSizes{sizes: map[string]int{"claude-sonnet-5-5": 200_000}}
	tests := []struct {
		model  string
		used   float64
		want   float64
		wantOK bool
	}{
		{"claude-opus-5-5[1m]", 1000, 1_000_000, true},
		{"Opus 5.5 (1M context)", 1000, 1_000_000, true},
		{"claude-sonnet-5-5", 1000, 200_000, true},      // learned
		{"claude-sonnet-5-5", 250_000, 1_000_000, true}, // larger than learned: only 1M holds it
		{"claude-opus-5-5", 250_000, 1_000_000, true},
		{"claude-opus-5-5", 1000, 0, false}, // could be 200k or 1M: no bar
		{"claude-haiku-5-5", 1000, 200_000, true},
		{"<synthetic>", 1000, 0, false},
		{"", 1000, 0, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s %v", tt.model, tt.used), func(t *testing.T) {
			got, ok := contextWindow(tt.model, tt.used, learned)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("contextWindow(%q, %v) = %v, %v; want %v, %v", tt.model, tt.used, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestContextSizesLearnAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "context-windows.json")
	sizes, err := LoadContextSizes(path)
	if err != nil {
		t.Fatalf("load without a file: %v", err)
	}
	if err := sizes.Learn("claude-opus-5-5[1m]", 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := sizes.Learn("", 5); err != nil { // nothing to learn
		t.Fatal(err)
	}
	again, err := LoadContextSizes(path)
	if err != nil {
		t.Fatal(err)
	}
	if size, ok := again.size("claude-opus-5-5"); !ok || size != 1_000_000 {
		t.Errorf("after reload: %v, %v; want 1,000,000 for the model without its marker", size, ok)
	}
	if _, ok := again.size(""); ok {
		t.Error("an empty model was learned")
	}
	// A 200k session of the same model does not undo the 1M one seen.
	if err := again.Learn("claude-opus-5-5", 200_000); err != nil {
		t.Fatal(err)
	}
	if size, _ := again.size("claude-opus-5-5"); size != 1_000_000 {
		t.Errorf("after a smaller session: %v, want the largest size seen", size)
	}
	if reloaded, _ := LoadContextSizes(path); reloaded.sizes["claude-opus-5-5"] != 1_000_000 {
		t.Errorf("file holds %v, want 1,000,000", reloaded.sizes)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if broken, err := LoadContextSizes(path); err == nil || broken == nil {
		t.Errorf("broken file: %v, %v; want an error and an empty store", broken, err)
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

// A dotfiles repository in the home folder is not the repository of every
// project below it.
func TestBranchIgnoresARepoInTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".git", "HEAD"), "ref: refs/heads/dotfiles\n")
	writeFile(t, filepath.Join(home, "work", "app", "keep"), "")
	writeFile(t, filepath.Join(home, "work", "repo", ".git", "HEAD"), "ref: refs/heads/main\n")

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"a project without a repo", filepath.Join(home, "work", "app"), ""},
		{"a project with its own repo", filepath.Join(home, "work", "repo"), "main"},
		{"a session in the home folder itself", home, "dotfiles"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Branch(tt.cwd, home); got != tt.want {
				t.Errorf("Branch(%q) = %q, want %q", tt.cwd, got, tt.want)
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
