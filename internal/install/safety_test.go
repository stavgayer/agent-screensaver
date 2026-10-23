package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/paths"
)

// loadAgentDir reads one agent extension folder.
func loadAgentDir(t *testing.T, dir string) agents.Agent {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, extensions.ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest extensions.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := extensions.Validate(manifest, dir); err != nil {
		t.Fatal(err)
	}
	agent, err := agents.FromManifest(manifest, dir)
	if err != nil {
		t.Fatal(err)
	}
	return agent
}

// builtinAgents are the shipped Claude, Codex and Cursor agents.
func builtinAgents(t *testing.T) map[string]agents.Agent {
	t.Helper()
	return map[string]agents.Agent{
		"claude": loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.claude")),
		"codex":  loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.codex")),
		"cursor": loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.cursor")),
	}
}

func hookRecipe(change func(hooks *agents.HookRecipe)) agents.Recipe {
	hooks := agents.HookRecipe{File: "~/.acme/hooks.json", Format: "json", Shape: "flat", Events: []string{"stop"}}
	change(&hooks)
	return agents.Recipe{Hooks: &hooks}
}

func TestCheckRecipe(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".linked")); err != nil {
		t.Fatal(err)
	}
	dataDir := paths.ForHome(home).DataDir
	at := func(file string) agents.Recipe {
		return hookRecipe(func(hooks *agents.HookRecipe) { hooks.File = file })
	}
	copyTo := func(target string) agents.Recipe {
		return agents.Recipe{Files: []agents.FileCopy{{From: "x", To: target}}}
	}
	handler := func(raw string) agents.Recipe {
		return hookRecipe(func(hooks *agents.HookRecipe) { hooks.Handler = json.RawMessage(raw) })
	}
	tests := []struct {
		name    string
		recipe  agents.Recipe
		wantErr string
	}{
		{"valid flat recipe", at("~/.acme/hooks.json"), ""},
		{"path without ~/", at("/tmp/hooks.json"), "~/"},
		{"path with ..", at("~/../hooks.json"), ".."},
		{"symlink to outside home", at("~/.linked/hooks.json"), "outside the home"},
		{"~/.zshrc", agents.Recipe{Files: []agents.FileCopy{{From: "x", To: "~/.zshrc"}}}, "protected"},
		{"~/.ssh/config", agents.Recipe{Files: []agents.FileCopy{{From: "x", To: "~/.ssh/config"}}}, "protected"},
		{"~/Library/LaunchAgents/x.plist", agents.Recipe{Files: []agents.FileCopy{{From: "x", To: "~/Library/LaunchAgents/x.plist"}}}, "protected"},
		{"data dir file", at("~/Library/Application Support/agent-screensaver/config.json"), "data folder"},
		{"~/.config/fish/config.fish", agents.Recipe{Files: []agents.FileCopy{{From: "x", To: "~/.config/fish/config.fish"}}}, "protected"},
		{"hook file .toml", hookRecipe(func(hooks *agents.HookRecipe) { hooks.File, hooks.Format = "~/.codex/config.toml", "toml" }), "toml is not supported yet"},
		{"json format but not a .json file", at("~/.acme/hooks.yaml"), ".json"},
		{"handler key command", handler(`{"command":"rm -rf ~"}`), "command"},
		{"handler key type", handler(`{"type":"prompt"}`), "type"},
		{"handler key failClosed", handler(`{"failClosed":true}`), "failClosed"},
		{"timeout 0", handler(`{"timeout":0}`), "timeout"},
		{"timeout 31", handler(`{"timeout":31}`), "timeout"},
		{"timeout 5 is fine", handler(`{"timeout":5}`), ""},
		{"matcher not a string", hookRecipe(func(hooks *agents.HookRecipe) {
			hooks.Shape, hooks.Group = "grouped", json.RawMessage(`{"matcher":1}`)
		}), "matcher"},
		{"unknown shape", hookRecipe(func(hooks *agents.HookRecipe) { hooks.Shape = "nested" }), "shape"},
		{"event name with a shell character", hookRecipe(func(hooks *agents.HookRecipe) { hooks.Events = []string{"stop;rm"} }), "event name"},
		{"new_file may not set hooks", hookRecipe(func(hooks *agents.HookRecipe) { hooks.NewFile = json.RawMessage(`{"hooks":{}}`) }), "new_file"},
		{"no hooks and no files", agents.Recipe{}, "no hooks and no files"},
		{"mixed-case data dir file", at("~/library/application support/AGENT-screensaver/extensions/evil/extension.json"), "data folder"},
		{"~/.zlogin", copyTo("~/.zlogin"), "protected"},
		{"~/.zlogout", copyTo("~/.zlogout"), "protected"},
		{"~/.bash_login", copyTo("~/.bash_login"), "protected"},
		{"~/.bash_logout", copyTo("~/.bash_logout"), "protected"},
		{"long s folds to s: ~/.ſsh/config", copyTo("~/.\u017fsh/config"), "protected"},
		{"Kelvin sign folds to k: ~/Library/\u212aeychains/x", copyTo("~/Library/\u212aeychains/x"), "protected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckRecipe(home, dataDir, tt.recipe)
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
	for _, entry := range deniedPaths {
		for _, form := range []string{strings.ToUpper(entry), mixedCase(entry)} {
			target := "~/" + strings.TrimSuffix(form, "/")
			if strings.HasSuffix(entry, "/") {
				target += "/x.json"
			}
			t.Run("case form "+target, func(t *testing.T) {
				err := CheckRecipe(home, dataDir, copyTo(target))
				if err == nil || !strings.Contains(err.Error(), "protected") {
					t.Errorf("error = %v, want protected", err)
				}
			})
		}
	}
	for name, agent := range builtinAgents(t) {
		t.Run(name+" built-in passes", func(t *testing.T) {
			if err := CheckRecipe(home, dataDir, agent.Install); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestFilesTargetThatIsNotOursIsRefusedAtApply(t *testing.T) {
	home := t.TempDir()
	agent := pluginAgent(t, "acme.plugin")
	writeFile(t, filepath.Join(home, ".acme", "plugins", "agent-screensaver.js"), []byte("the user's own file"))
	rec := Record{Version: RecordVersion}

	err := AddAgentHooks(home, "/bin/ags", agent, &rec)

	if err == nil || !strings.Contains(err.Error(), "not our copy") {
		t.Errorf("error = %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, ".acme", "plugins", "agent-screensaver.js")); string(got) != "the user's own file" {
		t.Errorf("the user's file was replaced: %q", got)
	}
}

// mixedCase flips the case of every other letter.
func mixedCase(text string) string {
	runes := []rune(text)
	for index, r := range runes {
		if index%2 == 0 {
			runes[index] = []rune(strings.ToUpper(string(r)))[0]
		} else {
			runes[index] = []rune(strings.ToLower(string(r)))[0]
		}
	}
	return string(runes)
}
