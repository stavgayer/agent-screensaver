// Package settings is the one place that changes the user's settings. The
// CLI, the menu bar app and the settings window all call it, so the rules
// (what is valid, what is stored, how files are copied) exist only once.
package settings

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"agent-screensaver/internal/aerials"
	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/install"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/view"
)

const (
	maxFPSLimit = 240
	// maxFolderName is the longest file name macOS allows.
	maxFolderName = 255
	builtinPrefix = "builtin."
)

// Service reads and changes settings under one data directory. Every change
// is saved at once and atomically; the daemon sees it by the file's mtime.
type Service struct {
	Paths paths.Paths
}

// Snapshot is everything a settings screen needs in one read.
type Snapshot struct {
	Config     config.Config   `json:"config"`
	Background string          `json:"background"` // the background in effect: the user's or the theme's
	Extensions []ExtensionInfo `json:"extensions"`
}

// ExtensionInfo is one installed extension with the values in effect.
type ExtensionInfo struct {
	Manifest extensions.Manifest `json:"manifest"`
	Values   map[string]any      `json:"values"` // manifest defaults overlaid with the user's options
	Builtin  bool                `json:"builtin"`
}

// Snapshot returns the config and every installed extension, sorted by id.
func (s Service) Snapshot() (Snapshot, error) {
	cfg := config.Load(s.Paths.ConfigFile)
	installed := extensions.Scan(s.Paths.ExtensionsDir)
	sort.Slice(installed, func(i, j int) bool { return installed[i].Manifest.ID < installed[j].Manifest.ID })
	infos := make([]ExtensionInfo, 0, len(installed))
	for _, ext := range installed {
		manifest := ext.Manifest
		infos = append(infos, ExtensionInfo{
			Manifest: manifest,
			Values:   currentValues(manifest, cfg.Options[manifest.ID]),
			Builtin:  strings.HasPrefix(manifest.ID, builtinPrefix),
		})
	}
	return Snapshot{Config: cfg, Background: view.Background(cfg, installed).ID, Extensions: infos}, nil
}

// currentValues overlays the user's options on the manifest defaults. Stored
// values for options the manifest no longer declares are left out.
func currentValues(manifest extensions.Manifest, user map[string]any) map[string]any {
	values := make(map[string]any, len(manifest.Options))
	for key, spec := range manifest.Options {
		var value any
		if err := json.Unmarshal(spec.Default, &value); err == nil {
			values[key] = value
		}
		if override, ok := user[key]; ok {
			values[key] = override
		}
	}
	return values
}

// noAerialsHint is shown when the chosen aerials group has no downloaded video.
const noAerialsHint = "Pick it once in System Settings → Screen Saver, and macOS will download the videos."

// Notice returns a message about an extension's current settings that the
// user should see, or "" when there is none. Today the only one says how to
// get aerial videos when the chosen group has none downloaded.
func (s Service) Notice(extID string) string {
	if extID != aerials.ExtensionID {
		return ""
	}
	ext, err := s.find(extID)
	if err != nil {
		return ""
	}
	values := currentValues(ext.Manifest, config.Load(s.Paths.ConfigFile).Options[extID])
	group, ok := aerials.ParseGroup(values[aerials.GroupOption])
	if !ok || len(s.Paths.Aerials.Downloaded(group)) > 0 {
		return ""
	}
	return noAerialsHint
}

// SetTheme selects an installed extension of type theme.
func (s Service) SetTheme(id string) error {
	ext, err := s.find(id)
	if err != nil {
		return err
	}
	if ext.Manifest.Type != extensions.TypeTheme {
		return fmt.Errorf("%q is a %s, not a theme", id, ext.Manifest.Type)
	}
	return s.update(func(cfg *config.Config) error {
		cfg.Theme = id
		return nil
	})
}

// SetBackground makes an installed background replace the theme's. An empty
// id goes back to the theme's background.
func (s Service) SetBackground(id string) error {
	if id != "" {
		ext, err := s.find(id)
		if err != nil {
			return err
		}
		if ext.Manifest.Type != extensions.TypeBackground {
			return fmt.Errorf("%q is a %s, not a background", id, ext.Manifest.Type)
		}
	}
	return s.update(func(cfg *config.Config) error {
		cfg.Background = id
		return nil
	})
}

