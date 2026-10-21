package view

import (
	"encoding/json"
	"reflect"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
)

// aerialsInstalled is a theme whose background is backgroundID, the aerials
// and Matrix backgrounds, and a layout, all built in memory.
func aerialsInstalled(backgroundID string, withMatrix bool) []extensions.Installed {
	number := func(value float64) *float64 { return &value }
	theme := extensions.Manifest{ID: "builtin.default", Type: extensions.TypeTheme,
		Theme: &extensions.ThemeSpec{Background: extensions.ExtRef{ID: backgroundID}, Layout: "t.layout"}}
	layout := extensions.Manifest{ID: "t.layout", Type: extensions.TypeLayout, Layout: json.RawMessage(`{"modes":{}}`)}
	aerials := extensions.Manifest{ID: "builtin.aerials", Type: extensions.TypeBackground, Entry: "index.js",
		Options: map[string]extensions.OptionSpec{
			"group": {Kind: extensions.OptionEnum, Values: []string{"cities", "landscapes", "space", "underwater"}, Default: json.RawMessage(`"cities"`)},
			"dim":   {Kind: extensions.OptionNumber, Min: number(0), Max: number(0.8), Default: json.RawMessage(`0.25`)},
		}}
	matrix := extensions.Manifest{ID: "builtin.matrix", Type: extensions.TypeBackground, Entry: "index.js",
		Options: map[string]extensions.OptionSpec{
			"speed": {Kind: extensions.OptionNumber, Min: number(0.25), Max: number(4), Default: json.RawMessage(`1`)},
		}}
	other := extensions.Manifest{ID: "t.bg", Type: extensions.TypeBackground, Entry: "index.js"}
	installed := []extensions.Installed{
		{Manifest: theme}, {Manifest: layout},
		{Manifest: aerials, Dir: "/x/builtin.aerials"},
		{Manifest: other, Dir: "/x/t.bg"},
	}
	if withMatrix {
		installed = append(installed, extensions.Installed{Manifest: matrix, Dir: "/x/builtin.matrix"})
	}
	return installed
}

func TestResolveAerials(t *testing.T) {
	cfg := config.Default()
	cfg.Options = map[string]map[string]any{
		"builtin.aerials": {"group": "space", "files": []any{"file:///etc/passwd"}},
		"builtin.matrix":  {"speed": 2.0},
	}
	clones := []string{"/d/media/aerials/A.mov", "/d/Application Support/B.mov"}
	tests := []struct {
		name        string
		background  string
		withMatrix  bool
		clones      []string
		wantID      string
		wantEntry   string
		wantOptions map[string]any
	}{
		{"clones become file URLs in the given order", "builtin.aerials", true, clones,
			"builtin.aerials", "file:///x/builtin.aerials/index.js",
			map[string]any{"group": "space", "dim": 0.25, "files": []string{
				"file:///d/media/aerials/A.mov", "file:///d/Application%20Support/B.mov"}}},
		{"no clones falls back to Matrix with the user's Matrix options", "builtin.aerials", true, nil,
			"builtin.matrix", "file:///x/builtin.matrix/index.js", map[string]any{"speed": 2.0}},
		{"no clones and no Matrix keeps aerials with no files", "builtin.aerials", false, []string{},
			"builtin.aerials", "file:///x/builtin.aerials/index.js",
			map[string]any{"group": "space", "dim": 0.25, "files": []string{}}},
		{"another background ignores the clones", "t.bg", true, clones,
			"t.bg", "file:///x/t.bg/index.js", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(cfg, aerialsInstalled(tt.background, tt.withMatrix), tt.clones).Background
			if got.ID != tt.wantID || got.EntryURL != tt.wantEntry {
				t.Fatalf("background = %s %s, want %s %s", got.ID, got.EntryURL, tt.wantID, tt.wantEntry)
			}
			if !reflect.DeepEqual(got.Options, tt.wantOptions) {
				t.Errorf("options = %#v, want %#v", got.Options, tt.wantOptions)
			}
		})
	}
}

func TestBackgroundIsTheThemeChoiceBeforeFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Options = map[string]map[string]any{"builtin.aerials": {"group": "underwater"}}
	tests := []struct {
		name      string
		installed []extensions.Installed
		wantID    string
		wantGroup any
	}{
		{"aerials theme", aerialsInstalled("builtin.aerials", true), "builtin.aerials", "underwater"},
		{"other background", aerialsInstalled("t.bg", true), "t.bg", nil},
		{"nothing installed", nil, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Background(cfg, tt.installed)
			if got.ID != tt.wantID || got.Options["group"] != tt.wantGroup {
				t.Errorf("Background = %s %v, want %s with group %v", got.ID, got.Options, tt.wantID, tt.wantGroup)
			}
		})
	}
}

func TestResolveBackgroundOverride(t *testing.T) {
	clones := []string{"/d/media/aerials/A.mov"}
	tests := []struct {
		name       string
		override   string
		clones     []string
		wantID     string
		wantOption map[string]any
	}{
		{"no override uses the theme's background", "", clones, "t.bg", map[string]any{}},
		{"installed background overrides the theme", "builtin.aerials", clones, "builtin.aerials",
			map[string]any{"group": "space", "dim": 0.25, "files": []string{"file:///d/media/aerials/A.mov"}}},
		{"override still falls back to Matrix with the user's options", "builtin.aerials", nil, "builtin.matrix",
			map[string]any{"speed": 2.0}},
		{"unknown id uses the theme's background", "nobody.none", clones, "t.bg", map[string]any{}},
		{"a non-background id uses the theme's background", "t.layout", clones, "t.bg", map[string]any{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Background = tt.override
			cfg.Options = map[string]map[string]any{"builtin.aerials": {"group": "space"}, "builtin.matrix": {"speed": 2.0}}
			installed := aerialsInstalled("t.bg", true)
			got := Resolve(cfg, installed, tt.clones).Background
			if got.ID != tt.wantID || !reflect.DeepEqual(got.Options, tt.wantOption) {
				t.Errorf("background = %s %#v, want %s %#v", got.ID, got.Options, tt.wantID, tt.wantOption)
			}
			if before := Background(cfg, installed); tt.override == "builtin.aerials" && before.ID != "builtin.aerials" {
				t.Errorf("Background = %s, want the override before the fallback", before.ID)
			}
		})
	}
}
