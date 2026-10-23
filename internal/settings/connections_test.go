package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/install"
)

const minute = int64(60_000)

func ms(value int64) *int64 { return &value }

func TestConnectionStateFor(t *testing.T) {
	const now = 1_000 * minute
	tests := []struct {
		name                   string
		found, hooksAdded, off bool
		lastEventMs            *int64
		want                   ConnectionState
	}{
		{"off wins over everything", false, false, true, ms(now), StateOff},
		{"detect missing → not_installed", false, true, false, ms(now), StateNotInstalled},
		{"found, no hooks → no_hooks", true, false, false, ms(now), StateNoHooks},
		{"event 9m ago → active", true, true, false, ms(now - 9*minute), StateActive},
		{"event 10m ago → idle", true, true, false, ms(now - 10*minute), StateIdle},
		{"no event yet → idle", true, true, false, nil, StateIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConnectionStateFor(tt.found, tt.hooksAdded, tt.off, tt.lastEventMs, now); got != tt.want {
				t.Fatalf("state = %q, want %q", got, tt.want)
			}
		})
	}
}

// installBuiltinAgents copies the repository's built-in agent extensions
// into the service's extensions folder, as install does.
func installBuiltinAgents(t *testing.T, service Service) {
	t.Helper()
	for _, id := range []string{"builtin.claude", "builtin.codex", "builtin.cursor"} {
		source := os.DirFS(filepath.Join("..", "..", "extensions", id))
		if err := os.CopyFS(filepath.Join(service.Paths.ExtensionsDir, id), source); err != nil {
			t.Fatal(err)
		}
	}
}

// saveRecord writes an install record with no agents, as install leaves it
// when no tool is found.
func saveRecord(t *testing.T, service Service) {
	t.Helper()
	record := install.Record{Version: install.RecordVersion, Bin: install.BinPath(service.Paths.Home)}
	if err := install.SaveRecord(service.Paths.InstallRecord, record); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func byConnectionID(t *testing.T, list []Connection, id string) Connection {
	t.Helper()
	index := slices.IndexFunc(list, func(c Connection) bool { return c.ID == id })
	if index < 0 {
		t.Fatalf("no connection %s in %+v", id, list)
	}
	return list[index]
}

func TestConnectionsListsBuiltinsAndOutside(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	if _, err := service.InstallExtension(agentFolder(t, "~/.acme/hooks.json")); err != nil {
		t.Fatal(err)
	}
	mkdir(t, filepath.Join(service.Paths.Home, ".claude"))

	list, err := service.Connections(100 * minute)
	if err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, connection := range list {
		ids = append(ids, connection.ID)
	}
	if want := []string{"builtin.claude", "builtin.codex", "builtin.cursor", "acme.agent"}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %q, want %q (built-ins first, then by name)", ids, want)
	}
	claude := byConnectionID(t, list, "builtin.claude")
	if claude.Name != "Claude Code" || claude.Source != "claude" || !claude.Builtin || claude.State != StateNoHooks || !claude.Found {
		t.Errorf("claude = %+v", claude)
	}
	if !strings.HasPrefix(claude.Mark, "file://") || !strings.HasSuffix(claude.Mark, "/builtin.claude/mark.svg") {
		t.Errorf("claude mark = %q", claude.Mark)
	}
	if codex := byConnectionID(t, list, "builtin.codex"); codex.State != StateNotInstalled || codex.Found {
		t.Errorf("codex without ~/.codex = %+v", codex)
	}
	acme := byConnectionID(t, list, "acme.agent")
	if acme.Builtin || acme.Version != "1.0.0" || acme.State != StateNoHooks {
		t.Errorf("acme = %+v", acme)
	}
}

