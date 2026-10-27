package extensions

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validDir = "testdata/valid-background"

func floatPointer(value float64) *float64 { return &value }

func baseBackground() Manifest {
	return Manifest{ID: "acme.rain", Name: "Rain", Version: "1.0.0", Type: TypeBackground, API: APIVersion, Entry: "main.js"}
}

func layoutManifest(layout string) Manifest {
	return Manifest{ID: "acme.layout", Name: "Layout", Version: "1.0.0", Type: TypeLayout, API: APIVersion, Layout: json.RawMessage(layout)}
}

const goodLayout = `{"modes":{"idle":{"center":[{"id":"builtin.clock"}]},"attention":{"bottom":[{"id":"builtin.cards"}]}}}`

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(m *Manifest)
		wantErr string
	}{
		{"valid background", func(m *Manifest) {}, ""},
		{"bad id", func(m *Manifest) { m.ID = "NoDots" }, "id"},
		{"unsupported api", func(m *Manifest) { m.API = 2 }, "api"},
		{"entry outside dir", func(m *Manifest) { m.Entry = "../x.js" }, "entry"},
		{"absolute entry", func(m *Manifest) { m.Entry = "/etc/passwd" }, "entry"},
		{"entry missing", func(m *Manifest) { m.Entry = "nope.js" }, "entry"},
		{"entry required", func(m *Manifest) { m.Entry = "" }, "entry"},
		{"unknown type", func(m *Manifest) { m.Type = "widget" }, "type"},
		{"theme without body", func(m *Manifest) { m.Type = TypeTheme; m.Entry = "" }, "theme"},
		{"valid layout", func(m *Manifest) { *m = layoutManifest(goodLayout) }, ""},
		{"unknown region", func(m *Manifest) {
			*m = layoutManifest(`{"modes":{"idle":{"middle":[]},"attention":{}}}`)
		}, "region"},
		{"unknown mode", func(m *Manifest) {
			*m = layoutManifest(`{"modes":{"idle":{},"attention":{},"sleep":{}}}`)
		}, "mode"},
		{"missing attention mode", func(m *Manifest) { *m = layoutManifest(`{"modes":{"idle":{}}}`) }, "attention"},
		{"layout required", func(m *Manifest) { *m = layoutManifest("") }, "layout"},
		{"option default out of range", func(m *Manifest) {
			m.Options = map[string]OptionSpec{"speed": {Kind: OptionNumber, Default: json.RawMessage(`9`), Max: floatPointer(5)}}
		}, "speed"},
		{"option default missing", func(m *Manifest) {
			m.Options = map[string]OptionSpec{"speed": {Kind: OptionNumber}}
		}, "default"},
		{"unknown option kind", func(m *Manifest) {
			m.Options = map[string]OptionSpec{"x": {Kind: "blob", Default: json.RawMessage(`1`)}}
		}, "type"},
		{"valid theme", func(m *Manifest) {
			m.Type = TypeTheme
			m.Entry = ""
			m.Theme = &ThemeSpec{Background: ExtRef{ID: "builtin.matrix"}, Layout: "builtin.layout-b"}
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := baseBackground()
			tt.mutate(&manifest)
			err := Validate(manifest, validDir)
			assertError(t, err, tt.wantErr)
		})
	}
}

