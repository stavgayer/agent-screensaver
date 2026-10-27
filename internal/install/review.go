package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/textdiff"
)

const (
	// MaxDiffBytes is the largest file shown as a text diff. A larger
	// file, or one that is not UTF-8 text, is shown by its size and hash.
	MaxDiffBytes = 256 << 10
	// MaxDiffLines caps the diff of one file.
	MaxDiffLines = 200
)

// ChangeKind says what an upgrade does to one runnable file.
type ChangeKind string

const (
	FileChanged ChangeKind = "changed"
	FileAdded   ChangeKind = "added"
	FileRemoved ChangeKind = "removed"
)

// FileChange is one runnable file that an upgrade changes, adds or removes.
type FileChange struct {
	// Path is relative to the extension folder, with "/" separators.
	Path string     `json:"path"`
	Kind ChangeKind `json:"kind"`
	// Diff is a unified diff for a text file; Truncated says it was cut
	// at MaxDiffLines lines.
	Diff      string `json:"diff,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	// Binary is true for a file that is not shown as text. The sizes and
	// hashes are set for every change; a missing side has size 0 and no
	// hash.
	Binary    bool   `json:"binary,omitempty"`
	OldSize   int64  `json:"old_size"`
	NewSize   int64  `json:"new_size"`
	OldSHA256 string `json:"old_sha256,omitempty"`
	NewSHA256 string `json:"new_sha256,omitempty"`
}

// RunnableFiles are the agent's files that run or are copied into the
// tool's own folder: its converter programs (hooks, status line, details)
// and the files its recipe copies. The paths are relative to the agent's
// folder, with "/" separators, sorted.
func RunnableFiles(a agents.Agent) []string {
	var files []string
	add := func(relative string) {
		relative = filepath.ToSlash(filepath.Clean(relative))
		if !slices.Contains(files, relative) {
			files = append(files, relative)
		}
	}
	for _, program := range Programs(a) {
		if relative, err := filepath.Rel(a.Dir, program); err == nil {
			add(relative)
		}
	}
	for _, file := range a.Install.Files {
		add(file.From)
	}
	slices.Sort(files)
	return files
}

// ReviewUpgrade compares the runnable files of the installed agent with
// those of the staged new version. installed is nil when the installed
// folder is not a valid agent; then every runnable file of the new version
// is added. No change means the upgrade runs nothing new.
func ReviewUpgrade(installed *agents.Agent, staged agents.Agent) ([]FileChange, error) {
	var oldFiles []string
	oldDir := ""
	if installed != nil {
		oldFiles, oldDir = RunnableFiles(*installed), installed.Dir
	}
	newFiles := RunnableFiles(staged)
	paths := slices.Clone(oldFiles)
	for _, path := range newFiles {
		if !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	var changes []FileChange
	for _, path := range paths {
		var old, new []byte
		var err error
		if slices.Contains(oldFiles, path) {
			if old, err = os.ReadFile(filepath.Join(oldDir, filepath.FromSlash(path))); err != nil {
				return nil, fmt.Errorf("read installed %s: %w", path, err)
			}
		}
		if slices.Contains(newFiles, path) {
			if new, err = os.ReadFile(filepath.Join(staged.Dir, filepath.FromSlash(path))); err != nil {
				return nil, fmt.Errorf("read new %s: %w", path, err)
			}
		}
		change, changed := compareFile(path, old, new, slices.Contains(oldFiles, path), slices.Contains(newFiles, path))
		if changed {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

func compareFile(path string, old, new []byte, inOld, inNew bool) (FileChange, bool) {
	change := FileChange{Path: path, Kind: FileChanged, OldSize: int64(len(old)), NewSize: int64(len(new))}
	oldName, newName := "a/"+path, "b/"+path
	switch {
	case !inOld:
		change.Kind, oldName = FileAdded, "/dev/null"
	case !inNew:
		change.Kind, newName = FileRemoved, "/dev/null"
	case bytes.Equal(old, new):
		return FileChange{}, false
	}
	if inOld {
		change.OldSHA256 = sha256Hex(old)
	}
	if inNew {
		change.NewSHA256 = sha256Hex(new)
	}
	if !isText(old) || !isText(new) {
		change.Binary = true
		return change, true
	}
	change.Diff, change.Truncated = textdiff.Unified(oldName, newName, old, new, MaxDiffLines)
	return change, true
}

// isText: valid UTF-8, no NUL byte and smaller than MaxDiffBytes.
func isText(data []byte) bool {
	return len(data) < MaxDiffBytes && utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}
