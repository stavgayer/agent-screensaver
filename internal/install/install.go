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
	"agent-screensaver/internal/jsonedit"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
)

const (
	// DaemonLabel names the launchd service that runs the daemon.
	DaemonLabel = "dev.agentscreensaver.daemon"

	backupSuffix  = ".agent-screensaver.bak"
	binaryName    = "agent-screensaver"
	saverName     = "AgentScreensaver.saver"
	builtinPrefix = "builtin."
)

// PlannedStep is one thing install or uninstall will do, shown to the user
// before anything changes.
type PlannedStep struct {
	Name, Description string
}

// Record says what Apply did, so Uninstall can reverse exactly that.
type Record struct {
	Bin                string   `json:"bin"`
	Agents             []string `json:"agents"`
	PreviousStatusline string   `json:"previous_statusline,omitempty"`
	Services           []string `json:"services"` // service labels
	Saver              string   `json:"saver"`
}

// locations are the files install owns, derived from the home directory.
type locations struct {
	paths paths.Paths
	bin   string
	saver string
}

func locationsFor(home string) locations {
	p := paths.ForHome(home)
	return locations{
		paths: p,
		bin:   filepath.Join(p.DataDir, "bin", binaryName),
		saver: filepath.Join(home, "Library", "Screen Savers", saverName),
	}
}

// hookInstallers lists the agents we can install hooks for, by name.
func hookInstallers() []namedInstaller {
	var found []namedInstaller
	for _, adapter := range agents.All() {
		if installer, ok := adapter.(agents.HookInstaller); ok {
			found = append(found, namedInstaller{name: adapter.Name(), installer: installer, adapter: adapter})
		}
	}
	return found
}

type namedInstaller struct {
	name      string
	installer agents.HookInstaller
	adapter   agents.Adapter
}

func (n namedInstaller) hookCommand(bin string) string {
	return shellQuote(bin) + " hook --tool " + n.name
}

func statuslineCommand(bin string) string { return shellQuote(bin) + " statusline" }

