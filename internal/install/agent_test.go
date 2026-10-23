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

func TestCheckHooks(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, home string)
		want   HooksState
	}{
		{"just added", func(*testing.T, string) {}, HooksOK},
		{"the user removed our entries", func(t *testing.T, home string) {
			writeFile(t, cursorPath(home), []byte(`{"version":1,"hooks":{}}`))
		}, HooksChanged},
		{"the user deleted the file", func(t *testing.T, home string) {
			if err := os.Remove(cursorPath(home)); err != nil {
				t.Fatal(err)
			}
		}, HooksChanged},
		{"our stop command moved to another event", func(t *testing.T, home string) {
			data, err := os.ReadFile(cursorPath(home))
			if err != nil {
				t.Fatal(err)
			}
			moved := strings.Replace(string(data), `"stop"`, `"somethingElse"`, 1)
			if moved == string(data) {
				t.Fatal("no stop event in the hook file")
			}
			writeFile(t, cursorPath(home), []byte(moved))
		}, HooksChanged},
		{"our shell command moved to another event", func(t *testing.T, home string) {
			data, err := os.ReadFile(cursorPath(home))
			if err != nil {
				t.Fatal(err)
			}
			moved := strings.Replace(string(data), `"beforeShellExecution"`, `"x"`, 1)
			writeFile(t, cursorPath(home), []byte(moved))
		}, HooksChanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			rec := Record{Version: RecordVersion}
			addAgent(t, home, cursorAgent(t), &rec)
			tt.change(t, home)

			entry, _ := rec.Agent("builtin.cursor")
			if got := CheckHooks(home, entry); got != tt.want {
				t.Fatalf("CheckHooks = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckHooksSeesTheUsersOwnStatusline(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	addAgent(t, home, loadAgentDir(t, filepath.Join("..", "..", "extensions", "builtin.claude")), &rec)
	entry, _ := rec.Agent("builtin.claude")
	if got := CheckHooks(home, entry); got != HooksOK {
		t.Fatalf("after add = %q", got)
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, settings, []byte(strings.Replace(string(data), jsonEscaped(entry.StatuslineCommand), "my-statusline", 1)))

	if got := CheckHooks(home, entry); got != HooksOwnStatusline {
		t.Fatalf("with the user's status line = %q, want %q", got, HooksOwnStatusline)
	}
}

func TestCheckHooksChecksCopiedFiles(t *testing.T) {
	home := t.TempDir()
	rec := Record{Version: RecordVersion}
	addAgent(t, home, pluginAgent(t, "acme.agent"), &rec)
	entry, _ := rec.Agent("acme.agent")
	if CheckHooks(home, entry) != HooksOK {
		t.Fatal("plugin file just copied, want in place")
	}
	if err := os.Remove(filepath.Join(home, ".acme", "plugins", "agent-screensaver.js")); err != nil {
		t.Fatal(err)
	}
	if CheckHooks(home, entry) != HooksChanged {
		t.Fatal("plugin file removed, want changed")
	}
}

func TestSourceFiles(t *testing.T) {
	agent := pluginAgent(t, "acme.agent")
	got := SourceFiles(agent)
	for _, want := range []string{
		filepath.Join(agent.Dir, "extension.json"),
		filepath.Join(agent.Dir, "bin", "convert"),
		filepath.Join(agent.Dir, "plugin", "agent-screensaver.js"),
	} {
		if !slices.Contains(got, want) {
			t.Errorf("SourceFiles = %q, want it to hold %q", got, want)
		}
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