func TestValidateOption(t *testing.T) {
	tests := []struct {
		name    string
		spec    OptionSpec
		value   any
		wantErr bool
	}{
		{"number in range", OptionSpec{Kind: OptionNumber, Min: floatPointer(1), Max: floatPointer(5)}, 3.0, false},
		{"number out of range", OptionSpec{Kind: OptionNumber, Min: floatPointer(1), Max: floatPointer(5)}, 9.0, true},
		{"number below min", OptionSpec{Kind: OptionNumber, Min: floatPointer(1)}, 0.5, true},
		{"number wrong type", OptionSpec{Kind: OptionNumber}, "3", true},
		{"enum member", OptionSpec{Kind: OptionEnum, Values: []string{"a", "b"}}, "a", false},
		{"enum not in values", OptionSpec{Kind: OptionEnum, Values: []string{"a", "b"}}, "c", true},
		{"color ok", OptionSpec{Kind: OptionColor}, "#00ff66", false},
		{"color must be #rrggbb", OptionSpec{Kind: OptionColor}, "green", true},
		{"color short form rejected", OptionSpec{Kind: OptionColor}, "#0f6", true},
		{"boolean ok", OptionSpec{Kind: OptionBoolean}, true, false},
		{"boolean wrong type", OptionSpec{Kind: OptionBoolean}, "true", true},
		{"string ok", OptionSpec{Kind: OptionString}, "hello", false},
		{"string wrong type", OptionSpec{Kind: OptionString}, 1.0, true},
		{"file is a string", OptionSpec{Kind: OptionFile}, "/Users/me/a.mp4", false},
		{"file wrong type", OptionSpec{Kind: OptionFile}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOption(tt.spec, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOption() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type zipEntry struct {
	name    string
	content string
	mode    os.FileMode
}

func writeZip(t *testing.T, entries []zipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ext.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		part, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(entry.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func manifestJSON(t *testing.T, id, version string) string {
	t.Helper()
	data, err := json.Marshal(Manifest{ID: id, Name: "Zipped", Version: version, Type: TypeBackground, API: APIVersion, Entry: "main.js"})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func validZip(t *testing.T, id, version string) string {
	t.Helper()
	return writeZip(t, []zipEntry{
		{name: "extension.json", content: manifestJSON(t, id, version)},
		{name: "main.js", content: "// " + version},
	})
}

func TestInstallFrom(t *testing.T) {
	tests := []struct {
		name    string
		source  func(t *testing.T) string
		wantErr string
		wantID  string
	}{
		{"install from folder", func(t *testing.T) string { return validDir }, "", "acme.rain"},
		{"install from zip", func(t *testing.T) string { return validZip(t, "acme.zipped", "1.0.0") }, "", "acme.zipped"},
		{"zip slip rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{
				{name: "extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
				{name: "../../evil", content: "x"},
			})
		}, "unsafe", ""},
		{"absolute zip path rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{{name: "/tmp/evil", content: "x"}})
		}, "unsafe", ""},
		{"backslash traversal rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{{name: `..\evil`, content: "x"}})
		}, "unsafe", ""},
		{"symlink in zip rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{
				{name: "extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
				{name: "main.js", content: "/etc/passwd", mode: os.ModeSymlink | 0o777},
			})
		}, "symlink", ""},
		{"builtin id rejected", func(t *testing.T) string { return "testdata/builtin-id" }, "builtin", ""},
		{"invalid manifest rejected", func(t *testing.T) string { return "testdata/invalid-id" }, "id", ""},
		{"zip without manifest rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{{name: "main.js", content: "x"}})
		}, "manifest", ""},
		{"zip with one top folder installs", func(t *testing.T) string { return finderZip(t) }, "", "acme.zipped"},
		{"zip with two top folders rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{
				{name: "one/extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
				{name: "two/main.js", content: "x"},
			})
		}, "2 top folders", ""},
		{"traversal inside the top folder rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{
				{name: "rain/extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
				{name: "rain/../../evil", content: "x"},
			})
		}, "unsafe", ""},
		{"symlink inside the top folder rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{
				{name: "rain/extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
				{name: "rain/main.js", content: "/etc/passwd", mode: os.ModeSymlink | 0o777},
			})
		}, "symlink", ""},
		{"too large inside the top folder rejected", func(t *testing.T) string {
			return writeZip(t, []zipEntry{
				{name: "rain/extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
				{name: "rain/big.bin", content: strings.Repeat("a", maxZipBytes+1)},
			})
		}, "large", ""},
		{"missing source", func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope") }, "no such file", ""},
		{"too many entries", func(t *testing.T) string {
			entries := make([]zipEntry, maxZipEntries+1)
			for index := range entries {
				entries[index] = zipEntry{name: "f" + itoa(index)}
			}
			return writeZip(t, entries)
		}, "entries", ""},
		{"too large uncompressed", func(t *testing.T) string {
			return writeZip(t, []zipEntry{{name: "big.bin", content: strings.Repeat("a", maxZipBytes+1)}})
		}, "large", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extDir := filepath.Join(t.TempDir(), "extensions")
			manifest, err := InstallFrom(tt.source(t), extDir)
			assertError(t, err, tt.wantErr)
			if tt.wantErr != "" {
				assertNoLeftovers(t, extDir)
				return
			}
			if manifest.ID != tt.wantID {
				t.Fatalf("manifest id = %q, want %q", manifest.ID, tt.wantID)
			}
			installed := Scan(extDir)
			if len(installed) != 1 || installed[0].Manifest.ID != tt.wantID {
				t.Fatalf("Scan() = %+v, want one extension %q", installed, tt.wantID)
			}
			assertNoLeftovers(t, extDir, tt.wantID)
		})
	}
}

