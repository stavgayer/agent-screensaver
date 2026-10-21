package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/aerials"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
)

const aerialsManifest = `{
  "categories": [
    {"id": "CAT-CITY", "localizedNameKey": "AerialCategoryCities"},
    {"id": "CAT-SPACE", "localizedNameKey": "AerialCategorySpace"}
  ],
  "assets": [
    {"id": "C1", "categories": ["CAT-CITY"]},
    {"id": "C2", "categories": ["CAT-CITY"]},
    {"id": "C3", "categories": ["CAT-CITY"]},
    {"id": "S1", "categories": ["CAT-SPACE"]}
  ]
}`

// testExtensions are the manifests the aerials tests install: a theme per
// background, the two backgrounds and a layout.
var testExtensions = map[string]string{
	"builtin.default": `{"id":"builtin.default","type":"theme","api":1,
		"theme":{"background":{"id":"builtin.aerials"},"layout":"builtin.spotlight"}}`,
	"test.plain-theme": `{"id":"test.plain-theme","type":"theme","api":1,
		"theme":{"background":{"id":"builtin.matrix"},"layout":"builtin.spotlight"}}`,
	"builtin.spotlight": `{"id":"builtin.spotlight","type":"layout","api":1,
		"layout":{"modes":{"idle":{},"attention":{}}}}`,
	"builtin.aerials": `{"id":"builtin.aerials","type":"background","api":1,"entry":"index.js",
		"options":{"group":{"type":"enum","default":"cities","values":["cities","landscapes","space","underwater"]}}}`,
	"builtin.matrix": `{"id":"builtin.matrix","type":"background","api":1,"entry":"index.js"}`,
}

// fakeCloner writes a small file instead of cloning, and fails for the
// sources it is told to.
type fakeCloner struct {
	failFor map[string]bool
	calls   []string
}

func (f *fakeCloner) Clone(src, dst string) error {
	f.calls = append(f.calls, filepath.Base(src))
	if f.failFor[filepath.Base(src)] {
		return errors.New("clonefile: cross-device link")
	}
	return os.WriteFile(dst, []byte("clone of "+src), 0o644)
}

type aerialsFixture struct {
	daemon *Daemon
	paths  paths.Paths
	cloner *fakeCloner
	logs   *[]string
}

func newAerialsFixture(t *testing.T, downloaded ...string) aerialsFixture {
	t.Helper()
	home := t.TempDir()
	p := paths.ForHome(home)
	p.Aerials = aerials.Locations{
		Manifest: filepath.Join(home, "wallpaper", "entries.json"),
		Catalog:  filepath.Join(home, "no-catalog.json"),
		Videos:   filepath.Join(home, "wallpaper", "videos"),
	}
	mustWrite(t, p.Aerials.Manifest, aerialsManifest)
	for _, id := range downloaded {
		mustWrite(t, filepath.Join(p.Aerials.Videos, id+".mov"), "video "+id)
	}
	for id, manifest := range testExtensions {
		mustWrite(t, filepath.Join(p.ExtensionsDir, id, "extension.json"), manifest)
		mustWrite(t, filepath.Join(p.ExtensionsDir, id, "index.js"), "// entry")
	}
	logs := []string{}
	cloner := &fakeCloner{failFor: map[string]bool{}}
	d := New(p)
	d.Cloner = cloner
	d.Logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	return aerialsFixture{daemon: d, paths: p, cloner: cloner, logs: &logs}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// saveConfig writes the config and moves its mtime forward, so the daemon
// sees a change even within the file system's time resolution.
func saveConfig(t *testing.T, p paths.Paths, theme, group string, step int) {
	t.Helper()
	cfg := config.Default()
	cfg.Theme = theme
	cfg.Options = map[string]map[string]any{"builtin.aerials": {"group": group}}
	if err := cfg.Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Duration(step+1) * time.Hour)
	if err := os.Chtimes(p.ConfigFile, later, later); err != nil {
		t.Fatal(err)
	}
}

func cloneNames(t *testing.T, p paths.Paths) []string {
	t.Helper()
	entries, err := os.ReadDir(p.AerialClonesDir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}
	}
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names
}

func backgroundOf(t *testing.T, p paths.Paths) (string, []string) {
	t.Helper()
	background := readStateFile(t, p).View.Background
	files := []string{}
	if list, ok := background.Options["files"].([]any); ok {
		for _, file := range list {
			files = append(files, filepath.Base(file.(string)))
		}
	}
	return background.ID, files
}

