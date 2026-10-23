package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/install"
)

func agentFolder(t *testing.T, hookFile string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "acme.agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "mark.svg"), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" fill="#888"/></svg>`)
	writeFile(t, filepath.Join(dir, "extension.json"), `{"id":"acme.agent","name":"Acme","version":"1.0.0","type":"agent","api":1,
		"agent":{"source":"acme","mark":"mark.svg",
		"hooks":{"event":"e","session":"id","rules":[{"kind":"finished"}]},
		"install":{"hooks":{"file":"`+hookFile+`","format":"json","shape":"flat","events":["stop"]}}}}`)
	return dir
}

func TestInstallAgentExtensionChecksTheRecipe(t *testing.T) {
	tests := []struct {
		name     string
		hookFile string
		wantErr  string
	}{
		{"a safe recipe is installed", "~/.acme/hooks.json", ""},
		{"a recipe that writes outside home is refused", "~/../hooks.json", ".."},
		{"a recipe that writes into ~/.ssh is refused", "~/.ssh/hooks.json", "protected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newService(t)

			_, err := service.InstallExtension(agentFolder(t, tt.hookFile))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if _, statErr := os.Stat(filepath.Join(service.Paths.Home, ".acme")); !os.IsNotExist(statErr) {
					t.Error("installing the extension added hooks")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if _, statErr := os.Stat(filepath.Join(service.Paths.ExtensionsDir, "acme.agent")); !os.IsNotExist(statErr) {
				t.Error("a refused extension was copied")
			}
		})
	}
}

// installWithHooks installs the test agent and adds its hooks.
func installWithHooks(t *testing.T, service Service) string {
	t.Helper()
	if _, err := service.InstallExtension(agentFolder(t, "~/.acme/hooks.json")); err != nil {
		t.Fatal(err)
	}
	loaded, _ := agents.Load(service.Paths.ExtensionsDir)
	agent, _ := agents.ByID(loaded, "acme.agent")
	record := install.Record{Version: install.RecordVersion, Bin: "/bin/ags"}
	if err := install.AddAgentHooks(service.Paths.Home, record.Bin, agent, &record); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(service.Paths.Home, ".acme", "hooks.json")
}

func TestRemoveAgentExtensionRemovesItsHooksFirst(t *testing.T) {
	service := newService(t)
	hookFile := installWithHooks(t, service)

	if err := service.RemoveExtension("acme.agent"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(hookFile); !os.IsNotExist(err) {
		t.Errorf("hook file left (err %v)", err)
	}
	record, err := install.LoadRecord(service.Paths.InstallRecord, nil)
	if err != nil || len(record.Agents) != 0 {
		t.Errorf("record = %+v, err %v", record, err)
	}
	if _, err := os.Stat(filepath.Join(service.Paths.ExtensionsDir, "acme.agent")); !os.IsNotExist(err) {
		t.Error("extension folder left")
	}
}

func TestRemoveAgentExtensionKeepsItWhenHooksCannotBeRemoved(t *testing.T) {
	service := newService(t)
	hookFile := installWithHooks(t, service)
	writeFile(t, hookFile, "{broken")

	err := service.RemoveExtension("acme.agent")

	if err == nil || !strings.Contains(err.Error(), "hooks") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(service.Paths.ExtensionsDir, "acme.agent", "extension.json")); err != nil {
		t.Errorf("the extension was removed: %v", err)
	}
}