func TestConnectionsReadLastEventFromState(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	mkdir(t, filepath.Join(service.Paths.Home, ".cursor"))
	saveRecord(t, service)
	addHooks(t, service, "builtin.cursor")
	const now = 500 * minute
	state := map[string]any{"sources": map[string]any{"cursor": map[string]any{"last_event_ms": now - 4*minute}}}
	data, _ := json.Marshal(state)
	writeFile(t, service.Paths.StateFile, string(data))

	list, err := service.Connections(now)
	if err != nil {
		t.Fatal(err)
	}

	cursor := byConnectionID(t, list, "builtin.cursor")
	if cursor.State != StateActive || cursor.LastEventMs == nil || *cursor.LastEventMs != now-4*minute {
		t.Errorf("cursor = %+v, want active with the state's last event", cursor)
	}
	if claude := byConnectionID(t, list, "builtin.claude"); claude.LastEventMs != nil {
		t.Errorf("claude last event = %v, want none", *claude.LastEventMs)
	}
}

// addHooks plans and applies adding one agent's hooks through the service.
func addHooks(t *testing.T, service Service, id string) {
	t.Helper()
	_, hash, err := service.PlanAgentHooks(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyAgentHooks(id, true, hash); err != nil {
		t.Fatal(err)
	}
}

func TestHooksChangedWhenUserRemovedOurEntries(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	mkdir(t, filepath.Join(service.Paths.Home, ".cursor"))
	saveRecord(t, service)
	addHooks(t, service, "builtin.cursor")
	hookFile := filepath.Join(service.Paths.Home, ".cursor", "hooks.json")

	before, _ := service.Connections(0)
	writeFile(t, hookFile, `{"version":1,"hooks":{}}`)
	after, _ := service.Connections(0)

	if c := byConnectionID(t, before, "builtin.cursor"); c.HooksChanged || !c.HooksAdded {
		t.Errorf("before = %+v, want hooks added and unchanged", c)
	}
	if c := byConnectionID(t, after, "builtin.cursor"); !c.HooksChanged || !c.HooksAdded || c.State != StateIdle {
		t.Errorf("after = %+v, want hooks changed, still recorded as added", c)
	}
}

func TestSetAgentOnPersistsAgentsOff(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)

	if err := service.SetAgentOn("builtin.codex", false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetAgentOn("builtin.codex", false); err != nil {
		t.Fatal(err)
	}
	if got := config.Load(service.Paths.ConfigFile).AgentsOff; !slices.Equal(got, []string{"builtin.codex"}) {
		t.Fatalf("agents_off = %q, want once", got)
	}
	list, _ := service.Connections(0)
	if c := byConnectionID(t, list, "builtin.codex"); c.State != StateOff {
		t.Errorf("codex = %+v, want off", c)
	}

	if err := service.SetAgentOn("builtin.codex", true); err != nil {
		t.Fatal(err)
	}
	if got := config.Load(service.Paths.ConfigFile).AgentsOff; len(got) != 0 {
		t.Fatalf("agents_off = %q, want empty", got)
	}
	if err := service.SetAgentOn("acme.missing", false); err == nil {
		t.Error("turning off an agent that is not installed was accepted")
	}
}

func TestApplyRefusesStaleHash(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	mkdir(t, filepath.Join(service.Paths.Home, ".cursor"))
	saveRecord(t, service)
	_, hash, err := service.PlanAgentHooks("builtin.cursor", true)
	if err != nil {
		t.Fatal(err)
	}
	hookFile := writeFile(t, filepath.Join(service.Paths.Home, ".cursor", "hooks.json"), `{"version":1,"hooks":{"stop":[{"command":"mine"}]}}`)

	err = service.ApplyAgentHooks("builtin.cursor", true, hash)

	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("error = %v, want the plan changed", err)
	}
	if data, _ := os.ReadFile(hookFile); string(data) != `{"version":1,"hooks":{"stop":[{"command":"mine"}]}}` {
		t.Errorf("hook file changed:\n%s", data)
	}
}

