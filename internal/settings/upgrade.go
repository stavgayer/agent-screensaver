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

// ErrUpgradeNotReviewed is returned when an install would replace the
// runnable files of an installed outside agent and no review was shown.
var ErrUpgradeNotReviewed = errors.New("this upgrade changes the agent's runnable files; review the changes and confirm them first")

// InstallReview is what installing a folder or a .zip file would replace.
// Only an upgrade of an outside agent has changes: its converter programs
// and the files its recipe copies are its code that runs on this Mac.
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
		review, err = s.reviewStaged(m, dir)
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
		review, err := s.reviewStaged(m, dir)
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

// reviewStaged compares the staged agent in dir with the installed one.
// An installed folder that is not a valid agent counts as having no
// runnable files, so every file of the new version is shown as added.
func (s Service) reviewStaged(m extensions.Manifest, dir string) (InstallReview, error) {
	review := InstallReview{Manifest: m, Changes: []install.FileChange{}}
	if m.Type != extensions.TypeAgent {
		return review, nil
	}
	if _, err := os.Stat(filepath.Join(s.Paths.ExtensionsDir, m.ID)); errors.Is(err, fs.ErrNotExist) {
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
	changes, err := install.ReviewUpgrade(installed, staged)
	if err != nil {
		return InstallReview{}, err
	}
	if len(changes) > 0 {
		review.Changes = changes
		if review.Hash, err = reviewHash(review); err != nil {
			return InstallReview{}, err
		}
	}
	return review, nil
}

// reviewHash covers the id, both versions and every change with the full
// hashes of both sides, so any other bytes give another hash.
func reviewHash(review InstallReview) (string, error) {
	data, err := json.Marshal(struct {
		ID, From, To string
		Changes      []install.FileChange
	}{review.Manifest.ID, review.FromVersion, review.Manifest.Version, review.Changes})
	if err != nil {
		return "", fmt.Errorf("encode review: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
