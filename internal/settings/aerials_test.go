package settings

import (
	"os"
	"path/filepath"
	"testing"

	"agent-screensaver/internal/aerials"
)

const aerialsBackground = `{"id":"builtin.aerials","type":"background","api":1,"entry":"index.js",
"options":{"group":{"type":"enum","default":"cities","values":["cities","landscapes","space","underwater"]}}}`

const aerialsCatalog = `{
  "categories": [{"id": "CAT-CITY", "localizedNameKey": "AerialCategoryCities"},
                 {"id": "CAT-SPACE", "localizedNameKey": "AerialCategorySpace"}],
  "assets": [{"id": "C1", "categories": ["CAT-CITY"]}, {"id": "S1", "categories": ["CAT-SPACE"]}]
}`

// aerialsService has builtin.aerials installed and one downloaded city video.
func aerialsService(t *testing.T) Service {
	t.Helper()
	service := newService(t)
	root := t.TempDir()
	service.Paths.Aerials = aerials.Locations{
		Manifest: filepath.Join(root, "entries.json"),
		Catalog:  filepath.Join(root, "no-catalog.json"),
		Videos:   filepath.Join(root, "videos"),
	}
	files := map[string]string{
		service.Paths.Aerials.Manifest:                                                  aerialsCatalog,
		filepath.Join(service.Paths.Aerials.Videos, "C1.mov"):                           "",
		filepath.Join(service.Paths.ExtensionsDir, "builtin.aerials", "extension.json"): aerialsBackground,
		filepath.Join(service.Paths.ExtensionsDir, "builtin.aerials", "index.js"):       "// entry",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, content)
	}
	return service
}

func TestNoticeForAerialsWithoutDownloads(t *testing.T) {
	const hint = "Pick it once in System Settings → Screen Saver, and macOS will download the videos."
	tests := []struct {
		name  string
		group string // "" keeps the manifest default (cities)
		extID string
		want  string
	}{
		{"default group has a download", "", "builtin.aerials", ""},
		{"group with a download", "space", "builtin.aerials", ""},
		{"group with nothing downloaded", "landscapes", "builtin.aerials", hint},
		{"another extension never gets the hint", "landscapes", "acme.rain", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := aerialsService(t)
			if tt.group == "space" {
				writeFile(t, filepath.Join(service.Paths.Aerials.Videos, "S1.mov"), "")
			}
			if tt.group != "" {
				if err := service.SetOption("builtin.aerials", "group", tt.group); err != nil {
					t.Fatal(err)
				}
			}
			if got := service.Notice(tt.extID); got != tt.want {
				t.Errorf("Notice(%s) = %q, want %q", tt.extID, got, tt.want)
			}
		})
	}
}

func TestNoticeWhenAerialsNotInstalled(t *testing.T) {
	if got := newService(t).Notice("builtin.aerials"); got != "" {
		t.Errorf("Notice = %q, want empty", got)
	}
}
