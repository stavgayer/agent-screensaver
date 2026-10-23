package sessioninfo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"agent-screensaver/internal/atomicfile"
)

// ContextSizes remembers the context window size of each model, learned
// from the Claude Code status line (context_window.context_window_size).
// The desktop app's transcripts do not say the size, so its sessions use
// what was learned for the same model. It is kept in a small JSON file and
// is not safe for use by more than one goroutine.
type ContextSizes struct {
	path  string
	sizes map[string]int // by model id without its "[1m]" style marker
}

// LoadContextSizes reads the sizes learned before. A missing file means
// none; a broken one is reported and an empty store is returned.
func LoadContextSizes(path string) (*ContextSizes, error) {
	sizes := &ContextSizes{path: path, sizes: map[string]int{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return sizes, nil
	}
	if err != nil {
		return sizes, fmt.Errorf("read context sizes: %w", err)
	}
	if err := json.Unmarshal(data, &sizes.sizes); err != nil {
		sizes.sizes = map[string]int{}
		return sizes, fmt.Errorf("decode context sizes: %w", err)
	}
	return sizes, nil
}

// Learn records the window size of a model and saves it when it grew. The
// largest size seen is kept: a session that runs the same model with the
// 200k window must not undo a 1M one, nor two sessions flip it.
func (c *ContextSizes) Learn(modelID string, tokens int) error {
	key := baseModel(modelID)
	if key == "" || tokens <= c.sizes[key] {
		return nil
	}
	c.sizes[key] = tokens
	data, err := json.Marshal(c.sizes)
	if err != nil {
		return fmt.Errorf("encode context sizes: %w", err)
	}
	if err := atomicfile.Write(c.path, data, 0o600); err != nil {
		return fmt.Errorf("save context sizes: %w", err)
	}
	return nil
}

func (c *ContextSizes) size(model string) (int, bool) {
	if c == nil {
		return 0, false
	}
	size, ok := c.sizes[baseModel(model)]
	return size, ok && size > 0
}

// baseModel drops a "[1m]" style marker, so the terminal's
// "claude-opus-5-5[1m]" and the desktop transcript's "claude-opus-5-5"
// share what was learned.
func baseModel(model string) string {
	base, _, _ := strings.Cut(model, "[")
	return base
}