// SetOption parses raw according to the option's kind, validates it and
// stores it as a typed value. A file option is copied into MediaDir first,
// because the screen saver can read the data directory but nothing else.
func (s Service) SetOption(extID, key, raw string) error {
	spec, err := s.optionSpec(extID, key)
	if err != nil {
		return err
	}
	value, err := parseOption(spec, raw)
	if err != nil {
		return fmt.Errorf("option %s %s: %w", extID, key, err)
	}
	if err := extensions.ValidateOption(spec, value); err != nil {
		return fmt.Errorf("option %s %s: %w", extID, key, err)
	}
	if spec.Kind == extensions.OptionFile {
		if value, err = s.copyIntoMedia(extID, key, value.(string)); err != nil {
			return fmt.Errorf("option %s %s: %w", extID, key, err)
		}
	}
	var previous any
	err = s.update(func(cfg *config.Config) error {
		previous = cfg.Options[extID][key]
		if cfg.Options == nil {
			cfg.Options = map[string]map[string]any{}
		}
		if cfg.Options[extID] == nil {
			cfg.Options[extID] = map[string]any{}
		}
		cfg.Options[extID][key] = value
		return nil
	})
	if err == nil && previous != value {
		s.removeMediaCopy(extID, key, previous) // the replaced copy is garbage now
	}
	return err
}

// ResetOption drops the user's value so the manifest default applies again.
// An empty key resets every option of the extension.
func (s Service) ResetOption(extID, key string) error {
	ext, err := s.find(extID)
	if err != nil {
		return err
	}
	if _, declared := ext.Manifest.Options[key]; key != "" && !declared {
		return fmt.Errorf("extension %q has no option %q", extID, key)
	}
	var dropped map[string]any
	err = s.update(func(cfg *config.Config) error {
		dropped = map[string]any{}
		for name, value := range cfg.Options[extID] {
			if key == "" || name == key {
				dropped[name] = value
				delete(cfg.Options[extID], name)
			}
		}
		if len(cfg.Options[extID]) == 0 {
			delete(cfg.Options, extID)
		}
		return nil
	})
	if err == nil {
		for name, value := range dropped {
			s.removeMediaCopy(extID, name, value)
		}
	}
	return err
}

// SetPrivacy turns privacy mode on or off.
func (s Service) SetPrivacy(on bool) error {
	return s.update(func(cfg *config.Config) error {
		cfg.Privacy = on
		return nil
	})
}

// SetSound turns the attention sound on or off. A non-empty file replaces the
// sound; an empty one keeps the current sound.
func (s Service) SetSound(on bool, file string) error {
	if file != "" {
		// The daemon plays the sound from outside the sandbox, so the file
		// stays where it is; only its absolute path is stored.
		absolute, err := existingFile(file)
		if err != nil {
			return fmt.Errorf("sound file: %w", err)
		}
		file = absolute
	}
	return s.update(func(cfg *config.Config) error {
		cfg.Sound = on
		if file != "" {
			cfg.SoundFile = file
		}
		return nil
	})
}

// SetMaxFPS sets the frame-rate ceiling: 0 follows the display, else 1..240.
func (s Service) SetMaxFPS(fps int) error {
	if fps < 0 || fps > maxFPSLimit {
		return fmt.Errorf("max fps %d is out of range, use 0 (display) or 1..%d", fps, maxFPSLimit)
	}
	return s.update(func(cfg *config.Config) error {
		cfg.MaxFPS = fps
		return nil
	})
}

// SetIgnoredProjects replaces the list of folders whose sessions get no
// card. Each entry is stored as a folder name (a path keeps its last part,
// which is what events carry); blanks and repeats are dropped.
func (s Service) SetIgnoredProjects(entries []string) error {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := config.ProjectName(entry)
		if utf8.RuneCountInString(name) > maxFolderName {
			return fmt.Errorf("%q is longer than a folder name can be", name)
		}
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return s.update(func(cfg *config.Config) error {
		cfg.IgnoredProjects = names
		return nil
	})
}

// InstallExtension installs an extension from a folder or a .zip file. An
// agent extension must also pass every agent check and the recipe safety
// rules; installing it adds no hooks (that is a separate, confirmed step).
func (s Service) InstallExtension(path string) (extensions.Manifest, error) {
	return extensions.InstallChecked(path, s.Paths.ExtensionsDir, s.checkAgent)
}

// checkAgent runs on the staged copy. It also leaves the execute bit only
// on the programs an agent manifest names as converters: every other file
// of any extension is 0644, so nothing else in it can be run by accident.
func (s Service) checkAgent(m extensions.Manifest, dir string) error {
	if m.Type != extensions.TypeAgent {
		return extensions.KeepExecutable(dir, nil)
	}
	agent, err := agents.FromManifest(m, dir)
	if err != nil {
		return err
	}
	if err := install.CheckRecipe(s.Paths.Home, s.Paths.DataDir, agent.Install); err != nil {
		return err
	}
	return extensions.KeepExecutable(dir, install.Programs(agent))
}

// RemoveExtension removes an installed extension. Built-ins are refused. An
// agent extension's hooks are removed first; when that fails, the
// extension stays, so its hooks can still be removed later.
func (s Service) RemoveExtension(id string) error {
	if err := s.removeAgentHooks(id); err != nil {
		return fmt.Errorf("remove %s: its hooks could not be removed, so it stays: %w", id, err)
	}
	return extensions.Remove(id, s.Paths.ExtensionsDir)
}

