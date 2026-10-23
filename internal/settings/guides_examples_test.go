package settings

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/event"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/view"
)

// The folders in docs/guides/examples are the samples the how-to guides
// point at. Readers copy them, so each one must install with the real
// checks, and the agent samples must map their recorded payloads.
const guideExamples = "../../docs/guides/examples"

func exampleFolders(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(guideExamples)
	if err != nil {
		t.Fatal(err)
	}
	var folders []string
	for _, entry := range entries {
		manifest := filepath.Join(guideExamples, entry.Name(), extensions.ManifestFile)
		if _, err := os.Stat(manifest); err == nil {
			folders = append(folders, entry.Name())
		}
	}
	if len(folders) == 0 {
		t.Fatal("no examples found")
	}
	return folders
}

func TestGuideExamplesInstall(t *testing.T) {
	service := newService(t)
	for _, name := range exampleFolders(t) {
		t.Run(name, func(t *testing.T) {
			manifest, err := service.InstallExtension(filepath.Join(guideExamples, name))
			if err != nil {
				t.Fatalf("install %s: %v", name, err)
			}
			if manifest.ID != name {
				t.Errorf("folder %s holds id %s: the folder must be named after the id", name, manifest.ID)
			}
		})
	}
	installed := extensions.Scan(service.Paths.ExtensionsDir)
	if len(installed) != len(exampleFolders(t)) {
		t.Errorf("%d examples are installed, want %d", len(installed), len(exampleFolders(t)))
	}
}

// TestGuideExamplesShowTogether proves the theme sample works with the
// other samples and the shipped built-ins: its background, its layout and
// its widget all resolve.
func TestGuideExamplesShowTogether(t *testing.T) {
	service := newService(t)
	for _, name := range exampleFolders(t) {
		if _, err := service.InstallExtension(filepath.Join(guideExamples, name)); err != nil {
			t.Fatal(err)
		}
	}
	installed := append(extensions.Scan(service.Paths.ExtensionsDir), shippedBuiltinStubs(t)...)
	resolved := view.Resolve(config.Config{Theme: "acme.dusk"}, installed, nil)
	if resolved.Theme != "acme.dusk" || resolved.Background.ID != "acme.drift" {
		t.Fatalf("theme %q, background %q", resolved.Theme, resolved.Background.ID)
	}
	if resolved.Background.Options["speed"] != 0.6 {
		t.Errorf("the theme's background speed is %v, want 0.6", resolved.Background.Options["speed"])
	}
	found := map[string]bool{}
	for _, component := range resolved.Components {
		found[component.ID] = true
	}
	for _, id := range []string{"acme.agent-count", "builtin.clock", "builtin.task-row", "builtin.approval-card"} {
		if !found[id] {
			t.Errorf("component %s is not in the resolved view", id)
		}
	}
}

// shippedBuiltinStubs scans the real built-in manifests. The bundler writes
// their entry files only at build time, so each entry is stubbed.
func shippedBuiltinStubs(t *testing.T) []extensions.Installed {
	t.Helper()
	const repoExtensions = "../../extensions"
	dir := t.TempDir()
	folders, err := os.ReadDir(repoExtensions)
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range folders {
		source := filepath.Join(repoExtensions, folder.Name())
		target := filepath.Join(dir, folder.Name())
		if err := os.CopyFS(target, os.DirFS(source)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "index.js"), []byte("// entry"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return extensions.Scan(dir)
}

// TestGuidePayloads feeds the recorded-style payloads through the real
// mapping, the way the hook does, and checks the event kinds.
func TestGuidePayloads(t *testing.T) {
	service := newService(t)
	for _, name := range []string{"acme.quill", "acme.tern"} {
		if _, err := service.InstallExtension(filepath.Join(guideExamples, name)); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := agents.Load(service.Paths.ExtensionsDir)
	if err != nil {
		t.Fatal(err)
	}
	// macOS checks a new program on its first start, which can take longer
	// than the converter's timeout on a busy machine. The guide says to send
	// the first event again. Here the program is started once, with no time
	// limit, so the checked runs below measure the converter, not that check.
	tern, ok := agents.BySource(loaded, "tern")
	if !ok || tern.Hooks.Converter == nil {
		t.Fatal("tern has no converter")
	}
	if err := exec.Command(tern.Hooks.Converter.Run).Run(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		source, file, eventArg string
		want                   []event.Kind
		wantTitle              string
	}{
		{"quill", "quill/prompt.json", "", []event.Kind{event.KindStarted}, "Add a retry to the payment client"},
		{"quill", "quill/before_tool.json", "", []event.Kind{event.KindToolIntent}, "Bash: go test ./payments/..."},
		{"quill", "quill/permission.json", "", []event.Kind{event.KindApproval}, "Bash: rm -rf build"},
		{"quill", "quill/permission_answered.json", "", []event.Kind{event.KindResumed}, ""},
		{"quill", "quill/stop.json", "", []event.Kind{event.KindFinished}, ""},
		{"quill", "quill/debug.json", "", nil, ""},
		{"tern", "tern/start.txt", "start", []event.Kind{event.KindStarted}, "Fix the footer links"},
		{"tern", "tern/wait.txt", "wait", []event.Kind{event.KindApproval}, "Run the deploy script"},
		{"tern", "tern/done.txt", "done", []event.Kind{event.KindFinished}, ""},
		{"tern", "tern/done.txt", "unknown-event", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.file+" "+tt.eventArg, func(t *testing.T) {
			agent, ok := agents.BySource(loaded, tt.source)
			if !ok {
				t.Fatalf("agent %s is not installed", tt.source)
			}
			payload, err := os.ReadFile(filepath.Join(guideExamples, "payloads", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			env := agents.ConverterEnv(t.TempDir(), os.Getenv("PATH"), tt.source, tt.eventArg)
			// A busy test run can still be slow: try again, as the guide says.
			var events []event.Event
			for attempt := 0; attempt < 3; attempt++ {
				events, err = agent.Hooks.Events(context.Background(), payload, tt.eventArg, env, 1_760_000_000_000)
				if err == nil {
					break
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != len(tt.want) {
				t.Fatalf("got %d events, want %d: %+v", len(events), len(tt.want), events)
			}
			for index, kind := range tt.want {
				if events[index].Kind != kind || events[index].Source != tt.source || events[index].Title != tt.wantTitle {
					t.Errorf("event %d = %+v, want kind %s title %q", index, events[index], kind, tt.wantTitle)
				}
			}
		})
	}
}
