package view

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
)

const fixtures = "testdata/extensions"

func installedWithout(t *testing.T, skip ...string) []extensions.Installed {
	t.Helper()
	dir, err := filepath.Abs(fixtures) // file URLs need absolute paths
	if err != nil {
		t.Fatal(err)
	}
	all := extensions.Scan(dir)
	if len(all) == 0 {
		t.Fatal("no fixtures found")
	}
	var kept []extensions.Installed
	for _, ext := range all {
		if !contains(skip, ext.Manifest.ID) {
			kept = append(kept, ext)
		}
	}
	return kept
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

func configWithColor(theme, color string) config.Config {
	cfg := config.Default()
	cfg.Theme = theme
	if color != "" {
		cfg.Options = map[string]map[string]any{"builtin.plain": {"color": color}}
	}
	return cfg
}

func TestResolveBackgroundOptions(t *testing.T) {
	tests := []struct {
		name      string
		cfg       config.Config
		wantID    string
		wantColor any
	}{
		{"theme over manifest default", configWithColor("builtin.default", ""), "builtin.plain", "#111111"},
		{"user over theme over default", configWithColor("builtin.default", "#333333"), "builtin.plain", "#333333"},
		{"invalid user option dropped", configWithColor("builtin.default", "red"), "builtin.plain", "#111111"},
		{"another theme uses its own background", configWithColor("acme.night", ""), "acme.rain", "#222222"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.cfg, installedWithout(t))
			if got.Background.ID != tt.wantID {
				t.Fatalf("background = %q, want %q", got.Background.ID, tt.wantID)
			}
			if got.Background.Options["color"] != tt.wantColor {
				t.Errorf("color = %v, want %v", got.Background.Options["color"], tt.wantColor)
			}
		})
	}
}

func TestResolveFallbacks(t *testing.T) {
	tests := []struct {
		name           string
		cfg            config.Config
		skip           []string
		wantTheme      string
		wantBackground string
	}{
		{"missing theme falls back to builtin default", configWithColor("nobody.nothing", ""), nil, "builtin.default", "builtin.plain"},
		{"theme that is not a theme falls back", configWithColor("builtin.clock", ""), nil, "builtin.default", "builtin.plain"},
		{"missing background extension falls back", configWithColor("acme.night", ""), []string{"acme.rain"}, "acme.night", "builtin.plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.cfg, installedWithout(t, tt.skip...))
			if got.Theme != tt.wantTheme || got.Background.ID != tt.wantBackground {
				t.Errorf("theme=%q background=%q, want %q and %q", got.Theme, got.Background.ID, tt.wantTheme, tt.wantBackground)
			}
			if len(got.Layout) == 0 {
				t.Error("layout is empty")
			}
		})
	}
}

func TestResolveWithoutBuiltinDefault(t *testing.T) {
	got := Resolve(config.Default(), nil)
	if got.Background.ID != "" || len(got.Components) != 0 || got.Layout != nil {
		t.Errorf("want empty view, got %+v", got)
	}
}

func TestResolveComponentsListedOnce(t *testing.T) {
	cfg := config.Default()
	cfg.Options = map[string]map[string]any{"builtin.clock": {"format": "12h"}}

	got := Resolve(cfg, installedWithout(t))

	var ids []string
	for _, component := range got.Components {
		ids = append(ids, component.ID)
	}
	if want := []string{"builtin.cards", "builtin.clock"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("components = %v, want %v", ids, want)
	}
	if got.Components[1].Options["format"] != "12h" {
		t.Errorf("clock options = %v, want user format", got.Components[1].Options)
	}
	if !strings.HasPrefix(got.Components[0].EntryURL, "file:///") {
		t.Errorf("entry url = %q, want file URL", got.Components[0].EntryURL)
	}
}

func TestResolveLayoutIsManifestLayout(t *testing.T) {
	got := Resolve(config.Default(), installedWithout(t))
	var parsed struct {
		Modes map[string]json.RawMessage `json:"modes"`
	}
	if err := json.Unmarshal(got.Layout, &parsed); err != nil || len(parsed.Modes) != 2 {
		t.Errorf("layout = %s (err %v), want two modes", got.Layout, err)
	}
}

func TestResolveStyleIsCopied(t *testing.T) {
	installed := installedWithout(t)
	got := Resolve(config.Default(), installed)
	if got.Style["accent"] != "#ff8800" {
		t.Fatalf("style = %v", got.Style)
	}
	got.Style["accent"] = "changed"
	if again := Resolve(config.Default(), installed); again.Style["accent"] != "#ff8800" {
		t.Error("mutating the view changed the manifest")
	}
}

func TestResolvePassesConfigThrough(t *testing.T) {
	tests := []struct {
		name    string
		privacy bool
		maxFPS  int
	}{
		{"privacy on", true, 0},
		{"privacy off and fps ceiling", false, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Privacy, cfg.MaxFPS = tt.privacy, tt.maxFPS
			got := Resolve(cfg, installedWithout(t))
			if got.Privacy != tt.privacy || got.MaxFPS != tt.maxFPS {
				t.Errorf("privacy=%v fps=%d, want %v and %d", got.Privacy, got.MaxFPS, tt.privacy, tt.maxFPS)
			}
		})
	}
}
