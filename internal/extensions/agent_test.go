package extensions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodMark = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#e3a46f" d="M2 2h20v20H2z"/></svg>`

// agentFolder writes an agent extension with the given source and mark.
func agentFolder(t *testing.T, id, source, mark string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{"source": source, "mark": "mark.svg"})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(Manifest{ID: id, Name: "Agent", Version: "1.0.0", Type: TypeAgent, API: APIVersion, Agent: body})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ManifestFile), string(manifest))
	writeFile(t, filepath.Join(dir, "mark.svg"), mark)
	return dir
}

func TestValidateAgent(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		mark    string
		mutate  func(m *Manifest)
		wantErr string
	}{
		{"valid agent", "acme", goodMark, nil, ""},
		{"source with uppercase", "Acme", goodMark, nil, "agent.source"},
		{"source too long", strings.Repeat("a", 33), goodMark, nil, "agent.source"},
		{"missing mark", "acme", "", func(m *Manifest) { m.Agent = json.RawMessage(`{"source":"acme"}`) }, "agent.mark"},
		{"mark outside the folder", "acme", goodMark, func(m *Manifest) { m.Agent = json.RawMessage(`{"source":"acme","mark":"../x.svg"}`) }, "agent.mark"},
		{"mark over 8 KiB", "acme", "<svg>" + strings.Repeat(" ", MaxMarkBytes) + "</svg>", nil, "limit"},
		{"mark is not svg", "acme", "<html></html>", nil, "<svg>"},
		{"mark with script", "acme", `<svg><script>alert(1)</script></svg>`, nil, "script"},
		{"mark with foreignObject", "acme", `<svg><foreignObject><div/></foreignObject></svg>`, nil, "foreignObject"},
		{"mark with onload", "acme", `<svg onload="x()"></svg>`, nil, "on..."},
		{"mark with external href", "acme", `<svg><image href="https://example.com/a.png"/></svg>`, nil, "link"},
		{"mark with external xlink:href", "acme", `<svg><use xlink:href="file:///etc/x"/></svg>`, nil, "link"},
		{"mark with url(http", "acme", `<svg><rect style="fill:url(http://x/y)"/></svg>`, nil, "url("},
		{"mark with @import", "acme", `<svg><style>@import "x.css";</style></svg>`, nil, "@import"},
		{"mark with local href and url is fine", "acme",
			`<svg><defs><linearGradient id="g"/></defs><use href="#g"/><rect fill="url(#g)"/></svg>`, nil, ""},
		{"agent with entry", "acme", goodMark, func(m *Manifest) { m.Entry = "x.js" }, "no entry"},
		{"agent without body", "acme", goodMark, func(m *Manifest) { m.Agent = nil }, "agent is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := agentFolder(t, "acme.agent", tt.source, tt.mark)
			manifest, err := os.ReadFile(filepath.Join(dir, ManifestFile))
			if err != nil {
				t.Fatal(err)
			}
			var parsed Manifest
			if err := json.Unmarshal(manifest, &parsed); err != nil {
				t.Fatal(err)
			}
			if tt.mutate != nil {
				tt.mutate(&parsed)
			}
			assertError(t, Validate(parsed, dir), tt.wantErr)
		})
	}
}

func TestInstallOfSecondAgentWithSameSourceIsRefused(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	if _, err := InstallFrom(agentFolder(t, "acme.first", "acme", goodMark), extDir); err != nil {
		t.Fatal(err)
	}

	_, err := InstallFrom(agentFolder(t, "acme.second", "acme", goodMark), extDir)

	assertError(t, err, `source "acme" is already used by acme.first`)
	assertNoLeftovers(t, extDir, "acme.first")
	if _, err := InstallFrom(agentFolder(t, "acme.first", "acme", goodMark), extDir); err != nil {
		t.Errorf("upgrade of the same id: %v", err)
	}
}

func TestInstallCheckedRunsTheCheckBeforeReplacing(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	refuse := func(Manifest, string) error { return os.ErrPermission }

	_, err := InstallChecked(agentFolder(t, "acme.first", "acme", goodMark), extDir, refuse)

	assertError(t, err, "permission")
	assertNoLeftovers(t, extDir)
}

func TestScanWithProblemsNamesSkippedFolders(t *testing.T) {
	extDir := t.TempDir()
	bad := agentFolder(t, "acme.bad", "acme", `<svg onload="x()"></svg>`)
	if err := os.Rename(bad, filepath.Join(extDir, "acme.bad")); err != nil {
		t.Fatal(err)
	}

	found, err := ScanWithProblems(extDir)

	if len(found) != 0 || err == nil || !strings.Contains(err.Error(), "acme.bad") {
		t.Errorf("found %v, err %v", found, err)
	}
}
