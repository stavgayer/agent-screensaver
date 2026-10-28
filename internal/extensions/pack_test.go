package extensions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func themeJSON(t *testing.T, id, background, layout string) string {
	t.Helper()
	data, err := json.Marshal(Manifest{ID: id, Name: "Theme", Version: "1.0.0", Type: TypeTheme, API: APIVersion,
		Theme: &ThemeSpec{Background: ExtRef{ID: background}, Layout: layout}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// packFolder writes one subfolder per part, each with its manifest and a
// main.js, and returns the pack folder.
func packFolder(t *testing.T, parts map[string]string) string {
	t.Helper()
	pack := t.TempDir()
	for name, manifest := range parts {
		dir := filepath.Join(pack, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, ManifestFile), manifest)
		writeFile(t, filepath.Join(dir, "main.js"), "// "+name)
	}
	return pack
}

func twoParts(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"rain":  manifestJSON(t, "acme.rain", "1.0.0"),
		"theme": themeJSON(t, "acme.dark", "acme.rain", "builtin.layout"),
	}
}

// packZip zips the parts, each in its folder under prefix ("" or "pack/").
func packZip(t *testing.T, prefix string, parts map[string]string) string {
	t.Helper()
	var entries []zipEntry
	for name, manifest := range parts {
		entries = append(entries,
			zipEntry{name: prefix + name + "/extension.json", content: manifest},
			zipEntry{name: prefix + name + "/main.js", content: "// " + name})
	}
	entries = append(entries, zipEntry{name: "__MACOSX/._x", content: "junk"}, zipEntry{name: prefix + ".DS_Store", content: "junk"})
	return writeZip(t, entries)
}

func manifestIDs(manifests []Manifest) []string {
	ids := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		ids = append(ids, manifest.ID)
	}
	return ids
}

func TestIsPack(t *testing.T) {
	tests := []struct {
		name string
		src  func(t *testing.T) string
		want bool
	}{
		{"folder of parts", func(t *testing.T) string { return packFolder(t, twoParts(t)) }, true},
		{"one extension folder", func(t *testing.T) string { return validDir }, false},
		{"folder with one part", func(t *testing.T) string {
			return packFolder(t, map[string]string{"rain": manifestJSON(t, "acme.rain", "1.0.0")})
		}, false},
		{"zip of parts at the top", func(t *testing.T) string { return packZip(t, "", twoParts(t)) }, true},
		{"zip of parts in one top folder", func(t *testing.T) string { return packZip(t, "pack/", twoParts(t)) }, true},
		{"zip of one extension", func(t *testing.T) string { return validZip(t, "acme.zipped", "1.0.0") }, false},
		{"Finder zip of one extension", finderZip, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsPack(tt.src(t))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("IsPack() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInstallPackInstallsEveryPart(t *testing.T) {
	sources := map[string]func(t *testing.T) string{
		"folder":              func(t *testing.T) string { return packFolder(t, twoParts(t)) },
		"zip":                 func(t *testing.T) string { return packZip(t, "", twoParts(t)) },
		"zip with top folder": func(t *testing.T) string { return packZip(t, "pack/", twoParts(t)) },
	}
	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			extDir := filepath.Join(t.TempDir(), "extensions")
			manifests, err := InstallPack(src(t), extDir, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := manifestIDs(manifests), []string{"acme.rain", "acme.dark"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("installed %v, want %v (in the order of the part folders)", got, want)
			}
			assertNoLeftovers(t, extDir, "acme.dark", "acme.rain")
			if _, err := os.Stat(filepath.Join(extDir, "acme.rain", "main.js")); err != nil {
				t.Fatalf("part files not installed: %v", err)
			}
			if len(Scan(extDir)) != 2 {
				t.Fatalf("Scan found %d extensions, want 2", len(Scan(extDir)))
			}
		})
	}
}

func TestInstallPackInstallsNothingWhenOnePartFails(t *testing.T) {
	agent := func(t *testing.T) string {
		pack := packFolder(t, twoParts(t))
		copyTestFolder(t, agentFolder(t, "acme.bot", "acme", goodMark), filepath.Join(pack, "bot"))
		return pack
	}
	tests := []struct {
		name    string
		src     func(t *testing.T) string
		check   func(m Manifest, dir string) error
		wantErr []string
	}{
		{"invalid part", func(t *testing.T) string {
			parts := twoParts(t)
			parts["broken"] = `{"id":"acme.broken","type":"background","api":1}`
			return packFolder(t, parts)
		}, nil, []string{"broken", "entry"}},
		{"part with a built-in id", func(t *testing.T) string {
			parts := twoParts(t)
			parts["fake"] = manifestJSON(t, "builtin.fake", "1.0.0")
			return packFolder(t, parts)
		}, nil, []string{"fake", "reserved"}},
		{"check refuses a part", func(t *testing.T) string { return packFolder(t, twoParts(t)) },
			func(m Manifest, dir string) error {
				if m.ID == "acme.rain" {
					return errors.New("not today")
				}
				return nil
			}, []string{"rain", "not today"}},
		{"two parts with one id", func(t *testing.T) string {
			parts := twoParts(t)
			parts["rain-copy"] = manifestJSON(t, "acme.rain", "2.0.0")
			return packFolder(t, parts)
		}, nil, []string{"rain-copy", "acme.rain", "twice"}},
		{"agent part", agent, nil, []string{"bot", "agent", "alone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extDir := filepath.Join(t.TempDir(), "extensions")
			_, err := InstallPack(tt.src(t), extDir, tt.check)
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want it to contain %q", err, want)
				}
			}
			assertNoLeftovers(t, extDir)
		})
	}
}

