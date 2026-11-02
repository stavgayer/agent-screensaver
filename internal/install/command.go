package install

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	profileBegin = "# >>> agent-screensaver >>>"
	profileEnd   = "# <<< agent-screensaver <<<"
	// profileBlock is what install adds to the shell profile. The markers
	// tell uninstall which lines are ours.
	profileBlock = profileBegin + "\n" + `export PATH="$HOME/.local/bin:$PATH"` + "\n" + profileEnd + "\n"

	pathHint = "~/.local/bin"
)

// linkState says what is at the command link path.
type linkState int

const (
	linkAbsent  linkState = iota // nothing there
	linkOurs                     // a symlink to our installed binary
	linkForeign                  // anything else, never touched
)

// pathAction says what install does about the PATH.
type pathAction int

const (
	pathNothing pathAction = iota // already on the PATH, or no link to reach
	pathEdit                      // add the block to the shell profile
	pathManual                    // another shell: the user adds it
)

// commandPlan is what install does to put the command on the PATH. It is
// read once from the disk and the environment given in Options, so the plan,
// the notes and Apply agree.
type commandPlan struct {
	link   linkState
	linkOK bool // the link folder exists
	action pathAction
	// profile is the file to edit when action is pathEdit.
	profile       string
	profileExists bool
	// shell is the name of the user's shell, for the line of pathManual.
	shell string
}

// planCommand reads the disk. An unreadable profile is an error, so install
// stops before it changes anything.
func planCommand(where locations, opts Options) (commandPlan, error) {
	plan := commandPlan{link: linkStateOf(where)}
	if info, err := os.Stat(filepath.Dir(where.link)); err == nil && info.IsDir() {
		plan.linkOK = true
	}
	// A command that is not ours at the link path is not reachable through
	// the PATH either, so the profile stays alone.
	if plan.link == linkForeign || onPath(where.home, opts.Path) {
		return plan, nil
	}
	if opts.Shell != "" {
		plan.shell = filepath.Base(opts.Shell)
	}
	profile, ok := profileFor(where.home, plan.shell)
	if !ok {
		plan.action = pathManual
		return plan, nil
	}
	data, err := os.ReadFile(profile)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return commandPlan{}, fmt.Errorf("read %s: %w", profile, err)
	default:
		plan.profileExists = true
		if _, found := withoutBlocks(data); found {
			return plan, nil // our block is there already
		}
	}
	plan.action, plan.profile = pathEdit, profile
	return plan, nil
}

// profileFor is the file that the PATH line goes to, by the name of the shell.
func profileFor(home, shell string) (string, bool) {
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zprofile"), true
	case "bash":
		return filepath.Join(home, ".bash_profile"), true
	}
	return "", false
}

// ownedProfiles are the files install may edit. Uninstall takes a profile
// from the record only when it is one of these, so an edited record cannot
// point it at another file.
func ownedProfiles(home string) []string {
	return []string{filepath.Join(home, ".zprofile"), filepath.Join(home, ".bash_profile")}
}

// onPath reports whether ~/.local/bin is in path. Entries are compared as
// cleaned absolute paths, and a leading ~ or $HOME is expanded first.
func onPath(home, path string) bool {
	want := filepath.Join(home, ".local", "bin")
	for _, entry := range filepath.SplitList(path) {
		entry = expandHomeEntry(home, entry)
		if filepath.IsAbs(entry) && filepath.Clean(entry) == want {
			return true
		}
	}
	return false
}

func expandHomeEntry(home, entry string) string {
	for _, prefix := range []string{"~", "${HOME}", "$HOME"} {
		if entry == prefix || strings.HasPrefix(entry, prefix+"/") {
			return home + strings.TrimPrefix(entry, prefix)
		}
	}
	return entry
}

// linkStateOf reads the command link. Ours is a symlink to our binary; a
// file, a folder, or a link to anything else is not ours.
func linkStateOf(where locations) linkState {
	info, err := os.Lstat(where.link)
	if errors.Is(err, os.ErrNotExist) {
		return linkAbsent
	}
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return linkForeign
	}
	target, err := os.Readlink(where.link)
	if err != nil || filepath.Clean(target) != where.bin {
		return linkForeign
	}
	return linkOurs
}

// installCommand makes the link and edits the profile as plan says. Both
// steps change nothing when they are done already.
func installCommand(where locations, plan commandPlan) error {
	if plan.link != linkForeign && plan.link != linkOurs {
		if err := os.MkdirAll(filepath.Dir(where.link), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(where.link), err)
		}
		if err := os.Symlink(where.bin, where.link); err != nil {
			return fmt.Errorf("link %s: %w", where.link, err)
		}
	}
	if plan.action != pathEdit {
		return nil
	}
	return addProfileBlock(plan.profile)
}

// addProfileBlock appends the block. An existing profile is backed up first,
// the same way as an agent settings file.
func addProfileBlock(profile string) error {
	original, err := os.ReadFile(profile)
	if errors.Is(err, os.ErrNotExist) {
		return writeFileAtomic(profile, []byte(profileBlock), 0o644)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", profile, err)
	}
	updated := slices.Concat(original, []byte(profileBlock))
	if len(original) > 0 && !bytes.HasSuffix(original, []byte("\n")) {
		updated = slices.Concat(original, []byte("\n"), []byte(profileBlock))
	}
	return writeEdit(configEdit{path: profile, original: original, updated: updated, backup: true})
}

