// Package config reads and writes the user's settings file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Config holds the user's settings. A missing field in the file keeps its
// default, so older files keep working when new settings are added.
type Config struct {
	Theme              string                    `json:"theme"`
	Background         string                    `json:"background"` // "" = the theme's background
	Options            map[string]map[string]any `json:"options"`    // extension id -> option key -> value
	Sound              bool                      `json:"sound"`
	SoundFile          string                    `json:"sound_file"`
	Privacy            bool                      `json:"privacy"`
	MaxFPS             int                       `json:"max_fps"` // 0 = display refresh rate; else 1..240
	PreviousStatusline string                    `json:"previous_statusline,omitempty"`
}

// Default returns the settings used when no file exists.
func Default() Config {
	return Config{
		Theme:     "builtin.default",
		Sound:     true,
		SoundFile: "/System/Library/Sounds/Glass.aiff",
	}
}

// Load reads path. A missing or broken file gives Default(): the saver must
// start even when the settings are damaged.
func Load(path string) Config {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default()
	}
	return cfg
}

// LoadStrict is for code that changes the file: a missing file gives
// Default(), but an unreadable or invalid one is an error, so a user's damaged
// settings are never silently replaced by defaults.
func LoadStrict(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config %s is not valid JSON: %w", path, err)
	}
	return cfg, nil
}

// Save writes the config to path through a temp file and a rename, so a
// crash never leaves a half-written file.
func (c Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer os.Remove(temp.Name()) // no-op after a successful rename
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
