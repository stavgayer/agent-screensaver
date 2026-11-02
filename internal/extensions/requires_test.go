package extensions

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// runningAs sets RunningVersion for one test.
func runningAs(t *testing.T, version string) {
	t.Helper()
	before := RunningVersion
	RunningVersion = version
	t.Cleanup(func() { RunningVersion = before })
}

func requiresJSON(t *testing.T, id, requires string) string {
	t.Helper()
	data, err := json.Marshal(Manifest{ID: id, Name: "Needs", Version: "1.0.0", Type: TypeBackground, API: APIVersion, Requires: requires, Entry: "main.js"})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestValidateRequires(t *testing.T) {
	tests := []struct {
		name     string
		running  string
		requires string
		wantErr  string
	}{
		{"no requires", "0.4.0", "", ""},
		{"newer is refused", "0.4.4", "0.5.0", "acme.rain needs agent-screensaver 0.5.0 or later; this is 0.4.4. Update agent-screensaver first."},
		{"equal is ok", "0.5.0", "0.5.0", ""},
		{"older is ok", "0.6.1", "0.5.0", ""},
		{"compares numbers, not text", "0.9.0", "0.10.0", "needs agent-screensaver 0.10.0 or later"},
		{"ten is above nine", "0.10.0", "0.9.0", ""},
		{"patch decides", "0.5.0", "0.5.1", "needs agent-screensaver 0.5.1 or later"},
		{"dev skips the check", "dev", "9.9.9", ""},
		{"a bare commit skips the check", "b86fee7", "9.9.9", ""},
		{"v prefix", "v0.5.0", "0.5.0", ""},
		{"v prefix too old", "v0.4.4", "0.5.0", "this is v0.4.4"},
		{"git describe after a tag compares the leading x.y.z", "v0.4.4-3-gabc1234", "0.5.0", "needs agent-screensaver 0.5.0 or later; this is v0.4.4-3-gabc1234"},
		{"git describe after a tag, enough", "v0.5.0-3-gabc1234-dirty", "0.5.0", ""},
		{"two numbers", "0.5.0", "0.5", "requires"},
		{"four numbers", "0.5.0", "0.5.0.1", "requires"},
		{"v in requires", "0.5.0", "v0.5.0", "requires"},
		{"pre-release suffix", "0.5.0", "0.5.0-rc1", "requires"},
		{"words", "0.5.0", "latest", "requires"},
		{"malformed is invalid even for dev", "dev", "5", "requires"},
		{"too large", "dev", "99999999999999999999.0.0", "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runningAs(t, tt.running)
			manifest := baseBackground()
			manifest.Requires = tt.requires
			assertError(t, Validate(manifest, validDir), tt.wantErr)
		})
	}
}

func TestInstallRefusesAnExtensionThatNeedsANewerApp(t *testing.T) {
	runningAs(t, "0.4.4")
	src := writeZip(t, []zipEntry{
		{name: "extension.json", content: requiresJSON(t, "acme.rain", "0.5.0")},
		{name: "main.js", content: "x"},
	})
	extDir := filepath.Join(t.TempDir(), "extensions")
	_, err := InstallFrom(src, extDir)
	assertError(t, err, "acme.rain needs agent-screensaver 0.5.0 or later; this is 0.4.4. Update agent-screensaver first.")
	assertNoLeftovers(t, extDir)

	runningAs(t, "0.5.0")
	if _, err := InstallFrom(src, extDir); err != nil {
		t.Fatalf("equal version: %v", err)
	}
	runningAs(t, "dev")
	if _, err := InstallFrom(src, extDir); err != nil {
		t.Fatalf("dev build: %v", err)
	}
}

func TestScanSkipsAnExtensionThatNeedsANewerApp(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	runningAs(t, "dev")
	if _, err := InstallFrom(validDir, extDir); err != nil {
		t.Fatal(err)
	}
	newer := writeZip(t, []zipEntry{
		{name: "extension.json", content: requiresJSON(t, "acme.future", "0.5.0")},
		{name: "main.js", content: "x"},
	})
	if _, err := InstallFrom(newer, extDir); err != nil {
		t.Fatal(err)
	}

	runningAs(t, "0.4.4")
	found, err := ScanWithProblems(extDir)
	if len(found) != 1 || found[0].Manifest.ID != "acme.rain" {
		t.Fatalf("ScanWithProblems() found %v, want only acme.rain", found)
	}
	assertError(t, err, "extension acme.future: acme.future needs agent-screensaver 0.5.0 or later; this is 0.4.4. Update agent-screensaver first.")
	if got := len(Scan(extDir)); got != 1 {
		t.Fatalf("Scan() found %d extensions, want 1", got)
	}

	runningAs(t, "0.5.0")
	if got := len(Scan(extDir)); got != 2 {
		t.Fatalf("Scan() found %d extensions at 0.5.0, want 2", got)
	}
}

func TestInstallPackFailsAsAWholeWhenOnePartNeedsANewerApp(t *testing.T) {
	runningAs(t, "0.4.4")
	parts := twoParts(t)
	parts["rain"] = requiresJSON(t, "acme.rain", "0.5.0")
	extDir := filepath.Join(t.TempDir(), "extensions")
	_, err := InstallPack(packFolder(t, parts), extDir, nil)
	assertError(t, err, "acme.rain needs agent-screensaver 0.5.0 or later; this is 0.4.4. Update agent-screensaver first.")
	assertError(t, err, `install pack part "rain"`)
	assertNoLeftovers(t, extDir)

	runningAs(t, "0.5.0")
	if _, err := InstallPack(packFolder(t, parts), extDir, nil); err != nil {
		t.Fatalf("0.5.0: %v", err)
	}
}
