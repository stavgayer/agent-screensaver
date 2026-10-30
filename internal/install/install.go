// Package install adds agent-screensaver to a user's machine and removes it
// again. It edits the user's real tool configs, so it changes only entries it
// can recognise as its own: the exact command string it wrote, which the
// install record lets uninstall rebuild.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
)

const (
	// DaemonLabel names the launchd service that runs the daemon.
	DaemonLabel = "dev.agentscreensaver.daemon"
	// TrayLabel names the launchd service that runs the menu bar app.
	TrayLabel = "dev.agentscreensaver.tray"

	backupSuffix  = ".agent-screensaver.bak"
	binaryName    = "agent-screensaver"
	saverName     = "AgentScreensaver.saver"
	builtinPrefix = "builtin."
)

// PlannedStep is one thing install or uninstall will do, shown to the user
// before anything changes. Warning marks a line the user must read: an
// outside agent that runs its own program or copies a plugin file.
type PlannedStep struct {
	Name, Description string
	Warning           bool
}

// locations are the files install owns, derived from the home directory.
type locations struct {
	paths paths.Paths
	bin   string
	saver string
	// launcher is the app that opens the settings window from Spotlight.
	launcher string
}

func locationsFor(home string) locations {
	p := paths.ForHome(home)
	return locations{
		paths: p,
		bin:   filepath.Join(p.DataDir, "bin", binaryName),
		saver: filepath.Join(home, "Library", "Screen Savers", saverName),
		// The user's own Applications folder: no admin rights needed.
		launcher: filepath.Join(home, "Applications", launcherName),
	}
}

// shellQuote quotes for sh. The installed path contains a space, and the
// tools run the command through a shell.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// installAgents are the agents Apply adds hooks for: the built-in agents
// of the build whose tool is on this Mac (their detect path exists).
func installAgents(home, artifacts string) ([]agents.Agent, error) {
	// Problems of other extensions do not stop an install; a broken agent
	// manifest does.
	installed, _ := extensions.ScanWithProblems(filepath.Join(artifacts, "extensions"))
	loaded, err := agents.FromInstalled(installed, nil)
	if err != nil {
		return nil, fmt.Errorf("built-in agents: %w", err)
	}
	var found []agents.Agent
	for _, agent := range loaded {
		if agent.Builtin && detected(home, agent.Install.Detect) {
			found = append(found, agent)
		}
	}
	return found, nil
}