func TestApplyAddsThenRemoves(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	mkdir(t, filepath.Join(service.Paths.Home, ".cursor"))
	saveRecord(t, service)
	hookFile := writeFile(t, filepath.Join(service.Paths.Home, ".cursor", "hooks.json"), `{"version":1,"hooks":{"stop":[{"command":"mine"}]}}`)

	addHooks(t, service, "builtin.cursor")
	added, _ := service.Connections(0)
	data, _ := os.ReadFile(hookFile)
	if !strings.Contains(string(data), "hook --tool cursor") {
		t.Fatalf("hooks not added:\n%s", data)
	}

	plan, hash, err := service.PlanAgentHooks("builtin.cursor", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 || !strings.Contains(plan[0].Description, "Remove our cursor hooks") {
		t.Fatalf("remove plan = %+v", plan)
	}
	if err := service.ApplyAgentHooks("builtin.cursor", false, hash); err != nil {
		t.Fatal(err)
	}

	removed, _ := service.Connections(0)
	if c := byConnectionID(t, added, "builtin.cursor"); c.State != StateIdle {
		t.Errorf("after add = %+v, want idle", c)
	}
	if c := byConnectionID(t, removed, "builtin.cursor"); c.State != StateNoHooks {
		t.Errorf("after remove = %+v, want no hooks", c)
	}
	var file map[string]any
	data, _ = os.ReadFile(hookFile)
	if err := json.Unmarshal(data, &file); err != nil || strings.Contains(string(data), "agent-screensaver") || !strings.Contains(string(data), `"mine"`) {
		t.Errorf("after remove the file is %s (err %v), want only the user's entry", data, err)
	}
}

func TestPlanRefusesWhatCannotBeDone(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	tests := []struct {
		name, id string
		add      bool
		wantErr  string
	}{
		{"unknown agent", "acme.missing", true, "not installed"},
		{"tool not on this Mac", "builtin.cursor", true, "not on this Mac"},
		{"no hooks to remove", "builtin.claude", false, "no hooks"},
	}
	saveRecord(t, service)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := service.PlanAgentHooks(tt.id, tt.add); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPlanNeedsAnInstall(t *testing.T) {
	service := newService(t)
	installBuiltinAgents(t, service)
	mkdir(t, filepath.Join(service.Paths.Home, ".cursor"))

	_, _, err := service.PlanAgentHooks("builtin.cursor", true)

	if err == nil || !strings.Contains(err.Error(), "agent-screensaver install") {
		t.Fatalf("error = %v, want a hint to run install", err)
	}
}

func TestPlanOfOutsideConverterAgentHasWarnings(t *testing.T) {
	service := newService(t)
	saveRecord(t, service)
	dir := agentFolder(t, "~/.acme/hooks.json")
	writeFile(t, filepath.Join(dir, "extension.json"), `{"id":"acme.agent","name":"Acme","version":"1.0.0","type":"agent","api":1,
		"agent":{"source":"acme","mark":"mark.svg","hooks":{"converter":{"run":"bin/convert"}},
		"install":{"hooks":{"file":"~/.acme/hooks.json","format":"json","shape":"flat","events":["stop"]}}}}`)
	mkdir(t, filepath.Join(dir, "bin"))
	writeFile(t, filepath.Join(dir, "bin", "convert"), "#!/bin/sh\ncat\n")
	if err := os.Chmod(filepath.Join(dir, "bin", "convert"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := service.InstallExtension(dir); err != nil {
		t.Fatal(err)
	}

	plan, _, err := service.PlanAgentHooks("acme.agent", true)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := service.Connections(0)

	warnings := TrustWarnings(plan)
	if len(warnings) != 1 || !strings.Contains(warnings[0], "runs its own program bin/convert") {
		t.Fatalf("warnings = %q, want the converter line", warnings)
	}
	if acme := byConnectionID(t, list, "acme.agent"); !slices.Equal(acme.Warnings, warnings) {
		t.Errorf("connection warnings = %q, want %q", acme.Warnings, warnings)
	}
	if claudeLike := TrustWarnings(planOf(t, service)); len(claudeLike) != 0 {
		t.Errorf("a built-in agent has warnings %q", claudeLike)
	}
}

// planOf is the add plan of the built-in Cursor agent, which has no warnings.
func planOf(t *testing.T, service Service) []install.PlannedStep {
	t.Helper()
	installBuiltinAgents(t, service)
	mkdir(t, filepath.Join(service.Paths.Home, ".cursor"))
	plan, _, err := service.PlanAgentHooks("builtin.cursor", true)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
