package install

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/extensions"
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

// HooksState says whether what the record lists for one agent is still in
// the tool's files.
type HooksState string

const (
	// HooksOK: every recorded command sits under its event, the status
	// line is ours and every copied file is there.
	HooksOK HooksState = "ok"
	// HooksChanged: someone removed or moved our entries after we wrote them.
	HooksChanged HooksState = "changed"
	// HooksOwnStatusline: the hooks are in place, but the status line now
	// runs a command of the user's.
	HooksOwnStatusline HooksState = "own_statusline"
)

// CheckHooks compares one record entry with the files on disk. Each event
// the record names must run one of our commands: the command with
// "--event <that event>" when the record has one, else a command without
// --event. A command moved to another event therefore counts as changed.
func CheckHooks(home string, entry AgentRecord) HooksState {
	state := HooksOK
	if entry.File != "" {
		state = checkHookFile(home, entry)
		if state == HooksChanged {
			return state
		}
	}
	for _, file := range entry.Files {
		path, err := expandHome(home, file.Path)
		if err != nil {
			return HooksChanged
		}
		if _, err := os.Stat(path); err != nil {
			return HooksChanged
		}
	}
	return state
}

func checkHookFile(home string, entry AgentRecord) HooksState {
	shape, ok := hookShapes[entry.Shape]
	path, err := expandHome(home, entry.File)
	if !ok || err != nil {
		return HooksChanged
	}
	config, original, err := readConfig(configFormats[formatJSON], path)
	if err != nil || original == nil {
		return HooksChanged
	}
	if len(entry.Commands) > 0 {
		hooksObject, err := objectOf(config, hooksKey)
		if err != nil {
			return HooksChanged
		}
		for _, eventName := range entry.Events {
			list, err := arrayOf(hooksObject, eventName)
			if err != nil || !slices.ContainsFunc(commandsFor(entry.Commands, eventName), func(command string) bool {
				return shape.runs(list, command)
			}) {
				return HooksChanged
			}
		}
	}
	if entry.StatuslineKey == "" {
		return HooksOK
	}
	line, err := objectOf(config, entry.StatuslineKey)
	if err != nil {
		return HooksChanged
	}
	switch current, _ := stringValue(line, "command"); current {
	case entry.StatuslineCommand:
		return HooksOK
	case "":
		return HooksChanged
	default:
		return HooksOwnStatusline
	}
}

// commandsFor are the recorded commands that belong under one event: those
// with "--event <event>", or, when there is none, those without --event.
func commandsFor(commands []string, eventName string) []string {
	named := func(command string) bool {
		return strings.HasSuffix(command, " --event "+eventName) || strings.Contains(command, " --event "+eventName+" ")
	}
	if slices.ContainsFunc(commands, named) {
		return slices.DeleteFunc(slices.Clone(commands), func(command string) bool { return !named(command) })
	}
	return slices.DeleteFunc(slices.Clone(commands), func(command string) bool { return strings.Contains(command, " --event ") })
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

// SourceFiles are the agent's own files that decide what adding its hooks
// does or runs: its manifest, its converter programs and the plugin files
// it copies.
func SourceFiles(a agents.Agent) []string {
	files := []string{filepath.Join(a.Dir, extensions.ManifestFile)}
	files = append(files, Programs(a)...)
	for _, file := range a.Install.Files {
		files = append(files, filepath.Join(a.Dir, filepath.Clean(file.From)))
	}
	return files
}

// Programs are the agent's own converter programs, as full paths: the hook
// and status line converters and the details converters.
func Programs(a agents.Agent) []string {
	var programs []string
	add := func(converter *agents.Converter) {
		if converter != nil && !slices.Contains(programs, converter.Run) {
			programs = append(programs, converter.Run)
		}
	}
	add(a.Hooks.Converter)
	if a.Statusline != nil {
		add(a.Statusline.Converter)
	}
	if a.Details != nil {
		for _, sources := range [][]agents.DetailSource{a.Details.Name, a.Details.ContextPercent, a.Details.Model} {
			for _, source := range sources {
				add(source.DetailsConverter())
			}
		}
	}
	return programs
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
