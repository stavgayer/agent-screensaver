package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agent-screensaver/internal/extensions"
)

const (
	packTheme = `{"id":"acme.dark","name":"Dark","version":"1.0.0","type":"theme","api":1,
"theme":{"background":{"id":"acme.rain"},"layout":"acme.layout","style":{}}}`
	packLayout = `{"id":"acme.layout","name":"Layout","version":"1.0.0","type":"layout","api":1,
"layout":{"modes":{"idle":{"center":[{"id":"acme.clock"}]},"attention":{"bottom":[{"id":"builtin.cards"}]}}}}`
	packClock = `{"id":"acme.clock","name":"Clock","version":"1.0.0","type":"component","api":1,"entry":"main.js"}`
	// lightTheme shares the pack's background.
	lightTheme = `{"id":"acme.light","name":"Light","version":"1.0.0","type":"theme","api":1,
"theme":{"background":{"id":"acme.rain"},"layout":"builtin.layout","style":{}}}`
)

// writePack makes a pack folder with a theme, its layout, background and
// widget. Every main.js is executable, to see that install narrows it.
func writePack(t *testing.T) string {
	t.Helper()
	pack := t.TempDir()
	for name, manifest := range map[string]string{"theme": packTheme, "layout": packLayout, "rain": backgroundManifest, "clock": packClock} {
		dir := filepath.Join(pack, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, "extension.json"), manifest)
		if err := os.WriteFile(filepath.Join(dir, "main.js"), []byte("//"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return pack
}

func installPack(t *testing.T, service Service) {
	t.Helper()
	if _, err := service.InstallPack(writePack(t)); err != nil {
		t.Fatal(err)
	}
}

func TestInstallPackInstallsEveryPart(t *testing.T) {
	service := newService(t)
	pack := writePack(t)
	if isPack, err := service.IsPack(pack); err != nil || !isPack {
		t.Fatalf("IsPack = %v, %v; want true", isPack, err)
	}

	manifests, err := service.InstallPack(pack)

	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, manifest := range manifests {
		ids = append(ids, manifest.ID)
	}
	if want := []string{"acme.clock", "acme.layout", "acme.rain", "acme.dark"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("installed %v, want %v", ids, want)
	}
	info, err := os.Stat(filepath.Join(service.Paths.ExtensionsDir, "acme.clock", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("main.js mode = %v, want 0644 like any other install", info.Mode().Perm())
	}
}

func TestRemoveTheme(t *testing.T) {
	tests := []struct {
		name        string
		extra       []string // manifests installed after the pack
		active      bool
		background  string
		remove      string
		wantRemoved []string
		wantKept    []extensions.KeptPart
		wantTheme   string
	}{
		{"every part", nil, false, "", "acme.dark",
			[]string{"acme.dark", "acme.layout", "acme.rain", "acme.clock"}, nil, "builtin.default"},
		{"a shared part stays", []string{lightTheme}, false, "", "acme.dark",
			[]string{"acme.dark", "acme.layout", "acme.clock"}, []extensions.KeptPart{{ID: "acme.rain", UsedBy: "acme.light"}}, "builtin.default"},
		{"the active theme goes back to the default", nil, true, "", "acme.dark",
			[]string{"acme.dark", "acme.layout", "acme.rain", "acme.clock"}, nil, "builtin.default"},
		{"another active theme stays", []string{lightTheme}, false, "", "acme.dark",
			[]string{"acme.dark", "acme.layout", "acme.clock"}, []extensions.KeptPart{{ID: "acme.rain", UsedBy: "acme.light"}}, "acme.light"},
		{"a removed background stops being the user's", nil, false, "acme.rain", "acme.dark",
			[]string{"acme.dark", "acme.layout", "acme.rain", "acme.clock"}, nil, "builtin.default"},
		{"not a theme removes only it", nil, false, "", "acme.clock", []string{"acme.clock"}, nil, "builtin.default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newService(t)
			installPack(t, service)
			for _, manifest := range tt.extra {
				if _, err := service.InstallExtension(writeExtensionFolder(t, manifest)); err != nil {
					t.Fatal(err)
				}
			}
			theme := tt.wantTheme
			if tt.active {
				theme = "acme.dark"
			}
			if err := service.SetTheme(theme); err != nil && theme != "builtin.default" {
				t.Fatal(err)
			}
			if tt.background != "" {
				if err := service.SetBackground(tt.background); err != nil {
					t.Fatal(err)
				}
			}

			removal, err := service.RemoveTheme(tt.remove)

			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(removal.Removed, tt.wantRemoved) || !reflect.DeepEqual(removal.Kept, tt.wantKept) {
				t.Errorf("removal = %+v, want removed %v kept %v", removal, tt.wantRemoved, tt.wantKept)
			}
			if removal.ThemeReset != tt.active {
				t.Errorf("ThemeReset = %v, want %v", removal.ThemeReset, tt.active)
			}
			for _, id := range tt.wantRemoved {
				if _, err := os.Stat(filepath.Join(service.Paths.ExtensionsDir, id)); !os.IsNotExist(err) {
					t.Errorf("%s is still installed (err %v)", id, err)
				}
			}
			cfg := loadConfig(service)
			if cfg.Theme != tt.wantTheme || cfg.Background != "" {
				t.Errorf("config theme %q background %q, want %q and none", cfg.Theme, cfg.Background, tt.wantTheme)
			}
		})
	}
}

func TestRemoveThemeKeepsTheErrorsOfRemove(t *testing.T) {
	service := newService(t)
	for id, want := range map[string]string{"builtin.default": "reserved", "acme.none": "no such file"} {
		if _, err := service.RemoveTheme(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("RemoveTheme(%s) error = %v, want %q", id, err, want)
		}
	}
}
