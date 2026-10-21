package settings

import (
	"os"
	"path/filepath"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
)

const backgroundManifest = `{
  "id": "acme.rain", "name": "Rain", "version": "1.0.0", "type": "background", "api": 1,
  "entry": "main.js",
  "options": {
    "speed":  {"type": "number",  "default": 1, "min": 0.1, "max": 5},
    "glow":   {"type": "boolean", "default": false},
    "color":  {"type": "color",   "default": "#00ff66"},
    "mode":   {"type": "enum",    "default": "soft", "values": ["soft", "hard"]},
    "title":  {"type": "string",  "default": "hi"},
    "image":  {"type": "file",    "default": ""}
  }
}`

const themeManifest = `{
  "id": "acme.dark", "name": "Dark", "version": "1.0.0", "type": "theme", "api": 1,
  "theme": {"background": {"id": "acme.rain"}, "layout": "builtin.layout", "style": {}}
}`

// newService returns a Service rooted in a temp home.
func newService(t *testing.T) Service {
	t.Helper()
	return Service{Paths: paths.ForHome(t.TempDir())}
}

// writeExtensionFolder creates a folder holding a manifest and an entry file.
func writeExtensionFolder(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range map[string]string{"extension.json": manifest, "main.js": "// entry"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func installFixtures(t *testing.T, service Service) {
	t.Helper()
	for _, manifest := range []string{backgroundManifest, themeManifest} {
		if _, err := service.InstallExtension(writeExtensionFolder(t, manifest)); err != nil {
			t.Fatal(err)
		}
	}
}

func loadConfig(service Service) config.Config {
	return config.Load(service.Paths.ConfigFile)
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSetTheme(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"installed theme", "acme.dark", false},
		{"unknown id", "acme.missing", true},
		{"installed but not a theme", "acme.rain", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.SetTheme(tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetTheme(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
	if got := loadConfig(service).Theme; got != "acme.dark" {
		t.Errorf("theme = %q, want the last valid choice acme.dark", got)
	}
}

func TestSetThemeUnknownFails(t *testing.T) {
	service := newService(t)
	if err := service.SetTheme("nobody.nothing"); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(service.Paths.ConfigFile); !os.IsNotExist(err) {
		t.Errorf("a failed SetTheme must not write the config, stat error = %v", err)
	}
}

func TestSetOptionParsesByKind(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	tests := []struct {
		key  string
		raw  string
		want any
	}{
		{"speed", "2.5", 2.5},
		{"glow", "on", true},
		{"glow", "No", false},
		{"glow", "true", true},
		{"color", "#AABBCC", "#AABBCC"},
		{"mode", "hard", "hard"},
		{"title", "any text", "any text"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.raw, func(t *testing.T) {
			if err := service.SetOption("acme.rain", tt.key, tt.raw); err != nil {
				t.Fatal(err)
			}
			got := loadConfig(service).Options["acme.rain"][tt.key]
			if got != tt.want {
				t.Errorf("stored %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestSetOptionValidatesRange(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	tests := []struct {
		name, extID, key, raw string
	}{
		{"number above max", "acme.rain", "speed", "9"},
		{"number below min", "acme.rain", "speed", "0"},
		{"not a number", "acme.rain", "speed", "fast"},
		{"non finite number", "acme.rain", "speed", "NaN"},
		{"bad boolean", "acme.rain", "glow", "maybe"},
		{"bad color", "acme.rain", "color", "green"},
		{"enum outside values", "acme.rain", "mode", "medium"},
		{"undeclared key", "acme.rain", "nope", "1"},
		{"extension not installed", "acme.missing", "speed", "1"},
		{"missing file", "acme.rain", "image", "/no/such/file.png"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := service.SetOption(tt.extID, tt.key, tt.raw); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
	if _, err := os.Stat(service.Paths.ConfigFile); !os.IsNotExist(err) {
		t.Errorf("failed SetOption calls must not write the config, stat error = %v", err)
	}
}

func TestFileOptionCopiesIntoMedia(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	source := writeFile(t, filepath.Join(t.TempDir(), "photo.png"), "first")

	if err := service.SetOption("acme.rain", "image", source); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(service.Paths.MediaDir, "acme.rain", "photo.png")
	if got := loadConfig(service).Options["acme.rain"]["image"]; got != want {
		t.Fatalf("stored %v, want the copied path %s", got, want)
	}
	assertFileContent(t, want, "first")

	writeFile(t, source, "second")
	if err := service.SetOption("acme.rain", "image", source); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, want, "second")
}

func TestFileOptionRejectsDirectory(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	if err := service.SetOption("acme.rain", "image", t.TempDir()); err == nil {
		t.Fatal("expected an error for a directory")
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("%s holds %q, want %q", path, data, want)
	}
}

func TestResetOption(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	for key, raw := range map[string]string{"speed": "2", "glow": "on"} {
		if err := service.SetOption("acme.rain", key, raw); err != nil {
			t.Fatal(err)
		}
	}

	if err := service.ResetOption("acme.rain", "speed"); err != nil {
		t.Fatal(err)
	}
	options := loadConfig(service).Options["acme.rain"]
	if _, has := options["speed"]; has || options["glow"] != true {
		t.Errorf("after one reset options = %v, want only glow", options)
	}
	if err := service.ResetOption("acme.rain", "nope"); err == nil {
		t.Error("resetting an undeclared key must fail")
	}

	if err := service.ResetOption("acme.rain", ""); err != nil {
		t.Fatal(err)
	}
	if _, has := loadConfig(service).Options["acme.rain"]; has {
		t.Error("an empty key must reset every option of the extension")
	}
	if err := service.ResetOption("acme.missing", ""); err == nil {
		t.Error("resetting a missing extension must fail")
	}
}

func TestInstallListRemoveExtension(t *testing.T) {
	service := newService(t)
	manifest, err := service.InstallExtension(writeExtensionFolder(t, backgroundManifest))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "acme.rain" {
		t.Fatalf("installed id = %q", manifest.ID)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Extensions) != 1 || snapshot.Extensions[0].Manifest.ID != "acme.rain" {
		t.Fatalf("extensions after install = %+v", snapshot.Extensions)
	}

	if err := service.RemoveExtension("acme.rain"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Extensions) != 0 {
		t.Errorf("extensions after remove = %+v", snapshot.Extensions)
	}
	if err := service.RemoveExtension("builtin.default"); err == nil {
		t.Error("removing a built-in must fail")
	}
}

func TestPrivacyTogglePersists(t *testing.T) {
	service := newService(t)
	for _, on := range []bool{true, false, true} {
		if err := service.SetPrivacy(on); err != nil {
			t.Fatal(err)
		}
		if got := loadConfig(service).Privacy; got != on {
			t.Fatalf("privacy = %v, want %v", got, on)
		}
	}
}

func TestSetSound(t *testing.T) {
	service := newService(t)
	chime := writeFile(t, filepath.Join(t.TempDir(), "chime.aiff"), "sound")
	defaultFile := config.Default().SoundFile

	if err := service.SetSound(false, ""); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(service); got.Sound || got.SoundFile != defaultFile {
		t.Errorf("sound off kept file? got sound=%v file=%q", got.Sound, got.SoundFile)
	}
	if err := service.SetSound(true, chime); err != nil {
		t.Fatal(err)
	}
	if got := loadConfig(service); !got.Sound || got.SoundFile != chime {
		t.Errorf("got sound=%v file=%q, want on with %q", got.Sound, got.SoundFile, chime)
	}
	if err := service.SetSound(true, "/no/such/sound.aiff"); err == nil {
		t.Error("a missing sound file must fail")
	}
	if got := loadConfig(service).SoundFile; got != chime {
		t.Errorf("a failed SetSound changed the file to %q", got)
	}
}

func TestSetMaxFPSRange(t *testing.T) {
	service := newService(t)
	tests := []struct {
		fps     int
		wantErr bool
	}{
		{0, false}, {1, false}, {60, false}, {240, false},
		{-1, true}, {241, true},
	}
	for _, tt := range tests {
		err := service.SetMaxFPS(tt.fps)
		if (err != nil) != tt.wantErr {
			t.Errorf("SetMaxFPS(%d) error = %v, wantErr %v", tt.fps, err, tt.wantErr)
		}
	}
	if got := loadConfig(service).MaxFPS; got != 240 {
		t.Errorf("max fps = %d, want the last valid value 240", got)
	}
}

func TestSnapshotShowsSchemasAndValues(t *testing.T) {
	service := newService(t)
	installFixtures(t, service)
	if err := service.SetOption("acme.rain", "speed", "3"); err != nil {
		t.Fatal(err)
	}
	// An extension of the builtin family, written straight into the folder.
	builtinFolder := filepath.Join(service.Paths.ExtensionsDir, "builtin.glow")
	if err := os.MkdirAll(builtinFolder, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(builtinFolder, "main.js"), "//")
	writeFile(t, filepath.Join(builtinFolder, "extension.json"),
		`{"id":"builtin.glow","name":"Glow","version":"1.0.0","type":"component","api":1,"entry":"main.js"}`)

	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	byID := map[string]ExtensionInfo{}
	for _, info := range snapshot.Extensions {
		ids = append(ids, info.Manifest.ID)
		byID[info.Manifest.ID] = info
	}
	wantIDs := []string{"acme.dark", "acme.rain", "builtin.glow"}
	if len(ids) != len(wantIDs) {
		t.Fatalf("ids = %v, want %v", ids, wantIDs)
	}
	for i := range wantIDs {
		if ids[i] != wantIDs[i] {
			t.Fatalf("ids = %v, want sorted %v", ids, wantIDs)
		}
	}

	rain := byID["acme.rain"]
	if rain.Builtin || !byID["builtin.glow"].Builtin {
		t.Error("Builtin flag is wrong")
	}
	if len(rain.Manifest.Options) != 6 {
		t.Errorf("option schemas = %d, want 6", len(rain.Manifest.Options))
	}
	if rain.Values["speed"] != 3.0 || rain.Values["mode"] != "soft" || rain.Values["glow"] != false {
		t.Errorf("merged values = %v, want speed overlaid and defaults kept", rain.Values)
	}
	if byID["builtin.glow"].Values == nil {
		t.Error("Values must be an empty map, not nil, for JSON consumers")
	}
	if snapshot.Config.Theme != config.Default().Theme {
		t.Errorf("config theme = %q", snapshot.Config.Theme)
	}
}