func TestStepSyncsAerialClones(t *testing.T) {
	tests := []struct {
		name           string
		downloaded     []string
		theme, group   string
		failFor        []string
		existingClones []string
		wantClones     []string
		wantBackground string
	}{
		{"clones the downloaded videos of the group", []string{"C1", "C2", "S1"}, "builtin.default", "cities", nil, nil,
			[]string{"C1.mov", "C2.mov"}, "builtin.aerials"},
		{"removes clones that are no longer selected", []string{"C1", "S1"}, "builtin.default", "space", nil, []string{"C1.mov"},
			[]string{"S1.mov"}, "builtin.aerials"},
		{"removes clones whose source vanished", []string{"C1"}, "builtin.default", "cities", nil, []string{"C1.mov", "C2.mov"},
			[]string{"C1.mov"}, "builtin.aerials"},
		{"skips a video that cannot be cloned", []string{"C1", "C2"}, "builtin.default", "cities", []string{"C1.mov"}, nil,
			[]string{"C2.mov"}, "builtin.aerials"},
		{"nothing downloaded shows Matrix", nil, "builtin.default", "cities", nil, nil,
			[]string{}, "builtin.matrix"},
		{"every clone failing shows Matrix", []string{"C1"}, "builtin.default", "cities", []string{"C1.mov"}, nil,
			[]string{}, "builtin.matrix"},
		{"another background removes every clone", []string{"C1"}, "test.plain-theme", "cities", nil, []string{"C1.mov"},
			[]string{}, "builtin.matrix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newAerialsFixture(t, tt.downloaded...)
			saveConfig(t, fixture.paths, tt.theme, tt.group, 0)
			for _, name := range tt.failFor {
				fixture.cloner.failFor[name] = true
			}
			for _, name := range tt.existingClones {
				mustWrite(t, filepath.Join(fixture.paths.AerialClonesDir, name), "old clone")
			}

			fixture.daemon.Step(1_000, false)

			if got := cloneNames(t, fixture.paths); !reflect.DeepEqual(got, tt.wantClones) {
				t.Errorf("clones = %v, want %v", got, tt.wantClones)
			}
			id, files := backgroundOf(t, fixture.paths)
			wantFiles := tt.wantClones
			if id != aerials.ExtensionID {
				wantFiles = []string{}
			}
			if id != tt.wantBackground || !reflect.DeepEqual(files, wantFiles) {
				t.Errorf("background = %s with files %v, want %s with %v", id, files, tt.wantBackground, wantFiles)
			}
			if got := len(*fixture.logs); got != len(tt.failFor) {
				t.Errorf("logged %d lines %q, want one per failed clone", got, *fixture.logs)
			}
		})
	}
}

func TestAerialCloneFailureLogsTheVideo(t *testing.T) {
	fixture := newAerialsFixture(t, "C1")
	saveConfig(t, fixture.paths, "builtin.default", "cities", 0)
	fixture.cloner.failFor["C1.mov"] = true

	fixture.daemon.Step(1_000, false)

	if len(*fixture.logs) != 1 || !strings.Contains((*fixture.logs)[0], "C1.mov") {
		t.Errorf("logs = %q, want one line naming C1.mov", *fixture.logs)
	}
}

func TestAerialClonesSyncOnlyOnResolveOrEveryTenMinutes(t *testing.T) {
	fixture := newAerialsFixture(t, "C1")
	saveConfig(t, fixture.paths, "builtin.default", "cities", 0)
	fixture.daemon.Step(1_000, false)
	mustWrite(t, filepath.Join(fixture.paths.Aerials.Videos, "C2.mov"), "video C2")
	if err := os.Remove(filepath.Join(fixture.paths.Aerials.Videos, "C1.mov")); err != nil {
		t.Fatal(err)
	}

	fixture.daemon.Step(1_000+9*60_000, false)
	if got := cloneNames(t, fixture.paths); !reflect.DeepEqual(got, []string{"C1.mov"}) {
		t.Fatalf("before ten minutes: clones = %v, want the first sync kept", got)
	}

	result := fixture.daemon.Step(1_000+10*60_000, false)
	if got := cloneNames(t, fixture.paths); !reflect.DeepEqual(got, []string{"C2.mov"}) {
		t.Errorf("after ten minutes: clones = %v, want [C2.mov]", got)
	}
	if _, files := backgroundOf(t, fixture.paths); !result.StateWritten || !reflect.DeepEqual(files, []string{"C2.mov"}) {
		t.Errorf("state written %v with files %v, want the new clone", result.StateWritten, files)
	}

	saveConfig(t, fixture.paths, "builtin.default", "space", 1)
	fixture.daemon.Step(1_000+10*60_000+500, false)
	if got := cloneNames(t, fixture.paths); !reflect.DeepEqual(got, []string{}) {
		t.Errorf("after a config change: clones = %v, want none (no space video)", got)
	}
}

func TestAerialSyncDoesNotRecloneExistingClones(t *testing.T) {
	fixture := newAerialsFixture(t, "C1")
	saveConfig(t, fixture.paths, "builtin.default", "cities", 0)
	fixture.daemon.Step(1_000, false)

	fixture.daemon.Step(1_000+10*60_000, false)

	if !reflect.DeepEqual(fixture.cloner.calls, []string{"C1.mov"}) {
		t.Errorf("clone calls = %v, want one", fixture.cloner.calls)
	}
}
