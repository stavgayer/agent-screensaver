package settings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/install"
)

// ErrUpgradeNotReviewed is returned when an install would change any file
// of an installed outside agent and no review was shown.
var ErrUpgradeNotReviewed = errors.New("this upgrade changes the agent's files; review the changes and confirm them first")

// InstallReview is what installing a folder or a .zip file would replace.
// Only an upgrade of an outside agent has changes: every file of its
// folder, because its programs can read or run any of them, and its
// manifest says what runs.
type InstallReview struct {
	Manifest extensions.Manifest `json:"manifest"`
	// Upgrade is true when an extension with this id is installed (at
	// FromVersion) and the new one is an agent.
	Upgrade     bool                 `json:"upgrade"`
	FromVersion string               `json:"from_version"`
	Changes     []install.FileChange `json:"changes"`
	// Hash names these changes. InstallReviewed needs it when there are
	// changes; it is "" when there are none.
	Hash string `json:"hash"`
}

// ReviewInstall checks path like an install, compares the runnable files
// with the installed copy and changes nothing.
func (s Service) ReviewInstall(path string) (InstallReview, error) {
	var review InstallReview
	_, err := extensions.Inspect(path, s.Paths.ExtensionsDir, func(m extensions.Manifest, dir string) error {
		if err := s.checkAgent(m, dir); err != nil {
			return err
		}
		var err error
		review, err = s.reviewStaged(path, m, dir)
		return err
	})
	return review, err
}

// InstallReviewed installs path when its review hash is reviewed. An
// upgrade without changes needs no hash. The review is done again on the
// staged copy, so files that changed after the review are refused.
func (s Service) InstallReviewed(path, reviewed string) (extensions.Manifest, error) {
	return extensions.InstallChecked(path, s.Paths.ExtensionsDir, func(m extensions.Manifest, dir string) error {
		if err := s.checkAgent(m, dir); err != nil {
			return err
		}
		review, err := s.reviewStaged(path, m, dir)
		switch {
		case err != nil:
			return err
		case review.Hash == reviewed:
			return nil
		case reviewed == "":
			return ErrUpgradeNotReviewed
		default:
			return ErrPlanChanged
		}
	})
}

// reviewStaged compares every file of the staged agent in dir with the
// installed copy. src is what the user installs from, for the paths of a
// file too large to show. An installed agent is never replaced by another
// type: its hooks would stay with no agent to read them.
func (s Service) reviewStaged(src string, m extensions.Manifest, dir string) (InstallReview, error) {
	review := InstallReview{Manifest: m, Changes: []install.FileChange{}}
	if ext, err := s.find(m.ID); err == nil && ext.Manifest.Type == extensions.TypeAgent && m.Type != extensions.TypeAgent {
		return InstallReview{}, fmt.Errorf("%s is installed as an agent and this is a %s; remove the agent first with: agent-screensaver ext remove %s", m.ID, m.Type, m.ID)
	}
	if m.Type != extensions.TypeAgent {
		return review, nil
	}
	installedDir := filepath.Join(s.Paths.ExtensionsDir, m.ID)
	if _, err := os.Stat(installedDir); errors.Is(err, fs.ErrNotExist) {
		return review, nil
	} else if err != nil {
		return InstallReview{}, err
	}
	review.Upgrade = true
	staged, err := agents.FromManifest(m, dir)
	if err != nil {
		return InstallReview{}, err
	}
	var installed *agents.Agent
	if ext, err := s.find(m.ID); err == nil {
		review.FromVersion = ext.Manifest.Version
		if agent, err := agents.FromManifest(ext.Manifest, ext.Dir); err == nil {
			installed = &agent
		}
	}
	changes, err := install.ReviewUpgrade(installedDir, installed, staged)
	if err != nil {
		return InstallReview{}, err
	}
	for index, change := range changes {
		if change.NewPath != "" {
			changes[index].NewPath = sourcePath(src, change.Path)
		}
	}
	if len(changes) > 0 {
		review.Changes = changes
		if review.Hash, err = reviewHash(review); err != nil {
			return InstallReview{}, err
		}
	}
	return review, nil
}

// sourcePath names a file of what the user installs from: a path in the
// folder, or the file's name inside the .zip.
func sourcePath(src, relative string) string {
	if info, err := os.Stat(src); err == nil && info.IsDir() {
		return filepath.Join(src, filepath.FromSlash(relative))
	}
	return relative + " inside " + src
}

// reviewHash covers the id, both versions and every changed file by path,
// kind, group and the full hashes of both sides (the manifest included),
// so any other bytes on either side give another hash. Display paths are
// left out: the staged copy lives in a new temp folder each time.
func reviewHash(review InstallReview) (string, error) {
	type fileState struct {
		Path, Kind, Group, Old, New string
	}
	files := make([]fileState, 0, len(review.Changes))
	for _, change := range review.Changes {
		files = append(files, fileState{change.Path, string(change.Kind), string(change.Group), change.OldSHA256, change.NewSHA256})
	}
	data, err := json.Marshal(struct {
		ID, From, To string
		Files        []fileState
	}{review.Manifest.ID, review.FromVersion, review.Manifest.Version, files})
	if err != nil {
		return "", fmt.Errorf("encode review: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
