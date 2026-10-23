package install

import (
	"os"
	"slices"
	"strings"

	"agent-screensaver/internal/agents"
)

// The helpers in this file answer the settings window's Connections tab and
// the `agent` CLI commands: is the tool here, are our entries still there,
// and what would removing one agent's hooks do.

// Detected reports whether the agent's tool is on this Mac: its detect path
// exists. An agent without a detect path counts as found.
func Detected(home string, a agents.Agent) bool {
	return detected(home, a.Install.Detect)
}

// BinPath is where install puts the binary for home, the one hook
// commands run.
func BinPath(home string) string {
	return locationsFor(home).bin
}

// Agent finds the entry of an agent id.
func (r Record) Agent(id string) (AgentRecord, bool) {
	return r.agentRecord(id)
}

// HooksInPlace reports whether every entry the record lists for one agent
// is still there: each command and the status line command in its hook
// file, and each copied file. False means the user changed the file after
// we wrote it.
func HooksInPlace(home string, entry AgentRecord) bool {
	if entry.File != "" {
		path, err := expandHome(home, entry.File)
		if err != nil {
			return false
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		for _, command := range slices.Concat(entry.Commands, []string{entry.StatuslineCommand}) {
			if command != "" && !strings.Contains(string(data), jsonEscaped(command)) {
				return false
			}
		}
	}
	for _, file := range entry.Files {
		path, err := expandHome(home, file.Path)
		if err != nil {
			return false
		}
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

// PlanRemoveAgent lists what RemoveAgentHooks does for one record entry.
func PlanRemoveAgent(home string, entry AgentRecord) []PlannedStep {
	var steps []PlannedStep
	if entry.File != "" {
		path, _ := expandHome(home, entry.File)
		steps = append(steps, PlannedStep{Name: "hooks-" + entry.Source, Description: "Remove our " + entry.Source + " hooks from " + path})
	}
	for _, file := range entry.Files {
		steps = append(steps, PlannedStep{Name: "hooks-" + entry.Source, Description: "Remove our copied file " + file.Path})
	}
	return steps
}

// TouchedFiles are the files that adding a's hooks or removing entry's
// would read or change, as full paths. A plan that shows them is stale once
// any of them changes. Paths that fail the safety rules are left out; the
// add or remove itself refuses them.
func TouchedFiles(home string, a agents.Agent, entry AgentRecord) []string {
	var recipePaths []string
	if a.Install.Hooks != nil {
		recipePaths = append(recipePaths, a.Install.Hooks.File)
	}
	for _, file := range a.Install.Files {
		recipePaths = append(recipePaths, file.To)
	}
	if entry.File != "" {
		recipePaths = append(recipePaths, entry.File)
	}
	for _, file := range entry.Files {
		recipePaths = append(recipePaths, file.Path)
	}
	var full []string
	for _, recipePath := range recipePaths {
		if path, err := expandHome(home, recipePath); err == nil {
			full = append(full, path)
		}
	}
	return full
}