func TestInstallPackReplacesInstalledParts(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	if _, err := InstallFrom(validZip(t, "acme.rain", "0.9.0"), extDir); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallPack(packFolder(t, twoParts(t)), extDir, nil); err != nil {
		t.Fatal(err)
	}
	for _, ext := range Scan(extDir) {
		if ext.Manifest.ID == "acme.rain" && ext.Manifest.Version != "1.0.0" {
			t.Fatalf("acme.rain is at %s, want 1.0.0", ext.Manifest.Version)
		}
	}
	assertNoLeftovers(t, extDir, "acme.dark", "acme.rain")
}

func TestInstallPackKeepsTheZipLimitsForTheWholePack(t *testing.T) {
	var entries []zipEntry
	for name, manifest := range twoParts(t) {
		entries = append(entries, zipEntry{name: name + "/extension.json", content: manifest}, zipEntry{name: name + "/main.js", content: "//"})
	}
	for index := 0; index < maxZipEntries; index++ {
		entries = append(entries, zipEntry{name: "rain/file" + itoa(index), content: "x"})
	}
	extDir := filepath.Join(t.TempDir(), "extensions")
	_, err := InstallPack(writeZip(t, entries), extDir, nil)
	assertError(t, err, "limit")
	assertNoLeftovers(t, extDir)
}

func TestInstallPackRefusesASingleExtension(t *testing.T) {
	_, err := InstallPack(validDir, filepath.Join(t.TempDir(), "extensions"), nil)
	assertError(t, err, "not a pack")
}

func installed(manifests ...Manifest) []Installed {
	list := make([]Installed, 0, len(manifests))
	for _, manifest := range manifests {
		list = append(list, Installed{Manifest: manifest})
	}
	return list
}

func theme(id, background, layout string) Manifest {
	return Manifest{ID: id, Type: TypeTheme, Theme: &ThemeSpec{Background: ExtRef{ID: background}, Layout: layout}}
}

func layoutOf(id string, widgets ...string) Manifest {
	var refs []string
	for _, widget := range widgets {
		refs = append(refs, `{"id":"`+widget+`"}`)
	}
	layout := `{"modes":{"idle":{"center":[` + strings.Join(refs, ",") + `]},"attention":{"bottom":[{"id":"builtin.cards"}]}}}`
	return Manifest{ID: id, Type: TypeLayout, Layout: json.RawMessage(layout)}
}

func part(id string, kind Type) Manifest { return Manifest{ID: id, Type: kind} }

func TestPlanThemeRemoval(t *testing.T) {
	tests := []struct {
		name       string
		installed  []Installed
		wantRemove []string
		wantKept   []KeptPart
	}{
		{"every part of the theme",
			installed(theme("acme.dark", "acme.rain", "acme.layout"), part("acme.rain", TypeBackground),
				layoutOf("acme.layout", "acme.clock", "acme.cards"), part("acme.clock", TypeComponent), part("acme.cards", TypeComponent)),
			[]string{"acme.dark", "acme.layout", "acme.rain", "acme.cards", "acme.clock"}, nil},
		{"built-ins and missing parts are left alone",
			installed(theme("acme.dark", "builtin.matrix", "acme.layout"), layoutOf("acme.layout", "builtin.clock", "acme.gone")),
			[]string{"acme.dark", "acme.layout"}, nil},
		{"a background another theme uses stays",
			installed(theme("acme.dark", "acme.rain", "builtin.layout"), theme("acme.light", "acme.rain", "builtin.layout"), part("acme.rain", TypeBackground)),
			[]string{"acme.dark"}, []KeptPart{{ID: "acme.rain", UsedBy: "acme.light"}}},
		{"a widget another layout uses stays",
			installed(theme("acme.dark", "builtin.matrix", "acme.layout"), layoutOf("acme.layout", "acme.clock", "acme.cards"),
				layoutOf("other.layout", "acme.clock"), part("acme.clock", TypeComponent), part("acme.cards", TypeComponent)),
			[]string{"acme.dark", "acme.layout", "acme.cards"}, []KeptPart{{ID: "acme.clock", UsedBy: "other.layout"}}},
		{"a layout another theme uses keeps its widgets",
			installed(theme("acme.dark", "builtin.matrix", "acme.layout"), theme("other.theme", "builtin.matrix", "acme.layout"),
				layoutOf("acme.layout", "acme.clock"), part("acme.clock", TypeComponent)),
			[]string{"acme.dark"}, []KeptPart{{ID: "acme.clock", UsedBy: "acme.layout"}, {ID: "acme.layout", UsedBy: "other.theme"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanThemeRemoval("acme.dark", tt.installed)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.Remove, tt.wantRemove) {
				t.Errorf("Remove = %v, want %v", plan.Remove, tt.wantRemove)
			}
			if !reflect.DeepEqual(plan.Kept, tt.wantKept) {
				t.Errorf("Kept = %v, want %v", plan.Kept, tt.wantKept)
			}
		})
	}
}

func TestPlanThemeRemovalOfAnotherTypeRemovesOnlyIt(t *testing.T) {
	plan, err := PlanThemeRemoval("acme.layout", installed(theme("acme.dark", "acme.rain", "acme.layout"), layoutOf("acme.layout", "acme.clock")))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Remove, []string{"acme.layout"}) || plan.Kept != nil {
		t.Fatalf("plan = %+v, want only acme.layout", plan)
	}
}

func TestPlanThemeRemovalOfAnUnknownIDFails(t *testing.T) {
	_, err := PlanThemeRemoval("acme.none", nil)
	assertError(t, err, "not installed")
}