// shellQuote quotes for sh. The installed path contains a space, and the
// tools run the command through a shell.
func shellQuote(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// Plan lists what Apply will do for home, one step per change.
func Plan(home string) []PlannedStep {
	where := locationsFor(home)
	steps := []PlannedStep{
		{"binary", "Copy the agent-screensaver binary to " + where.bin},
		{"runtime", "Copy the screen saver web page to " + where.paths.RuntimeDir},
		{"extensions", "Copy the built-in extensions to " + where.paths.ExtensionsDir + " (other extensions stay)"},
		{"saver", "Install the screen saver at " + where.saver},
	}
	for _, target := range hookInstallers() {
		path := target.installer.ConfigPath(home)
		steps = append(steps, PlannedStep{"hooks-" + target.name,
			fmt.Sprintf("Add %s hooks to %s (backup: %s%s)", target.name, path, path, backupSuffix)})
		if _, isClaude := target.adapter.(agents.Claude); isClaude {
			steps = append(steps, PlannedStep{"statusline",
				"Point the Claude status line in " + path + " at agent-screensaver (your command is saved and still runs)"})
		}
	}
	return append(steps, PlannedStep{"service", "Start the daemon at every login (launch agent " + DaemonLabel + ")"})
}

// PlanUninstall lists what Uninstall will do, read from the install record.
func PlanUninstall(home string) ([]PlannedStep, error) {
	where := locationsFor(home)
	record, err := loadRecord(where.paths.InstallRecord)
	if err != nil {
		return nil, err
	}
	var steps []PlannedStep
	for _, name := range record.Agents {
		if target, ok := installerByName(name); ok {
			steps = append(steps, PlannedStep{"hooks-" + name, "Remove our " + name + " hooks from " + target.installer.ConfigPath(home)})
		}
	}
	for _, label := range record.Services {
		steps = append(steps, PlannedStep{"service", "Stop and remove the launch agent " + label})
	}
	return append(steps,
		PlannedStep{"files", fmt.Sprintf("Delete %s, %s, %s and the built-in extensions in %s",
			where.bin, where.paths.RuntimeDir, where.saver, where.paths.ExtensionsDir)},
		PlannedStep{"record", "Delete " + where.paths.InstallRecord + " (your settings and spool stay)"},
	), nil
}

func installerByName(name string) (namedInstaller, bool) {
	for _, target := range hookInstallers() {
		if target.name == name {
			return target, true
		}
	}
	return namedInstaller{}, false
}

// configEdit is a config file change prepared in memory. Nothing is written
// until every file has been read, parsed and edited without error.
type configEdit struct {
	path     string
	original []byte // nil when the file does not exist
	updated  []byte
	backup   bool // install only, and only while the file is still the user's own
}

// Apply installs everything. The source binary is bin; artifacts is a folder
// holding runtime, extensions and AgentScreensaver.saver. Every config file
// is checked before the first change, so invalid JSON aborts cleanly.
func Apply(home, bin, artifacts string, sm platform.ServiceManager) (Record, error) {
	where := locationsFor(home)
	prior, _ := loadRecord(where.paths.InstallRecord) // missing record: first install
	// Our own settings file is checked up front too: saving the status line
	// later must never replace a damaged file with defaults.
	_, err := config.LoadStrict(where.paths.ConfigFile)
	var edits []configEdit
	var previous string
	if err == nil {
		edits, previous, err = prepareInstall(home, where.bin, prior.PreviousStatusline)
	}
	if err != nil {
		return Record{}, fmt.Errorf("cannot install: %w; nothing was changed", err)
	}
	if err := checkArtifacts(artifacts); err != nil {
		return Record{}, fmt.Errorf("cannot install: %w; nothing was changed", err)
	}
	if err := copyArtifacts(bin, artifacts, where); err != nil {
		return Record{}, err
	}
	// The record and the saved status line command go to disk before the first
	// user file is touched, so a failure later still leaves uninstall (and a
	// retry) able to find and restore everything. The service is listed before
	// it is installed for the same reason: a failed bootstrap can leave a plist.
	record := Record{Bin: where.bin, PreviousStatusline: previous, Saver: where.saver, Services: []string{DaemonLabel}}
	for _, target := range hookInstallers() {
		record.Agents = append(record.Agents, target.name)
	}
	if err := saveStatusline(where.paths.ConfigFile, previous); err != nil {
		return Record{}, err
	}
	if err := saveRecord(where.paths.InstallRecord, record); err != nil {
		return Record{}, err
	}
	for _, edit := range edits {
		if err := writeEdit(edit); err != nil {
			return Record{}, err
		}
	}
	if _, err := sm.Install(DaemonLabel, []string{where.bin, "daemon"}); err != nil {
		return Record{}, fmt.Errorf("install service %s: %w", DaemonLabel, err)
	}
	return record, nil
}

func prepareInstall(home, bin, savedPrevious string) (edits []configEdit, previous string, err error) {
	previous = savedPrevious
	for _, target := range hookInstallers() {
		path := target.installer.ConfigPath(home)
		cfg, original, err := readConfig(path)
		if err != nil {
			return nil, "", err
		}
		if err := target.installer.AddHooks(cfg, target.hookCommand(bin)); err != nil {
			return nil, "", fmt.Errorf("%s: %w", path, err)
		}
		if _, isClaude := target.adapter.(agents.Claude); isClaude {
			swapped, err := agents.SwapStatusline(cfg, statuslineCommand(bin))
			if err != nil {
				return nil, "", fmt.Errorf("%s: %w", path, err)
			}
			if swapped != "" {
				previous = swapped
			}
		}
		updated, err := cfg.Marshal()
		if err != nil {
			return nil, "", fmt.Errorf("%s: %w", path, err)
		}
		edits = append(edits, configEdit{path: path, original: original, updated: updated,
			backup: original != nil && !strings.Contains(string(original), jsonEscaped(target.hookCommand(bin)))})
	}
	return edits, previous, nil
}

// Uninstall reverses Apply as described by the install record. Services are
// stopped before the binary is deleted. If the config files cannot be read or
// a service cannot be removed, the record stays so the command can be run again.
func Uninstall(home string, sm platform.ServiceManager) error {
	where := locationsFor(home)
	record, err := loadRecord(where.paths.InstallRecord)
	if err != nil {
		return err
	}
	edits, err := prepareUninstall(home, record)
	if err != nil {
		return fmt.Errorf("cannot uninstall: %w; nothing was changed", err)
	}
	for _, edit := range edits {
		if err := writeEdit(edit); err != nil {
			return err
		}
	}
	for _, label := range record.Services {
		if err := sm.Uninstall(label); err != nil {
			return fmt.Errorf("remove service %s: %w", label, err)
		}
	}
	if err := removeInstalledFiles(where); err != nil {
		return err
	}
	if err := clearSavedStatusline(where.paths.ConfigFile); err != nil {
		return err
	}
	return os.Remove(where.paths.InstallRecord)
}

func prepareUninstall(home string, record Record) ([]configEdit, error) {
	var edits []configEdit
	for _, name := range record.Agents {
		target, ok := installerByName(name)
		if !ok {
			continue
		}
		path := target.installer.ConfigPath(home)
		cfg, original, err := readConfig(path)
		if err != nil {
			return nil, err
		}
		if original == nil {
			continue // the user deleted the file; nothing of ours is left in it
		}
		if err := target.installer.RemoveHooks(cfg, target.hookCommand(record.Bin)); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if _, isClaude := target.adapter.(agents.Claude); isClaude {
			if err := agents.RestoreStatusline(cfg, record.PreviousStatusline, statuslineCommand(record.Bin)); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		}
		updated, err := cfg.Marshal()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		edits = append(edits, configEdit{path: path, original: original, updated: updated})
	}
	return edits, nil
}

var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// readConfig reads a user file. A missing file is an empty object with a nil
// original. A file that is not exactly one JSON object is an error.
func readConfig(path string) (*jsonedit.Object, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &jsonedit.Object{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	if strings.HasPrefix(string(data), string(byteOrderMark)) {
		return nil, nil, fmt.Errorf("%s starts with a UTF-8 byte order mark; remove it and run again", path)
	}
	cfg, err := jsonedit.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is not valid JSON (%v)", path, err)
	}
	return cfg, data, nil
}

// writeEdit writes the new content, after a backup of the old one when asked.
// A file that already holds our entries is not backed up: it is no longer the
// user's original, and the older backup of ours must survive a retry.
func writeEdit(edit configEdit) error {
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

func loadRecord(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, fmt.Errorf("agent-screensaver is not installed (no record at %s)", path)
	}
	if err != nil {
		return Record{}, fmt.Errorf("read install record: %w", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("install record %s is damaged: %w", path, err)
	}
	return record, nil
}

func saveRecord(path string, record Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install record: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'), 0o644)
}
