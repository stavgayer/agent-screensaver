package install

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/textdiff"
)

// MaxDiffBytes is the largest diff kept for one file. A file whose diff
// is longer is marked TooLarge and is shown by its paths, to be read by
// hand.
const MaxDiffBytes = 2 << 20

// MaxDiffInput is the largest old plus new size that is diffed at all. A
// larger pair is TooLarge at once: the diff would not fit MaxDiffBytes
// anyway, and building it would cost time and memory twice (review and
// confirm).
const MaxDiffInput = 4 << 20

// ChangeKind says what an upgrade does to one file.
type ChangeKind string

const (
	FileChanged ChangeKind = "changed"
	FileAdded   ChangeKind = "added"
	FileRemoved ChangeKind = "removed"
)

// ChangeGroup says what part a file plays, in the order a review shows them.
type ChangeGroup string

const (
	// GroupManifest is extension.json: it says what runs and what is read.
	GroupManifest ChangeGroup = "manifest"
	// GroupRunnable are the converter programs and the files the recipe
	// copies into the tool's folder.
	GroupRunnable ChangeGroup = "runnable"
	// GroupOther is every other file. A program can still read or run it,
	// so it is reviewed too.
	GroupOther ChangeGroup = "other"
)

var groupOrder = map[ChangeGroup]int{GroupManifest: 0, GroupRunnable: 1, GroupOther: 2}

// FileChange is one file of the extension that an upgrade changes, adds or
// removes.
type FileChange struct {
	// Path is relative to the extension folder, with "/" separators.
	Path  string      `json:"path"`
	Kind  ChangeKind  `json:"kind"`
	Group ChangeGroup `json:"group"`
	// Diff is the whole unified diff of a text file (valid UTF-8, no NUL
	// byte). The manifest is always diffed as text.
	Diff string `json:"diff,omitempty"`
	// Binary is a file that is not text: it is shown by its sizes and
	// hashes. TooLarge is a text file whose diff is over MaxDiffBytes:
	// OldPath and NewPath name the two copies to read by hand.
	Binary   bool   `json:"binary,omitempty"`
	TooLarge bool   `json:"too_large,omitempty"`
	OldPath  string `json:"old_path,omitempty"`
	NewPath  string `json:"new_path,omitempty"`
	// NewArchive and NewEntry are set when the new copy is inside a .zip:
	// the archive and the entry's name in it, to extract it for reading.
	NewArchive string `json:"new_archive,omitempty"`
	NewEntry   string `json:"new_entry,omitempty"`
	// The sizes and hashes are set for every change; a missing side has
	// size 0 and no hash.
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

// ReviewUpgrade compares every file of the installed folder with the
// staged new version: a converter can source or import any file next to
// it, and the manifest decides what runs and what is read. installedDir
// is "" when nothing is installed; installed is nil when the installed
// folder is not a valid agent, and then only the new version's files count
// as runnable. The changes come in group order (manifest, runnable, other),
// then by path. No change means the upgrade changes no byte.
func ReviewUpgrade(installedDir string, installed *agents.Agent, staged agents.Agent) ([]FileChange, error) {
	oldFiles, err := folderFiles(installedDir)
	if err != nil {
		return nil, fmt.Errorf("read installed copy: %w", err)
	}
	newFiles, err := folderFiles(staged.Dir)
	if err != nil {
		return nil, fmt.Errorf("read new version: %w", err)
	}
	runnable := RunnableFiles(staged)
	if installed != nil {
		runnable = append(runnable, RunnableFiles(*installed)...)
	}
	var changes []FileChange
	for _, path := range unionOf(oldFiles, newFiles) {
		old, inOld := oldFiles[path]
		new, inNew := newFiles[path]
		change, changed := compareFile(path, old, new, inOld, inNew)
		if !changed {
			continue
		}
		change.Group = groupOf(path, runnable)
		if change.TooLarge {
			change.OldPath, change.NewPath = sidePath(installedDir, path, inOld), sidePath(staged.Dir, path, inNew)
		}
		changes = append(changes, change)
	}
	slices.SortStableFunc(changes, func(a, b FileChange) int { return groupOrder[a.Group] - groupOrder[b.Group] })
	return changes, nil
}

func groupOf(path string, runnable []string) ChangeGroup {
	switch {
	case path == extensions.ManifestFile:
		return GroupManifest
	case slices.Contains(runnable, path):
		return GroupRunnable
	default:
		return GroupOther
	}
}

func sidePath(dir, path string, present bool) string {
	if !present {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(path))
}

// folderFiles reads every regular file under dir by its "/" path. Install
// refuses symlinks and special files, so there are none to read. A dir of
// "" has no files.
func folderFiles(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	if dir == "" {
		return files, nil
	}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || !entry.Type().IsRegular() {
			return walkErr
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = data
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	return files, err
}

func unionOf(a, b map[string][]byte) []string {
	var paths []string
	for path := range a {
		paths = append(paths, path)
	}
	for path := range b {
		if _, seen := a[path]; !seen {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths
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
	if path != extensions.ManifestFile && (!isText(old) || !isText(new)) {
		change.Binary = true
		return change, true
	}
	if len(old)+len(new) > MaxDiffInput {
		change.TooLarge = true
		return change, true
	}
	diff, _ := textdiff.Unified(oldName, newName, old, new, 0)
	if len(diff) > MaxDiffBytes {
		change.TooLarge = true
		return change, true
	}
	change.Diff = diff
	return change, true
}

// isText: valid UTF-8 and no NUL byte.
func isText(data []byte) bool {
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}