// withoutBlocks returns data without its marked blocks. A block runs from a
// begin line to the next end line; a begin line with no end line is not a
// block and stays, as does everything outside the markers.
func withoutBlocks(data []byte) ([]byte, bool) {
	lines := bytes.SplitAfter(data, []byte("\n"))
	var kept [][]byte
	var pending [][]byte // from the last begin line, until its end line
	found := false
	for _, line := range lines {
		switch marker := strings.TrimSpace(string(line)); {
		case marker == profileBegin:
			kept = append(kept, pending...) // an earlier begin with no end
			pending = [][]byte{line}
		case marker == profileEnd && pending != nil:
			pending, found = nil, true
		case pending != nil:
			pending = append(pending, line)
		default:
			kept = append(kept, line)
		}
	}
	kept = append(kept, pending...)
	return bytes.Join(kept, nil), found
}

// planProfileRemovals reads the recorded profiles and prepares the edit for
// each one that still holds our block.
func planProfileRemovals(where locations, record Record) ([]configEdit, error) {
	var edits []configEdit
	for _, profile := range record.Profiles {
		if !slices.Contains(ownedProfiles(where.home), profile) {
			continue
		}
		data, err := os.ReadFile(profile)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", profile, err)
		}
		if updated, found := withoutBlocks(data); found {
			edits = append(edits, configEdit{path: profile, original: data, updated: updated})
		}
	}
	return edits, nil
}

// removeCommandLink deletes the command link when it is still ours.
func removeCommandLink(where locations) error {
	if linkStateOf(where) != linkOurs {
		return nil
	}
	if err := os.Remove(where.link); err != nil {
		return fmt.Errorf("remove %s: %w", where.link, err)
	}
	return nil
}

// planCommandSteps are the install plan lines for the command.
func planCommandSteps(where locations, plan commandPlan) []PlannedStep {
	if plan.link == linkForeign {
		return []PlannedStep{{Name: "command", Warning: true, Description: "Skip the agent-screensaver command: " + where.link +
			" already exists and is not ours, so it stays as it is"}}
	}
	description := "Link the agent-screensaver command at " + where.link
	if !plan.linkOK {
		description += "; the folder " + filepath.Dir(where.link) + " does not exist yet and is made first"
	}
	steps := []PlannedStep{{Name: "command", Description: description}}
	switch plan.action {
	case pathEdit:
		detail := "the file does not exist yet and is created"
		if plan.profileExists {
			detail = "backup: " + plan.profile + backupSuffix
		}
		steps = append(steps, PlannedStep{Name: "path", Description: fmt.Sprintf("Add %s to your PATH in %s (%s)", pathHint, plan.profile, detail)})
	case pathManual:
		reason := "install does not know your shell"
		if plan.shell != "" {
			reason = "your shell (" + plan.shell + ") is not zsh or bash"
		}
		steps = append(steps, PlannedStep{Name: "path", Warning: true, Description: "Leave your shell profile alone: " + reason +
			". Add " + pathHint + " to your PATH yourself to use the agent-screensaver command"})
	}
	return steps
}

// CommandNotes are the lines for "What is left for you" about the command.
// Read them before Apply: they depend on what is on the disk now.
func CommandNotes(home string, opts Options) []string {
	plan, err := planCommand(locationsFor(home), opts)
	if err != nil {
		return nil
	}
	switch plan.action {
	case pathEdit:
		return []string{"Open a new Terminal window to use the agent-screensaver command."}
	case pathManual:
		return []string{"Add " + pathHint + " to your PATH to use the agent-screensaver command."}
	}
	return nil
}

// planRemoveCommand are the uninstall plan lines for the command, empty when
// install never made the link and added no lines.
func planRemoveCommand(where locations, record Record) []PlannedStep {
	var steps []PlannedStep
	if record.CommandLink != "" {
		switch linkStateOf(where) {
		case linkOurs:
			steps = append(steps, PlannedStep{Name: "command", Description: "Delete the agent-screensaver command link " + where.link})
		case linkForeign:
			steps = append(steps, PlannedStep{Name: "command", Warning: true, Description: "Leave " + where.link +
				" where it is: it is not the link that install made"})
		}
	}
	edits, _ := planProfileRemovals(where, record)
	for _, edit := range edits {
		steps = append(steps, PlannedStep{Name: "path", Description: "Remove the lines of agent-screensaver that add " + pathHint +
			" to your PATH from " + edit.path + " (your other lines stay)"})
	}
	return steps
}

// commandRecord is what the record says about the command. The link counts
// as ours when it is there or install makes it now. A profile stays in the
// list from earlier installs, so a run that adds nothing keeps it.
func commandRecord(where locations, plan commandPlan, prior Record) (string, []string) {
	link := ""
	if plan.link != linkForeign {
		link = where.link
	} else if prior.CommandLink != "" {
		link = prior.CommandLink
	}
	profiles := slices.Clone(prior.Profiles)
	if plan.action == pathEdit && !slices.Contains(profiles, plan.profile) {
		profiles = append(profiles, plan.profile)
	}
	return link, profiles
}