func detected(home, detect string) bool {
	if detect == "" {
		return true
	}
	path, err := expandHome(home, detect)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Plan lists what Apply will do for home, one step per change. artifacts
// is the build folder, whose built-in agents say which hooks are added.
func Plan(home, artifacts string) ([]PlannedStep, error) {
	where := locationsFor(home)
	steps := []PlannedStep{
		{Name: "binary", Description: "Copy the agent-screensaver binary to " + where.bin},
		{Name: "runtime", Description: "Copy the screen saver web page to " + where.paths.RuntimeDir},
		{Name: "extensions", Description: "Copy the built-in extensions to " + where.paths.ExtensionsDir + " (other extensions stay)"},
		{Name: "saver", Description: "Install the screen saver at " + where.saver},
		planLauncher(where),
	}
	found, err := installAgents(home, artifacts)
	if err != nil {
		return nil, err
	}
	prior, _ := LoadRecord(where.paths.InstallRecord, found)
	for _, agent := range found {
		agentSteps, err := PlanAgent(home, where.bin, agent, prior)
		if err != nil {
			return nil, err
		}
		steps = append(steps, agentSteps...)
	}
	return append(steps,
		PlannedStep{Name: "service", Description: "Start the daemon at every login (launch agent " + DaemonLabel + ")"},
		PlannedStep{Name: "tray", Description: "Show the menu bar icon at every login (launch agent " + TrayLabel + ")"},
	), nil
}

// Notes are the lines the installed agents ask to show after install.
func Notes(home, artifacts string) []string {
	found, _ := installAgents(home, artifacts)
	var notes []string
	for _, agent := range found {
		notes = append(notes, agent.Install.Notes...)
	}
	return notes
}

// PlanUninstall lists what Uninstall will do, read from the install record.
func PlanUninstall(home string) ([]PlannedStep, error) {
	where := locationsFor(home)
	record, err := loadInstalledRecord(where)
	if err != nil {
		return nil, err
	}
	var steps []PlannedStep
	for _, entry := range record.Agents {
		steps = append(steps, PlanRemoveAgent(home, entry)...)
	}
	for _, label := range record.Services {
		steps = append(steps, PlannedStep{Name: "service", Description: "Stop and remove the launch agent " + label})
	}
	steps = append(steps, planRemoveLauncher(where)...)
	return append(steps,
		PlannedStep{Name: "files", Description: fmt.Sprintf("Delete %s, %s, %s, the aerial clones in %s and the built-in extensions in %s",
			where.bin, where.paths.RuntimeDir, where.saver, where.paths.AerialClonesDir, where.paths.ExtensionsDir)},
		PlannedStep{Name: "record", Description: "Delete " + where.paths.InstallRecord + " (your settings and spool stay)"},
	), nil
}

// loadInstalledRecord reads the record, upgrading a version 1 record with
// the built-in agents installed in the extensions folder.
func loadInstalledRecord(where locations) (Record, error) {
	installed, _ := agents.Load(where.paths.ExtensionsDir)
	return LoadRecord(where.paths.InstallRecord, installed)
}

// configEdit is a config file change prepared in memory. Nothing is written
// until every file has been read, parsed and edited without error.
type configEdit struct {
	path     string
	original []byte // nil when the file does not exist
	updated  []byte
	backup   bool // install only, and only while the file is still the user's own
	remove   bool // the file install created holds nothing of the user's
}

// Apply installs everything. The source binary is bin; artifacts is a folder
// holding runtime, extensions and AgentScreensaver.saver. Every config file
// is checked before the first change, so invalid JSON aborts cleanly.
// The launcher app is added at the end; one that is not ours is skipped.
func Apply(home, bin, artifacts string, sm platform.ServiceManager, opts Options) (Record, error) {
	where := locationsFor(home)
	w, record, err := prepareInstall(home, artifacts, where)
	if err != nil {
		return Record{}, fmt.Errorf("cannot install: %w; nothing was changed", err)
	}
	if err := copyArtifacts(bin, artifacts, where); err != nil {
		return Record{}, err
	}
	// The record and the saved status line command go to disk before the first
	// user file is touched, so a failure later still leaves uninstall (and a
	// retry) able to find and restore everything. The service is listed before
	// it is installed for the same reason: a failed bootstrap can leave a plist.
	if err := saveStatusline(where.paths.ConfigFile, previousStatusline(record)); err != nil {
		return Record{}, err
	}
	if err := SaveRecord(where.paths.InstallRecord, record); err != nil {
		return Record{}, err
	}
	if err := w.write(); err != nil {
		return Record{}, err
	}
	if _, err := sm.Install(DaemonLabel, []string{where.bin, "daemon"}, where.paths.DaemonLog); err != nil {
		return Record{}, fmt.Errorf("install service %s: %w", DaemonLabel, err)
	}
	if _, err := sm.Install(TrayLabel, []string{where.bin, "tray"}, where.paths.TrayLog); err != nil {
		return Record{}, fmt.Errorf("install service %s: %w", TrayLabel, err)
	}
	// Last, so a failure here never stops the daemon from starting. Running
	// install again retries it.
	if err := installLauncher(where, opts); err != nil {
		return Record{}, err
	}
	return record, nil
}

// prepareInstall reads and edits every file in memory. Our own settings
// file is checked up front too: saving the status line later must never
// replace a damaged file with defaults.
func prepareInstall(home, artifacts string, where locations) (*workspace, Record, error) {
	if _, err := config.LoadStrict(where.paths.ConfigFile); err != nil {
		return nil, Record{}, err
	}
	if err := checkArtifacts(artifacts); err != nil {
		return nil, Record{}, err
	}
	found, err := installAgents(home, artifacts)
	if err != nil {
		return nil, Record{}, err
	}
	prior, _ := LoadRecord(where.paths.InstallRecord, found) // missing record: first install
	record := Record{Version: RecordVersion, Bin: where.bin, Saver: where.saver, Services: []string{DaemonLabel, TrayLabel}}
	if launcherStateOf(where.launcher) != launcherForeign {
		record.Launcher = where.launcher
	}
	priorBin := prior.Bin
	if priorBin == "" {
		priorBin = where.bin
	}
	w := newWorkspace(home, priorBin)
	for _, agent := range found {
		previous, _ := prior.agentRecord(agent.ID)
		entry, err := w.addAgent(where.bin, agent, previous)
		if err != nil {
			return nil, Record{}, err
		}
		record.Agents = append(record.Agents, entry)
	}
	// An agent an older install hooked that this build no longer adds (its
	// tool is gone, or so is its extension) keeps its hooks and its entry.
	for _, entry := range prior.Agents {
		if _, kept := record.agentRecord(entry.ID); !kept {
			record.Agents = append(record.Agents, entry)
		}
	}
	if _, err := w.edits(); err != nil {
		return nil, Record{}, err
	}
	return w, record, nil
}

// previousStatusline is the user's own status line command, which the
// status line runner keeps running. One agent has a status line today.
func previousStatusline(record Record) string {
	for _, entry := range record.Agents {
		if entry.PreviousCommand != "" {
			return entry.PreviousCommand
		}
	}
	return ""
}

// Uninstall reverses Apply as described by the install record. Services are
// stopped before the binary is deleted. If the config files cannot be read or
// a service cannot be removed, the record stays so the command can be run again.
func Uninstall(home string, sm platform.ServiceManager) error {
	where := locationsFor(home)
	record, err := loadInstalledRecord(where)
	if err != nil {
		return err
	}
	w := newWorkspace(home, record.Bin)
	for _, entry := range record.Agents {
		if err := w.removeAgent(entry); err != nil {
			return fmt.Errorf("cannot uninstall: %w; nothing was changed", err)
		}
	}
	if _, err := w.edits(); err != nil {
		return fmt.Errorf("cannot uninstall: %w; nothing was changed", err)
	}
	if err := w.write(); err != nil {
		return err
	}
	for _, label := range record.Services {
		if err := sm.Uninstall(label); err != nil {
			return fmt.Errorf("remove service %s: %w", label, err)
		}
	}
	if err := removeInstalledFiles(where); err != nil {
		return err
	}
	if err := removeLauncher(where); err != nil {
		return err
	}
	if err := clearSavedStatusline(where.paths.ConfigFile); err != nil {
		return err
	}
	return os.Remove(where.paths.InstallRecord)
}

// writeEdit writes the new content, after a backup of the old one when asked.
// A file that already holds our entries is not backed up: it is no longer the
// user's original, and the older backup of ours must survive a retry.
func writeEdit(edit configEdit) error {
	if edit.remove {
		if err := os.Remove(edit.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", edit.path, err)
		}
		return nil
	}
	if edit.backup {
		if err := writeFileAtomic(edit.path+backupSuffix, edit.original, 0o600); err != nil {
			return fmt.Errorf("back up %s: %w", edit.path, err)
		}
	}
	return writeFileAtomic(edit.path, edit.updated, 0o600)
}

// jsonEscaped is text as it appears inside a JSON string of our own writing.
func jsonEscaped(text string) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(text) // a string always encodes
	return strings.TrimSuffix(strings.TrimSpace(buffer.String()), `"`)[1:]
}

func saveStatusline(configFile, previous string) error {
	cfg, err := config.LoadStrict(configFile)
	if err != nil {
		return err
	}
	cfg.PreviousStatusline = previous
	return cfg.Save(configFile)
}

func clearSavedStatusline(configFile string) error {
	if _, err := os.Stat(configFile); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := config.LoadStrict(configFile); err != nil {
		return nil // a damaged file is the user's to fix; do not block uninstall
	}
	return saveStatusline(configFile, "")
}