// finderZip makes a zip like Finder's Compress on a folder: one top
// folder, plus the __MACOSX and .DS_Store entries macOS adds.
func finderZip(t *testing.T) string {
	t.Helper()
	return writeZip(t, []zipEntry{
		{name: "rain/"},
		{name: "rain/extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
		{name: "rain/main.js", content: "// 1.0.0"},
		{name: "rain/.DS_Store", content: "junk"},
		{name: "__MACOSX/"},
		{name: "__MACOSX/rain/._extension.json", content: "junk"},
		{name: ".DS_Store", content: "junk"},
	})
}

func TestInstallFromZipWithOneTopFolderUnwrapsIt(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	if _, err := InstallFrom(finderZip(t), extDir); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(extDir, "acme.zipped")
	for _, file := range []string{"extension.json", "main.js"} {
		if _, err := os.Stat(filepath.Join(installed, file)); err != nil {
			t.Errorf("%s not at the top of the extension: %v", file, err)
		}
	}
	for _, junk := range []string{"rain", "__MACOSX", ".DS_Store"} {
		if _, err := os.Stat(filepath.Join(installed, junk)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s was installed, want it left out (err %v)", junk, err)
		}
	}
}

func TestInstallFromZipSlipWritesNothingOutside(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "a", "b", "extensions")
	archive := writeZip(t, []zipEntry{
		{name: "extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
		{name: "../../evil", content: "x"},
	})
	if _, err := InstallFrom(archive, extDir); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := os.Stat(filepath.Join(root, "a", "evil")); !os.IsNotExist(err) {
		t.Fatalf("file escaped the extension folder: %v", err)
	}
}

func TestInstallFromReplacesSameID(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	first := writeZip(t, []zipEntry{
		{name: "extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
		{name: "main.js", content: "old"},
		{name: "old-only.txt", content: "stale"},
	})
	if _, err := InstallFrom(first, extDir); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFrom(validZip(t, "acme.zipped", "2.0.0"), extDir); err != nil {
		t.Fatal(err)
	}
	installed := Scan(extDir)
	if len(installed) != 1 || installed[0].Manifest.Version != "2.0.0" {
		t.Fatalf("Scan() = %+v, want version 2.0.0", installed)
	}
	if _, err := os.Stat(filepath.Join(installed[0].Dir, "old-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file from the old version survived: %v", err)
	}
	assertNoLeftovers(t, extDir, "acme.zipped")
}

func TestInstallFromFolderRejectsSymlink(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "extension.json"), manifestJSON(t, "acme.linked", "1.0.0"))
	if err := os.Symlink("/etc/passwd", filepath.Join(source, "main.js")); err != nil {
		t.Fatal(err)
	}
	extDir := filepath.Join(t.TempDir(), "extensions")
	_, err := InstallFrom(source, extDir)
	assertError(t, err, "symlink")
	assertNoLeftovers(t, extDir)
}

func TestScan(t *testing.T) {
	// build installs the valid extension, then lets setup change the folder.
	build := func(t *testing.T, setup func(extDir string)) string {
		extDir := filepath.Join(t.TempDir(), "extensions")
		if _, err := InstallFrom(validDir, extDir); err != nil {
			t.Fatal(err)
		}
		setup(extDir)
		return extDir
	}
	tests := []struct {
		name  string
		setup func(extDir string)
		want  []string
	}{
		{"scan skips invalid", func(extDir string) {
			if err := os.MkdirAll(filepath.Join(extDir, "acme.broken"), 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(extDir, "acme.broken", ManifestFile), "{not json")
			writeFile(t, filepath.Join(extDir, "notes.txt"), "not a folder")
		}, []string{"acme.rain"}},
		{"folder name differs from id skipped", func(extDir string) {
			copyTestFolder(t, validDir, filepath.Join(extDir, "other-name"))
		}, []string{"acme.rain"}},
		{"duplicate id only listed once", func(extDir string) {
			copyTestFolder(t, validDir, filepath.Join(extDir, "aaa.stale"))
		}, []string{"acme.rain"}},
		{"missing folder gives nothing", func(extDir string) {
			if err := os.RemoveAll(extDir); err != nil {
				t.Fatal(err)
			}
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, installed := range Scan(build(t, tt.setup)) {
				got = append(got, installed.Manifest.ID)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("Scan() ids = %v, want %v", got, tt.want)
			}
		})
	}
}

