package view

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
)

const repoExtensions = "../../extensions"

// shippedBuiltins copies the real built-in manifests into a temp dir and
// stubs each entry file, because the bundler writes the entries only at
// build time. The tests then scan exactly what the repo ships.
func shippedBuiltins(t *testing.T) []extensions.Installed {
	t.Helper()
	folders, err := os.ReadDir(repoExtensions)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, folder := range folders {
		data, err := os.ReadFile(filepath.Join(repoExtensions, folder.Name(), extensions.ManifestFile))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, folder.Name())
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, extensions.ManifestFile), data, 0o644); err != nil {
			t.Fatal(err)
		}
		var manifest extensions.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Entry != "" {
			if err := os.WriteFile(filepath.Join(target, manifest.Entry), []byte("// entry"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return extensions.Scan(dir)
}

func TestShippedThemes(t *testing.T) {
	installed := shippedBuiltins(t)
	tests := []struct {
		theme          string
		wantBackground string
		wantLayout     string
	}{
		{"builtin.default", "builtin.matrix", "builtin.spotlight"},
		{"builtin.calm", "builtin.aerials", "builtin.spotlight"},
	}
	for _, tt := range tests {
		t.Run(tt.theme, func(t *testing.T) {
			var theme *extensions.Manifest
			for _, ext := range installed {
				if ext.Manifest.ID == tt.theme && ext.Manifest.Type == extensions.TypeTheme {
					theme = &ext.Manifest
				}
			}
			if theme == nil || theme.Theme == nil {
				t.Fatalf("%s is not a shipped, valid theme", tt.theme)
			}
			if theme.Theme.Background.ID != tt.wantBackground || theme.Theme.Layout != tt.wantLayout {
				t.Errorf("%s uses background %s and layout %s, want %s and %s", tt.theme,
					theme.Theme.Background.ID, theme.Theme.Layout, tt.wantBackground, tt.wantLayout)
			}
		})
	}
}

func TestCalmThemeFallsBackToMatrixWithoutAerials(t *testing.T) {
	installed := shippedBuiltins(t)
	cfg := config.Default()
	cfg.Theme = "builtin.calm"
	tests := []struct {
		name   string
		clones []string
		wantID string
	}{
		{"downloaded aerials play", []string{"/d/media/aerials/A.mov"}, "builtin.aerials"},
		{"no aerials shows Matrix", nil, "builtin.matrix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(cfg, installed, tt.clones)
			if got.Theme != "builtin.calm" || got.Background.ID != tt.wantID {
				t.Errorf("theme %s background %s, want builtin.calm with %s", got.Theme, got.Background.ID, tt.wantID)
			}
			if got.Style["--accent"] == "#00e07a" {
				t.Errorf("Calm kept the Matrix accent %v", got.Style["--accent"])
			}
		})
	}
}
