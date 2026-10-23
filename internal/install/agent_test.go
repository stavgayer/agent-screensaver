package install

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDetected(t *testing.T) {
	home := t.TempDir()
	agent := cursorAgent(t)
	if Detected(home, agent) {
		t.Fatal("Cursor detected without ~/.cursor")
	}
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !Detected(home, agent) {
		t.Fatal("Cursor not detected with ~/.cursor")
	}
}

func TestHooksInPlace(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, home string)
		want   bool
	}{
		{"just added", func(*testing.T, string) {}, true},
		{"the user removed our entries", func(t *testing.T, home string) {
			writeFile(t, cursorPath(home), []byte(`{"version":1,"hooks":{}}`))
		}, false},
		{"the user deleted the file", func(t *testing.T, home string) {
			if err := os.Remove(cursorPath(home)); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			rec := Record{Version: RecordVersion}
			addAgent(t, home, cursorAgent(t), &rec)
			tt.change(t, home)

			entry, _ := rec.Agent("builtin.cursor")
			if got := HooksInPlace(home, entry); got != tt.want {
				t.Fatalf("HooksInPlace = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHooksInPlaceChecksCopiedFiles(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	addAgent(t, home, pluginAgent(t, "acme.agent"), &rec)
	entry, _ := rec.Agent("acme.agent")
	if !HooksInPlace(home, entry) {
		t.Fatal("plugin file just copied, want in place")
	}
	if err := os.Remove(filepath.Join(home, ".acme", "plugins", "agent-screensaver.js")); err != nil {
		t.Fatal(err)
	}
	if HooksInPlace(home, entry) {
		t.Fatal("plugin file removed, want changed")
	}
}

func TestPlanRemoveAgentNamesTheRecordedFiles(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	addAgent(t, home, pluginAgent(t, "acme.agent"), &rec)
	addAgent(t, home, cursorAgent(t), &rec)

	cursor, _ := rec.Agent("builtin.cursor")
	plugin, _ := rec.Agent("acme.agent")
	cursorPlan, pluginPlan := PlanRemoveAgent(home, cursor), PlanRemoveAgent(home, plugin)

	if len(cursorPlan) != 1 || !strings.Contains(cursorPlan[0].Description, cursorPath(home)) {
		t.Errorf("cursor plan = %+v", cursorPlan)
	}
	if len(pluginPlan) != 1 || !strings.Contains(pluginPlan[0].Description, "~/.acme/plugins/agent-screensaver.js") {
		t.Errorf("plugin plan = %+v", pluginPlan)
	}
}

func TestTouchedFiles(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	agent := pluginAgent(t, "acme.agent")
	addAgent(t, home, cursorAgent(t), &rec)
	entry, _ := rec.Agent("builtin.cursor")

	got := TouchedFiles(home, agent, entry)

	for _, want := range []string{filepath.Join(home, ".acme", "plugins", "agent-screensaver.js"), cursorPath(home)} {
		if !slices.Contains(got, want) {
			t.Errorf("TouchedFiles = %q, want it to hold %q", got, want)
		}
	}
}
