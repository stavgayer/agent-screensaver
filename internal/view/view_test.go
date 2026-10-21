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

// sanitizeInstalled builds a minimal set of extensions around the given
// layout JSON: a theme, a background, a layout and the listed extras.
func sanitizeInstalled(layout string, extras ...extensions.Installed) []extensions.Installed {
	number := func(value float64) *float64 { return &value }
	sizeSpec := extensions.OptionSpec{Kind: extensions.OptionEnum, Values: []string{"large", "small"}, Default: json.RawMessage(`"large"`)}
	scaleSpec := extensions.OptionSpec{Kind: extensions.OptionNumber, Min: number(0), Max: number(10), Default: json.RawMessage(`1`)}
	manifest := func(id string, kind extensions.Type) extensions.Manifest {
		return extensions.Manifest{ID: id, Type: kind, Entry: "index.js"}
	}
	card := manifest("t.card", extensions.TypeComponent)
	card.Options = map[string]extensions.OptionSpec{"size": sizeSpec, "scale": scaleSpec}
	theme := manifest("builtin.default", extensions.TypeTheme)
	theme.Theme = &extensions.ThemeSpec{Background: extensions.ExtRef{ID: "t.bg"}, Layout: "t.layout"}
	layoutManifest := manifest("t.layout", extensions.TypeLayout)
	layoutManifest.Layout = json.RawMessage(layout)
	installed := []extensions.Installed{
		{Manifest: theme}, {Manifest: layoutManifest},
		{Manifest: manifest("t.bg", extensions.TypeBackground), Dir: "/x/t.bg"},
		{Manifest: card, Dir: "/x/t.card"},
		{Manifest: manifest("t.plain", extensions.TypeComponent), Dir: "/x/t.plain"},
	}
	return append(installed, extras...)
}

func sanitizedLayout(t *testing.T, layout string) map[string]map[string][]extensions.ExtRef {
	t.Helper()
	got := Resolve(config.Default(), sanitizeInstalled(layout))
	var parsed layoutModes
	if err := json.Unmarshal(got.Layout, &parsed); err != nil {
		t.Fatalf("layout %s: %v", got.Layout, err)
	}
	return parsed.Modes
}

func TestResolveSanitizesPlacements(t *testing.T) {
	tests := []struct {
		name   string
		layout string
		want   []extensions.ExtRef
	}{
		{"placement option not in schema is dropped",
			`{"modes":{"idle":{"center":[{"id":"t.card","options":{"bogus":1}}]}}}`,
			[]extensions.ExtRef{{ID: "t.card"}}},
		{"invalid placement option is dropped",
			`{"modes":{"idle":{"center":[{"id":"t.card","options":{"size":"huge"}}]}}}`,
			[]extensions.ExtRef{{ID: "t.card"}}},
		{"valid placement option is kept",
			`{"modes":{"idle":{"center":[{"id":"t.card","options":{"size":"small","scale":2,"bogus":1}}]}}}`,
			[]extensions.ExtRef{{ID: "t.card", Options: map[string]any{"size": "small", "scale": 2.0}}}},
		{"placement of missing component is dropped",
			`{"modes":{"idle":{"center":[{"id":"t.missing"},{"id":"t.card"}]}}}`,
			[]extensions.ExtRef{{ID: "t.card"}}},
		{"placement of a non-component extension is dropped",
			`{"modes":{"idle":{"center":[{"id":"t.bg"},{"id":"t.layout"},{"id":"t.card"}]}}}`,
			[]extensions.ExtRef{{ID: "t.card"}}},
		{"placement order kept",
			`{"modes":{"idle":{"center":[{"id":"t.plain"},{"id":"t.card"},{"id":"t.plain"}]}}}`,
			[]extensions.ExtRef{{ID: "t.plain"}, {ID: "t.card"}, {ID: "t.plain"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizedLayout(t, tt.layout)["idle"]["center"]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("placements = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolveKeepsModesAndRegions(t *testing.T) {
	layout := `{"modes":{"idle":{"top-left":[{"id":"t.card"}],"bottom":[{"id":"t.gone"}]},"attention":{"center":[{"id":"t.plain"}]}}}`

	got := sanitizedLayout(t, layout)

	if len(got) != 2 || len(got["idle"]) != 2 || len(got["attention"]) != 1 {
		t.Fatalf("modes = %+v, want idle with two regions and attention with one", got)
	}
	if bottom := got["idle"]["bottom"]; bottom == nil || len(bottom) != 0 {
		t.Errorf("emptied region = %#v, want empty non-nil slice", bottom)
	}
}

func TestResolveViewJSONHasNoNullCollections(t *testing.T) {
	layout := `{"modes":{"idle":{"center":[{"id":"t.gone"}]}}}`
	tests := []struct {
		name string
		view View
	}{
		{"empty fallback view", Resolve(config.Default(), nil)},
		{"normal view", Resolve(config.Default(), installedWithout(t))},
		{"view with every placement dropped", Resolve(config.Default(), sanitizeInstalled(layout))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.view)
			if err != nil {
				t.Fatal(err)
			}
			var parsed struct {
				Components []json.RawMessage `json:"components"`
				Style      map[string]any    `json:"style"`
				Background struct {
					Options map[string]any `json:"options"`
				} `json:"background"`
			}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed.Components == nil || parsed.Style == nil || parsed.Background.Options == nil {
				t.Errorf("null collection in %s", data)
			}
			for _, component := range tt.view.Components {
				if component.Options == nil {
					t.Errorf("component %s has null options", component.ID)
				}
			}
		})
	}
}

func TestResolveMaxFPSRange(t *testing.T) {
	tests := []struct {
		name         string
		maxFPS, want int
	}{
		{"negative becomes zero", -1, 0},
		{"above 240 becomes zero", 241, 0},
		{"zero kept", 0, 0},
		{"one kept", 1, 1},
		{"240 kept", 240, 240},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.MaxFPS = tt.maxFPS
			if got := Resolve(cfg, installedWithout(t)).MaxFPS; got != tt.want {
				t.Errorf("max fps = %d, want %d", got, tt.want)
			}
		})
	}
}
