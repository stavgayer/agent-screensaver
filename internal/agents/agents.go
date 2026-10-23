// Package agents is the engine behind agent extensions (docs/agents.md).
// An agent extension is data: how a coding agent's hook payloads become
// events, how its hooks are added, and where the service reads extra
// details. Claude Code and Codex are built-in agent extensions; no Go code
// here names them. Adding an agent needs no Go change: the user installs an
// extension folder.
package agents

import "path/filepath"

// Agent is one valid agent extension: data that teaches the service one
// coding agent (docs/agents.md).
type Agent struct {
	ID, Name, Version string
	// Source is the event source, the --tool value and the card's source.
	Source string
	// Dir is the extension folder and Mark the mark file's path inside it.
	Dir, Mark  string
	Hooks      Mapping
	Statusline *Mapping
	Install    Recipe
	Details    *Details
	// Limits is the id of a Go limits provider, "" for none.
	Limits  string
	Builtin bool
}

// MarkPath is the full path of the agent's mark file.
func (a Agent) MarkPath() string { return filepath.Join(a.Dir, a.Mark) }
