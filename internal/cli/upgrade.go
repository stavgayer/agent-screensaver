package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"agent-screensaver/internal/install"
	"agent-screensaver/internal/settings"
)

const extUsage = "ext list | ext install [--yes] [--full] <path> | ext remove <id>"

// errCancelled: the install stopped after it printed why and that nothing
// changed.
var errCancelled = errors.New("cancelled")

const (
	// shortHash is how many hex digits of a SHA-256 the review prints.
	shortHash = 12
	// previewLines and maxLineRunes cut the review unless --full is given.
	previewLines = 200
	maxLineRunes = 500
)

// installExtension installs a folder or a .zip file. An upgrade of an
// outside agent that changes any of its files prints the changes and asks
// first. --yes skips the question, not the list. A review that had to be
// cut is refused until --full prints all of it, and a file too large to
// show needs --yes after the user read it by hand.
func installExtension(service settings.Service, rest []string, stdin io.Reader, stdout io.Writer) error {
	yes, full := slices.Contains(rest, "--yes"), slices.Contains(rest, "--full")
	sources := slices.DeleteFunc(slices.Clone(rest), func(arg string) bool { return arg == "--yes" || arg == "--full" })
	if len(sources) != 1 {
		return usageError(extUsage)
	}
	review, err := service.ReviewInstall(sources[0])
	if err != nil {
		return err
	}
	switch {
	case len(review.Changes) > 0:
		text, cut, tooLarge := FormatUpgradeReview(review, full)
		fmt.Fprint(stdout, text)
		if refused := refusal(cut, tooLarge, yes); refused != "" {
			fmt.Fprintln(stdout, refused)
			fmt.Fprintln(stdout, "Cancelled. Nothing was changed.")
			return errCancelled
		}
		if !ask("Install this upgrade?", yes, stdin, stdout) {
			return errCancelled
		}
	case review.Upgrade:
		fmt.Fprintf(stdout, "%s: no files changed.\n", upgradeTitle(review))
	}
	manifest, err := service.InstallReviewed(sources[0], review.Hash)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "installed: %s %s\n", manifest.ID, manifest.Version)
	return nil
}

// refusal says why the install cannot be confirmed yet, or "".
func refusal(cut, tooLarge, yes bool) string {
	switch {
	case cut:
		return "Part of the review was cut. Read all of it before you install: run again with --full."
	case tooLarge && !yes:
		return "A file is too large to show. Review the files by hand, then run again with --yes to install."
	}
	return ""
}

func upgradeTitle(review settings.InstallReview) string {
	return fmt.Sprintf("Upgrade %s from %s to %s", escapeControls(review.Manifest.ID),
		escapeControls(versionText(review.FromVersion)), escapeControls(versionText(review.Manifest.Version)))
}

func versionText(version string) string {
	if version == "" {
		return "an unknown version"
	}
	return version
}

var groupHeadings = map[install.ChangeGroup]string{
	install.GroupManifest: "Manifest:",
	install.GroupRunnable: "Programs and copied files (these run on your Mac or go into the agent's folder):",
	install.GroupOther:    "Other files (a program of the agent can read or run these too):",
}

// FormatUpgradeReview is the text the CLI prints for an upgrade with
// changes: the files under their group, each with its diff indented under
// it, or the sizes and hashes of a binary file. Unless full is set, a diff
// stops after previewLines lines and a line after maxLineRunes characters;
// cut says that happened. tooLarge says a file is shown only by its paths.
// Control characters are shown as escapes, never sent to the terminal.
func FormatUpgradeReview(review settings.InstallReview, full bool) (text string, cut, tooLarge bool) {
	var out strings.Builder
	fmt.Fprintf(&out, "%s. These files of the extension change:\n", upgradeTitle(review))
	var group install.ChangeGroup
	for _, change := range review.Changes {
		if change.Group != group {
			group = change.Group
			fmt.Fprintln(&out, groupHeadings[group])
		}
		path := escapeControls(change.Path)
		switch {
		case change.Binary:
			fmt.Fprintf(&out, "  %s: %s (binary)\n", change.Kind, path)
			if change.Kind != install.FileAdded {
				fmt.Fprintf(&out, "    old: %d bytes, sha256 %s\n", change.OldSize, short(change.OldSHA256))
			}
			if change.Kind != install.FileRemoved {
				fmt.Fprintf(&out, "    new: %d bytes, sha256 %s\n", change.NewSize, short(change.NewSHA256))
			}
		case change.TooLarge:
			tooLarge = true
			fmt.Fprintf(&out, "  %s: %s\n    too large to show: review the files by hand\n", change.Kind, path)
			if change.OldPath != "" {
				fmt.Fprintf(&out, "    old: %s\n", escapeControls(change.OldPath))
			}
			if change.NewPath != "" {
				fmt.Fprintf(&out, "    new: %s\n", escapeControls(change.NewPath))
			}
			if change.NewArchive != "" {
				fmt.Fprintf(&out, "    extract it with: %s\n", escapeControls(UnzipCommand(change.NewArchive, change.NewEntry)))
			}
		default:
			fmt.Fprintf(&out, "  %s: %s\n", change.Kind, path)
			if writeDiff(&out, change.Diff, full) {
				cut = true
			}
		}
	}
	fmt.Fprintln(&out, "The installed version stays until you confirm.")
	return out.String(), cut, tooLarge
}

// writeDiff writes the diff indented, cut unless full, and says whether it
// cut anything.
func writeDiff(out *strings.Builder, diff string, full bool) bool {
	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	cut := false
	for index, line := range lines {
		if !full && index == previewLines {
			fmt.Fprintf(out, "    (diff cut after %d lines, %d more)\n", previewLines, len(lines)-previewLines)
			return true
		}
		line = escapeControls(line)
		if count := utf8.RuneCountInString(line); !full && count > maxLineRunes {
			line = string([]rune(line)[:maxLineRunes]) + fmt.Sprintf(" (line cut, %d more characters)", count-maxLineRunes)
			cut = true
		}
		fmt.Fprintf(out, "    %s\n", line)
	}
	return cut
}

// escapeControls shows every character that could move the cursor,
// recolor, hide or reorder text as a visible escape: C0 controls but tab,
// DEL and bytes that are not UTF-8 as \xNN; C1 controls and the bidi and
// invisible format characters as <U+NNNN>.
func escapeControls(text string) string {
	var out strings.Builder
	for index := 0; index < len(text); {
		r, size := utf8.DecodeRuneInString(text[index:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&out, `\x%02x`, text[index])
		case r == '\t':
			out.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&out, `\x%02x`, r)
		case isHiddenFormat(r):
			fmt.Fprintf(&out, "<U+%04X>", r)
		default:
			out.WriteRune(r)
		}
		index += size
	}
	return out.String()
}

// isHiddenFormat: C1 controls, every format character (Unicode category
// Cf: bidi marks, embeddings, overrides and isolates, zero-width
// characters, the BOM, the soft hyphen, tag characters), the line and
// paragraph separators, and the variation selectors.
func isHiddenFormat(r rune) bool {
	return (r >= 0x80 && r <= 0x9f) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 ||
		(r >= 0xfe00 && r <= 0xfe0f) || (r >= 0xe0000 && r <= 0xe007f) || (r >= 0xe0100 && r <= 0xe01ef)
}

// UnzipCommand prints one entry of a .zip to a pager, for reading by hand.
func UnzipCommand(archive, entry string) string {
	return "unzip -p " + shellQuote(archive) + " " + shellQuote(entry) + " | less"
}

func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

func short(hash string) string {
	if len(hash) > shortHash {
		return hash[:shortHash]
	}
	return hash
}
