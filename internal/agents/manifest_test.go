package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/extensions"
)

// builtinDir is the repo folder that holds the built-in extensions.
const builtinDir = "../../extensions"

// agentCopy copies a built-in agent folder to a temp folder named id, lets
// mutate change the "agent" object of its manifest, and returns the folder.
func agentCopy(t *testing.T, builtin, id string, mutate func(agent map[string]any)) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	copyFolder(t, filepath.Join(builtinDir, builtin), dir)
	path := filepath.Join(dir, extensions.ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["id"] = id
	if mutate != nil {
		mutate(manifest["agent"].(map[string]any))
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, string(data), 0o644)
	return dir
}

func copyFolder(t *testing.T, from, to string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if entry.IsDir() {
			copyFolder(t, filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name()))
			continue
		}
		data, err := os.ReadFile(filepath.Join(from, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(to, entry.Name()), string(data), info.Mode().Perm())
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func readManifest(t *testing.T, dir string) extensions.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, extensions.ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest extensions.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// object and list walk the decoded manifest in mutations.
func object(value any, key string) map[string]any {
	return value.(map[string]any)[key].(map[string]any)
}
func list(value any, key string) []any { return value.(map[string]any)[key].([]any) }
func firstRule(agent map[string]any) map[string]any {
	return list(object(agent, "hooks"), "rules")[0].(map[string]any)
}

// firstRuleWithSet is the first hook rule that sets fields (the first rule
// may be an ignore rule, as in builtin.claude).
func firstRuleWithSet(agent map[string]any) map[string]any {
	for _, rule := range list(object(agent, "hooks"), "rules") {
		if _, ok := rule.(map[string]any)["set"]; ok {
			return rule.(map[string]any)
		}
	}
	return nil
}

func TestValidateAgentManifest(t *testing.T) {
	setMark := func(content string) func(t *testing.T, dir string) {
		return func(t *testing.T, dir string) { writeTestFile(t, filepath.Join(dir, "mark.svg"), content, 0o644) }
	}
	tests := []struct {
		name    string
		builtin string
		mutate  func(agent map[string]any)
		files   func(t *testing.T, dir string)
		wantErr string
	}{
		{name: "valid claude manifest", builtin: "builtin.claude"},
		{name: "valid codex manifest", builtin: "builtin.codex"},
		{name: "valid cursor manifest", builtin: "builtin.cursor"},
		{name: "missing mark", builtin: "builtin.claude", mutate: func(agent map[string]any) { delete(agent, "mark") }, wantErr: "agent.mark"},
		{name: "mark over 8 KiB", builtin: "builtin.claude", files: setMark("<svg>" + strings.Repeat(" ", 9000) + "</svg>"), wantErr: "limit"},
		{name: "mark with script", builtin: "builtin.claude", files: setMark(`<svg><script>x()</script></svg>`), wantErr: "script"},
		{name: "mark with foreignObject", builtin: "builtin.claude", files: setMark(`<svg><foreignObject/></svg>`), wantErr: "foreignObject"},
		{name: "mark with onload", builtin: "builtin.claude", files: setMark(`<svg onload="x()"/>`), wantErr: "on..."},
		{name: "mark with external href", builtin: "builtin.claude", files: setMark(`<svg><image href="http://x/y.png"/></svg>`), wantErr: "link"},
		{name: "mark with url(http", builtin: "builtin.claude", files: setMark(`<svg><rect fill="url(http://x/y)"/></svg>`), wantErr: "url("},
		{name: "source with uppercase", builtin: "builtin.claude", mutate: func(agent map[string]any) { agent["source"] = "Claude" }, wantErr: "agent.source"},
		{name: "unknown kind in rule", builtin: "builtin.claude", mutate: func(agent map[string]any) { firstRule(agent)["kind"] = "paused" }, wantErr: "kind"},
		{name: "rule with converter and rules", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(agent, "hooks")["converter"] = map[string]any{"run": "mark.svg"}
		}, wantErr: "converter"},
		{name: "unknown transform", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(firstRuleWithSet(agent), "set")["title"] = map[string]any{"path": "prompt", "as": "upper"}
		}, wantErr: "transform"},
		{name: "two operators in one condition", builtin: "builtin.codex", mutate: func(agent map[string]any) {
			firstRule(agent)["if"] = map[string]any{"path": "transcript_path", "equals": nil, "present": true}
		}, wantErr: "one operator"},
		{name: "unknown field in a rule", builtin: "builtin.claude", mutate: func(agent map[string]any) { firstRule(agent)["when"] = "x" }, wantErr: "when"},
		{name: "unknown limits provider", builtin: "builtin.codex", mutate: func(agent map[string]any) {
			agent["limits"] = map[string]any{"provider": "web-scraper"}
		}, wantErr: "limits provider"},
		{name: "reply allow", builtin: "builtin.claude", mutate: func(agent map[string]any) { object(agent, "hooks")["reply"] = "allow" }, wantErr: "reply"},
		{name: "reply deny", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(agent, "hooks")["reply"] = map[string]any{"default": "none", "events": map[string]any{"Stop": "deny"}}
		}, wantErr: "reply"},
		{name: "reply of any other value", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(agent, "hooks")["reply"] = map[string]any{"default": `{"permission":"allow"}`}
		}, wantErr: "reply"},
		{name: "reply ask", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(agent, "hooks")["reply"] = map[string]any{"default": "empty_object", "events": map[string]any{"PermissionRequest": "ask"}}
		}, wantErr: "reply"},
		{name: "default reply ask", builtin: "builtin.claude", mutate: func(agent map[string]any) { object(agent, "hooks")["reply"] = "ask" }, wantErr: "reply"},
		{name: "per-event empty_object is allowed", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(agent, "hooks")["reply"] = map[string]any{"default": "none", "events": map[string]any{"PermissionRequest": "empty_object"}}
		}},
		{name: "on in a status line rule", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			list(object(agent, "statusline"), "rules")[0].(map[string]any)["on"] = "x"
		}, wantErr: "on"},
		{name: "ratio outside details", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			object(firstRuleWithSet(agent), "set")["context_percent"] = map[string]any{"ratio": []any{"a", "b"}}
		}, wantErr: "ratio"},
		{name: "unknown details provider", builtin: "builtin.claude", mutate: func(agent map[string]any) {
			list(object(agent, "details"), "context_percent")[0] = map[string]any{"provider": "x"}
		}, wantErr: "details provider"},
		{name: "details tail over 1024 KiB", builtin: "builtin.codex", mutate: func(agent map[string]any) {
			list(object(agent, "details"), "name")[0].(map[string]any)["tail_kb"] = 2048
		}, wantErr: "tail_kb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := agentCopy(t, tt.builtin, "acme.agent", tt.mutate)
			if tt.files != nil {
				tt.files(t, dir)
			}
			err := Validate(readManifest(t, dir), dir)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestInstallOfSecondAgentWithSameSourceIsRefused(t *testing.T) {
	extDir := filepath.Join(t.TempDir(), "extensions")
	if _, err := extensions.InstallFrom(agentCopy(t, "builtin.codex", "acme.one", nil), extDir); err != nil {
		t.Fatal(err)
	}

	_, err := extensions.InstallFrom(agentCopy(t, "builtin.codex", "acme.two", nil), extDir)

	if err == nil || !strings.Contains(err.Error(), "already used by acme.one") {
		t.Errorf("error = %v", err)
	}
}

func TestLoadFindsBuiltinsBySource(t *testing.T) {
	// The repo folder holds the web built-ins without their built index.js,
	// so Load reports those; the agents need no build.
	loaded, err := Load(builtinDir)
	if err != nil && strings.Contains(err.Error(), "agent") {
		t.Fatalf("Load: %v", err)
	}
	for _, source := range []string{"claude", "codex"} {
		agent, ok := BySource(loaded, source)
		if !ok || !agent.Builtin || agent.Mark == "" || agent.Name == "" {
			t.Errorf("BySource(%q) = %+v, %v", source, agent, ok)
		}
	}
	if _, ok := BySource(loaded, "nobody"); ok {
		t.Error("found an agent for an unknown source")
	}
}

func TestLoadSkipsAndReportsInvalidAgents(t *testing.T) {
	extDir := t.TempDir()
	good := agentCopy(t, "builtin.codex", "acme.good", func(agent map[string]any) { agent["source"] = "good" })
	bad := agentCopy(t, "builtin.codex", "acme.bad", func(agent map[string]any) { firstRule(agent)["kind"] = "paused" })
	for _, dir := range []string{good, bad} {
		copyFolder(t, dir, filepath.Join(extDir, filepath.Base(dir)))
	}

	loaded, err := Load(extDir)

	if len(loaded) != 1 || loaded[0].ID != "acme.good" {
		t.Errorf("loaded = %+v", loaded)
	}
	if err == nil || !strings.Contains(err.Error(), "acme.bad") {
		t.Errorf("error = %v, want it to name acme.bad", err)
	}
}
