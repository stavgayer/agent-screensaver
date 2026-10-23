package agents

import (
	"errors"
	"fmt"
	"slices"

	"agent-screensaver/internal/extensions"
)

// Load returns every valid agent extension in the extensions folder. An
// invalid one is skipped, and the error says why, for the log. Built-in
// agents come first, so when two agents claim one source (only possible
// when someone copies folders by hand) the built-in one wins.
func Load(extensionsDir string) ([]Agent, error) {
	installed, scanProblems := extensions.ScanWithProblems(extensionsDir)
	return FromInstalled(installed, scanProblems)
}

// FromInstalled picks the agents out of the extensions found by a scan.
// earlier holds the scan's own problems, which are passed on.
func FromInstalled(installed []extensions.Installed, earlier error) ([]Agent, error) {
	problems := []error{earlier}
	var found []Agent
	for _, extension := range installed {
		if extension.Manifest.Type != extensions.TypeAgent {
			continue
		}
		agent, err := FromManifest(extension.Manifest, extension.Dir)
		if err != nil {
			problems = append(problems, fmt.Errorf("agent %s: %w", extension.Manifest.ID, err))
			continue
		}
		found = append(found, agent)
	}
	slices.SortStableFunc(found, func(a, b Agent) int { return boolOrder(b.Builtin) - boolOrder(a.Builtin) })
	unique := found[:0]
	for _, agent := range found {
		if _, taken := BySource(unique, agent.Source); taken {
			problems = append(problems, fmt.Errorf("agent %s: source %q is already used", agent.ID, agent.Source))
			continue
		}
		unique = append(unique, agent)
	}
	return unique, errors.Join(problems...)
}

func boolOrder(value bool) int {
	if value {
		return 1
	}
	return 0
}

// BySource finds the agent that writes events with this source.
func BySource(agents []Agent, source string) (Agent, bool) {
	for _, agent := range agents {
		if agent.Source == source {
			return agent, true
		}
	}
	return Agent{}, false
}

// ByID finds an agent by its extension id.
func ByID(agents []Agent, id string) (Agent, bool) {
	for _, agent := range agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return Agent{}, false
}