func (s Service) removeAgentHooks(id string) error {
	if _, err := os.Stat(s.Paths.InstallRecord); os.IsNotExist(err) {
		return nil // nothing installed, so no hooks were added
	}
	loaded, _ := agents.Load(s.Paths.ExtensionsDir)
	record, err := install.LoadRecord(s.Paths.InstallRecord, loaded)
	if err != nil {
		return err
	}
	return install.RemoveAgentHooks(s.Paths.Home, agents.Agent{ID: id}, &record)
}

// update loads the config, applies change and saves it. When change or the
// save fails, the file on disk is untouched.
func (s Service) update(change func(cfg *config.Config) error) error {
	cfg, err := config.LoadStrict(s.Paths.ConfigFile)
	if err != nil {
		return err
	}
	if err := change(&cfg); err != nil {
		return err
	}
	if err := cfg.Save(s.Paths.ConfigFile); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

func (s Service) find(id string) (extensions.Installed, error) {
	for _, ext := range extensions.Scan(s.Paths.ExtensionsDir) {
		if ext.Manifest.ID == id {
			return ext, nil
		}
	}
	return extensions.Installed{}, fmt.Errorf("extension %q is not installed", id)
}

func (s Service) optionSpec(extID, key string) (extensions.OptionSpec, error) {
	ext, err := s.find(extID)
	if err != nil {
		return extensions.OptionSpec{}, err
	}
	spec, ok := ext.Manifest.Options[key]
	if !ok {
		return extensions.OptionSpec{}, fmt.Errorf("extension %q has no option %q", extID, key)
	}
	return spec, nil
}

// parseOption turns the text typed by the user into the Go type that
// extensions.ValidateOption expects for the kind. Range, color and enum
// checks stay in ValidateOption, so the rules live in one place.
func parseOption(spec extensions.OptionSpec, raw string) (any, error) {
	switch spec.Kind {
	case extensions.OptionNumber:
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, fmt.Errorf("%q is not a number", raw)
		}
		return number, nil
	case extensions.OptionBoolean:
		return parseBoolean(raw)
	case extensions.OptionFile:
		return existingFile(raw)
	default: // string, color, enum
		return raw, nil
	}
}

func parseBoolean(raw string) (bool, error) {
	switch strings.ToLower(raw) {
	case "true", "on", "yes":
		return true, nil
	case "false", "off", "no":
		return false, nil
	}
	return false, fmt.Errorf("%q is not a boolean, use true|false|on|off|yes|no", raw)
}

// existingFile returns the absolute path of a regular file.
func existingFile(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q is not a regular file", path)
	}
	return absolute, nil
}

// mediaFolder is where the copies of one option live. One folder per option
// keeps two options with same-named source files from overwriting each other.
func (s Service) mediaFolder(extID, key string) string {
	return filepath.Join(s.Paths.MediaDir, extID, key)
}

// removeMediaCopy deletes the file a file option pointed at, but only when it
// is inside that option's own media folder, so a stored path can never make
// us delete something else.
func (s Service) removeMediaCopy(extID, key string, stored any) {
	path, ok := stored.(string)
	if !ok || path == "" {
		return
	}
	folder := s.mediaFolder(extID, key)
	if relative, err := filepath.Rel(folder, path); err != nil || !filepath.IsLocal(relative) {
		return
	}
	_ = os.Remove(path) // best effort: a stale copy is harmless
}

// copyIntoMedia copies source to MediaDir/<extID>/<key>/<base name>,
// replacing an older copy, and returns the new absolute path. It writes a
// temp file and renames it, so a failed copy never leaves a half-written
// media file.
func (s Service) copyIntoMedia(extID, key, source string) (string, error) {
	if key != filepath.Base(key) || key == "." || !filepath.IsLocal(key) {
		return "", fmt.Errorf("option name %q cannot be used as a folder name", key)
	}
	folder := s.mediaFolder(extID, key)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return "", fmt.Errorf("create media folder: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer in.Close()
	temp, err := os.CreateTemp(folder, ".copy-*")
	if err != nil {
		return "", fmt.Errorf("create temp media file: %w", err)
	}
	defer os.Remove(temp.Name()) // no-op after a successful rename
	// CreateTemp makes the file private; the screen saver must be able to read it.
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return "", fmt.Errorf("set media file mode: %w", err)
	}
	if _, err := io.Copy(temp, in); err != nil {
		temp.Close()
		return "", fmt.Errorf("copy file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close media file: %w", err)
	}
	target := filepath.Join(folder, filepath.Base(source))
	if err := os.Rename(temp.Name(), target); err != nil {
		return "", fmt.Errorf("replace media file: %w", err)
	}
	return target, nil
}
