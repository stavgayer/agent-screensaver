package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"agent-screensaver/internal/install"
	"agent-screensaver/internal/settings"
)

const extUsage = "ext list | ext install [--yes] <path> | ext remove <id>"

// errCancelled: the user said no to a question, which already printed that
// nothing changed.
var errCancelled = errors.New("cancelled")

// shortHash is how many hex digits of a SHA-256 the review prints.
const shortHash = 12

// installExtension installs a folder or a .zip file. An upgrade of an
// outside agent that changes its runnable files prints the changes and
// asks first; --yes skips the question, not the list.
func installExtension(service settings.Service, rest []string, stdin io.Reader, stdout io.Writer) error {
	yes := slices.Contains(rest, "--yes")
	sources := slices.DeleteFunc(slices.Clone(rest), func(arg string) bool { return arg == "--yes" })
	if len(sources) != 1 {
		return usageError(extUsage)
	}
	review, err := service.ReviewInstall(sources[0])
	if err != nil {
		return err
	}
	switch {
	case len(review.Changes) > 0:
		fmt.Fprint(stdout, FormatUpgradeReview(review))
		if !ask("Install this upgrade?", yes, stdin, stdout) {
			return errCancelled
		}
	case review.Upgrade:
		fmt.Fprintf(stdout, "%s: no runnable files changed.\n", upgradeTitle(review))
	}
	manifest, err := service.InstallReviewed(sources[0], review.Hash)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "installed: %s %s\n", manifest.ID, manifest.Version)
	return nil
}

func upgradeTitle(review settings.InstallReview) string {
	return fmt.Sprintf("Upgrade %s from %s to %s", review.Manifest.ID, versionText(review.FromVersion), versionText(review.Manifest.Version))
}

func versionText(version string) string {
	if version == "" {
		return "an unknown version"
	}
	return version
}

// FormatUpgradeReview is the text the CLI prints for an upgrade with
// changes: one block per file, a text diff indented under it, or the sizes
// and hashes of a binary file.
func FormatUpgradeReview(review settings.InstallReview) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s. These files run on your Mac or are copied into the agent's folder, and they change:\n", upgradeTitle(review))
	for _, change := range review.Changes {
		if change.Binary {
			fmt.Fprintf(&out, "  %s: %s (binary)\n", change.Kind, change.Path)
			if change.Kind != install.FileAdded {
				fmt.Fprintf(&out, "    old: %d bytes, sha256 %s\n", change.OldSize, short(change.OldSHA256))
			}
			if change.Kind != install.FileRemoved {
				fmt.Fprintf(&out, "    new: %d bytes, sha256 %s\n", change.NewSize, short(change.NewSHA256))
			}
			continue
		}
		fmt.Fprintf(&out, "  %s: %s\n", change.Kind, change.Path)
		for _, line := range strings.SplitAfter(change.Diff, "\n") {
			if line != "" {
				fmt.Fprintf(&out, "    %s", line)
			}
		}
		if change.Truncated {
			fmt.Fprintf(&out, "    (diff truncated after %d lines)\n", install.MaxDiffLines)
		}
	}
	fmt.Fprintln(&out, "The installed version stays until you answer y.")
	return out.String()
}

func short(hash string) string {
	if len(hash) > shortHash {
		return hash[:shortHash]
	}
	return hash
}