func copyTestFolder(t *testing.T, src, dest string) {
	t.Helper()
	if err := copyFolder(src, dest); err != nil {
		t.Fatal(err)
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		{"remove installed", "acme.rain", ""},
		{"remove unknown", "acme.unknown", "no such file"},
		{"remove refuses builtin", "builtin.matrix", "builtin"},
		{"remove refuses path traversal", "../acme.rain", "invalid id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extDir := filepath.Join(t.TempDir(), "extensions")
			if _, err := InstallFrom(validDir, extDir); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(extDir, "builtin.matrix"), 0o755); err != nil {
				t.Fatal(err)
			}
			err := Remove(tt.id, extDir)
			assertError(t, err, tt.wantErr)
			if tt.wantErr == "builtin" && !errors.Is(err, ErrBuiltin) {
				t.Fatalf("error %v does not wrap ErrBuiltin", err)
			}
			_, statErr := os.Stat(filepath.Join(extDir, "acme.rain"))
			if tt.wantErr == "" && !os.IsNotExist(statErr) {
				t.Fatalf("extension still on disk after Remove: %v", statErr)
			}
			if tt.wantErr != "" && statErr != nil {
				t.Fatalf("a failed Remove deleted something: %v", statErr)
			}
		})
	}
}

func assertError(t *testing.T, err error, wantSubstring string) {
	t.Helper()
	if wantSubstring == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("error = %v, want it to contain %q", err, wantSubstring)
	}
}

// assertNoLeftovers fails if extDir holds anything except the wanted folders,
// so temp staging folders and half-written installs are caught.
func assertNoLeftovers(t *testing.T, extDir string, wanted ...string) {
	t.Helper()
	entries, err := os.ReadDir(extDir)
	if os.IsNotExist(err) && len(wanted) == 0 {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if strings.Join(names, ",") != strings.Join(wanted, ",") {
		t.Fatalf("extDir holds %v, want %v", names, wanted)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(number int) string {
	data, _ := json.Marshal(number)
	return string(data)
}

// Built-ins are plain extension folders, so they must pass the same rules as
// outside ones. They are built into build/extensions by `npm run build`.
func TestAllBuiltinManifestsAreValid(t *testing.T) {
	const builtDir = "../../build/extensions"
	entries, err := os.ReadDir(builtDir)
	if err != nil {
		t.Skipf("%s is missing: run `npm run build` in web/ first (%v)", builtDir, err)
	}
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			manifest, err := readManifest(filepath.Join(builtDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if manifest.ID != entry.Name() {
				t.Errorf("folder %q holds id %q", entry.Name(), manifest.ID)
			}
		})
		checked++
	}
	if checked == 0 {
		t.Fatalf("%s holds no extensions", builtDir)
	}
}

// An agent's converter program must stay executable after install, from a
// folder and from a zip; other files get no execute bit.
func TestInstallKeepsTheExecuteBit(t *testing.T) {
	folder := t.TempDir()
	copyTestFolder(t, validDir, folder)
	writeFile(t, filepath.Join(folder, "convert"), "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(folder, "convert"), 0o755); err != nil {
		t.Fatal(err)
	}
	archive := writeZip(t, []zipEntry{
		{name: "extension.json", content: manifestJSON(t, "acme.zipped", "1.0.0")},
		{name: "main.js", content: "//"},
		{name: "convert", content: "#!/bin/sh\n", mode: 0o755},
	})
	for name, source := range map[string]string{"folder": folder, "zip": archive} {
		t.Run(name, func(t *testing.T) {
			extDir := t.TempDir()
			manifest, err := InstallFrom(source, extDir)
			if err != nil {
				t.Fatal(err)
			}
			for file, want := range map[string]os.FileMode{"convert": 0o755, "extension.json": 0o644} {
				info, err := os.Stat(filepath.Join(extDir, manifest.ID, file))
				if err != nil {
					t.Fatal(err)
				}
				if got := info.Mode().Perm(); got != want {
					t.Errorf("%s mode = %v, want %v", file, got, want)
				}
			}
		})
	}
}

func TestInspectRunsTheCheckOnAStagedCopyAndChangesNothing(t *testing.T) {
	// Given no installed extension
	extDir := t.TempDir()
	var seen string

	// When the valid background is inspected
	manifest, err := Inspect(validDir, extDir, func(m Manifest, dir string) error {
		seen = dir
		_, statErr := os.Stat(filepath.Join(dir, ManifestFile))
		return statErr
	})

	// Then the check saw a staged copy, and nothing is left in extDir
	if err != nil || manifest.ID != "acme.rain" {
		t.Fatalf("manifest = %+v, err %v", manifest, err)
	}
	if seen == "" || !strings.HasPrefix(seen, extDir) {
		t.Errorf("check ran on %q", seen)
	}
	entries, _ := os.ReadDir(extDir)
	if len(entries) != 0 {
		t.Errorf("extDir holds %v after an inspect", entries)
	}
}
