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
	"sort"
	"strconv"
	"strings"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/paths"
)

const (
	maxFPSLimit   = 240
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
	return Snapshot{Config: cfg, Extensions: infos}, nil
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
		if value, err = s.copyIntoMedia(extID, value.(string)); err != nil {
			return fmt.Errorf("option %s %s: %w", extID, key, err)
		}
	}
	return s.update(func(cfg *config.Config) error {
		if cfg.Options == nil {
			cfg.Options = map[string]map[string]any{}
		}
		if cfg.Options[extID] == nil {
			cfg.Options[extID] = map[string]any{}
		}
		cfg.Options[extID][key] = value
		return nil
	})
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
	return s.update(func(cfg *config.Config) error {
		if key == "" {
			delete(cfg.Options, extID)
			return nil
		}
		delete(cfg.Options[extID], key)
		if len(cfg.Options[extID]) == 0 {
			delete(cfg.Options, extID)
		}
		return nil
	})
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
		if _, err := os.Stat(file); err != nil {
			return fmt.Errorf("sound file: %w", err)
		}
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

// InstallExtension installs an extension from a folder or a .zip file.
func (s Service) InstallExtension(path string) (extensions.Manifest, error) {
	return extensions.InstallFrom(path, s.Paths.ExtensionsDir)
}

// RemoveExtension removes an installed extension. Built-ins are refused.
func (s Service) RemoveExtension(id string) error {
	return extensions.Remove(id, s.Paths.ExtensionsDir)
}

// update loads the config, applies change and saves it. When change or the
// save fails, the file on disk is untouched.
func (s Service) update(change func(cfg *config.Config) error) error {
	cfg := config.Load(s.Paths.ConfigFile)
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

// copyIntoMedia copies source to MediaDir/<extID>/<base name>, replacing an
// older copy, and returns the new absolute path. It writes a temp file and
// renames it, so a failed copy never leaves a half-written media file.
func (s Service) copyIntoMedia(extID, source string) (string, error) {
	folder := filepath.Join(s.Paths.MediaDir, extID)
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
