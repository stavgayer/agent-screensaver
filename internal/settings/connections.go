package settings

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/install"
	"agent-screensaver/internal/statefile"
	"agent-screensaver/internal/view"
)

// ConnectionState is what the Connections tab and `agent list` say about
// one agent extension (spec §7d "The Connections tab").
type ConnectionState string

const (
	StateNotInstalled ConnectionState = "not_installed" // the detect path does not exist
	StateNoHooks      ConnectionState = "no_hooks"      // found, and the record has no hooks for it
	StateActive       ConnectionState = "active"        // on, hooked, an event in the last ActiveWindowMs
	StateIdle         ConnectionState = "idle"          // on, hooked, no recent event or none yet
	StateOff          ConnectionState = "off"           // the id is in agents_off
)

// ActiveWindowMs is spec §7d's rule: an event in the last 10 minutes means
// the agent is in use. It is the same number as the approval-to-unknown rule.
const ActiveWindowMs = 10 * 60 * 1000

// ErrPlanChanged is returned when the plan to apply is not the plan the
// user saw: a file it touches changed in between.
var ErrPlanChanged = errors.New("the plan changed since it was shown; look at the new plan")

// Connection is one row of the Connections tab.
type Connection struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Source  string          `json:"source"`
	Mark    string          `json:"mark"` // file URL of the mark inside the data folder, as the cards use it
	Version string          `json:"version"`
	Builtin bool            `json:"builtin"`
	State   ConnectionState `json:"state"`
	// LastEventMs is the time of the source's last event; nil: none yet.
	LastEventMs *int64 `json:"last_event_ms"`
	// Found is true when the tool is on this Mac, HooksAdded when the
	// install record lists hooks for the agent. Off rows need both to
	// offer the right hook button.
	Found      bool `json:"found"`
	HooksAdded bool `json:"hooks_added"`
	// HooksChanged: the record says added, the file no longer holds our entries.
	HooksChanged bool `json:"hooks_changed"`
	// Warnings are the converter and plugin file lines of the add plan; the
	// confirm button then says "I trust <name>, add hooks".
	Warnings []string `json:"warnings"`
}

// ConnectionStateFor is pure: the order is off, not installed, no hooks,
// then active or idle.
func ConnectionStateFor(found, hooksAdded, off bool, lastEventMs *int64, nowMs int64) ConnectionState {
	switch {
	case off:
		return StateOff
	case !found:
		return StateNotInstalled
	case !hooksAdded:
		return StateNoHooks
	case lastEventMs != nil && nowMs-*lastEventMs < ActiveWindowMs:
		return StateActive
	default:
		return StateIdle
	}
}

// Connections lists every installed agent extension: built-ins first, then
// by name.
func (s Service) Connections(nowMs int64) ([]Connection, error) {
	loaded, _ := agents.Load(s.Paths.ExtensionsDir) // a broken agent is left out, as everywhere
	record, err := s.record(loaded)
	if err != nil {
		return nil, err
	}
	cfg := config.Load(s.Paths.ConfigFile)
	sources, _ := statefile.ReadSources(s.Paths.StateFile) // a damaged state file: no events known
	marks := view.AgentViews(loaded, s.Paths.MarksDir)
	list := make([]Connection, 0, len(loaded))
	for _, agent := range loaded {
		entry, added := record.Agent(agent.ID)
		var last *int64
		if at := sources[agent.Source].LastEventMs; at > 0 {
			last = &at
		}
		found := install.Detected(s.Paths.Home, agent)
		connection := Connection{
			ID: agent.ID, Name: marks[agent.Source].Name, Source: agent.Source, Mark: marks[agent.Source].MarkURL,
			Version: agent.Version, Builtin: agent.Builtin,
			State:        ConnectionStateFor(found, added, cfg.AgentOff(agent.ID), last, nowMs),
			LastEventMs:  last,
			Found:        found,
			HooksAdded:   added,
			HooksChanged: added && !install.HooksInPlace(s.Paths.Home, entry),
			Warnings:     []string{},
		}
		if !agent.Builtin {
			if plan, err := install.PlanAgent(s.Paths.Home, s.bin(record), agent, record); err == nil {
				connection.Warnings = TrustWarnings(plan)
			}
		}
		list = append(list, connection)
	}
	slices.SortStableFunc(list, func(a, b Connection) int {
		return cmp.Or(boolFirst(a.Builtin, b.Builtin), cmp.Compare(a.Name, b.Name))
	})
	return list, nil
}

func boolFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

// TrustWarnings are the warning lines of an add plan that need the user's
// trust: an outside agent's own programs and plugin files. The first line,
// "Add hooks for X (from <id> <version>)", is a warning for every outside
// agent and is not one of them.
func TrustWarnings(plan []install.PlannedStep) []string {
	warnings := []string{}
	for index, step := range plan {
		if index > 0 && step.Warning {
			warnings = append(warnings, step.Description)
		}
	}
	return warnings
}

// SetAgentOn turns an agent on or off. Off keeps its hooks; it only goes
// silent. Turning on an id that is no longer installed is allowed, so a
// stale entry can always be cleared.
func (s Service) SetAgentOn(id string, on bool) error {
	if !on {
		loaded, _ := agents.Load(s.Paths.ExtensionsDir)
		if _, ok := agents.ByID(loaded, id); !ok {
			return fmt.Errorf("agent %q is not installed", id)
		}
	}
	return s.update(func(cfg *config.Config) error {
		cfg.AgentsOff = slices.DeleteFunc(cfg.AgentsOff, func(off string) bool { return off == id })
		if !on {
			cfg.AgentsOff = append(cfg.AgentsOff, id)
		}
		if len(cfg.AgentsOff) == 0 {
			cfg.AgentsOff = nil
		}
		return nil
	})
}

// PlanAgentHooks lists what adding (add) or removing one agent's hooks
// will do, and the hash of that plan and of the files it touches.
func (s Service) PlanAgentHooks(id string, add bool) (plan []install.PlannedStep, hash string, err error) {
	prepared, err := s.prepareHooks(id, add)
	if err != nil {
		return nil, "", err
	}
	return prepared.plan, prepared.hash, nil
}

// ApplyAgentHooks recomputes the plan and applies it only when its hash is
// the one the user saw; otherwise it returns ErrPlanChanged and changes
// nothing.
func (s Service) ApplyAgentHooks(id string, add bool, hash string) error {
	prepared, err := s.prepareHooks(id, add)
	if err != nil {
		return err
	}
	if prepared.hash != hash {
		return ErrPlanChanged
	}
	if add {
		return install.AddAgentHooks(s.Paths.Home, prepared.bin, prepared.agent, &prepared.record)
	}
	return install.RemoveAgentHooks(s.Paths.Home, prepared.agent, &prepared.record)
}

// preparedHooks is one hook change as planned, with what applying it needs.
type preparedHooks struct {
	plan   []install.PlannedStep
	hash   string
	agent  agents.Agent
	record install.Record
	bin    string
}

func (s Service) prepareHooks(id string, add bool) (preparedHooks, error) {
	loaded, _ := agents.Load(s.Paths.ExtensionsDir)
	agent, ok := agents.ByID(loaded, id)
	if !ok {
		return preparedHooks{}, fmt.Errorf("agent %q is not installed", id)
	}
	record, err := install.LoadRecord(s.Paths.InstallRecord, loaded)
	if err != nil {
		return preparedHooks{}, fmt.Errorf("%w; run agent-screensaver install first", err)
	}
	prepared := preparedHooks{agent: agent, record: record, bin: s.bin(record)}
	entry, added := record.Agent(id)
	switch {
	case add && !install.Detected(s.Paths.Home, agent):
		return preparedHooks{}, fmt.Errorf("%s is not on this Mac (no %s)", agent.Name, agent.Install.Detect)
	case add:
		prepared.plan, err = install.PlanAgent(s.Paths.Home, prepared.bin, agent, record)
		if err != nil {
			return preparedHooks{}, err
		}
	case !added:
		return preparedHooks{}, fmt.Errorf("%s has no hooks to remove", agent.Name)
	default:
		prepared.plan = install.PlanRemoveAgent(s.Paths.Home, entry)
	}
	prepared.hash, err = planHash(id, add, prepared.plan, entry, install.TouchedFiles(s.Paths.Home, agent, entry))
	return prepared, err
}

// planHash covers the plan lines, the agent's record entry and the bytes of
// every file the change touches, so any change in between is seen.
func planHash(id string, add bool, plan []install.PlannedStep, entry install.AgentRecord, files []string) (string, error) {
	type fileState struct {
		Path, SHA256 string
	}
	contents := make([]fileState, 0, len(files))
	for _, path := range files {
		state := fileState{Path: path, SHA256: "missing"}
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			sum := sha256.Sum256(data)
			state.SHA256 = hex.EncodeToString(sum[:])
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		contents = append(contents, state)
	}
	data, err := json.Marshal(struct {
		ID    string
		Add   bool
		Plan  []install.PlannedStep
		Entry install.AgentRecord
		Files []fileState
	}{id, add, plan, entry, contents})
	if err != nil {
		return "", fmt.Errorf("encode plan: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// record is the install record, or an empty one when nothing is installed.
func (s Service) record(loaded []agents.Agent) (install.Record, error) {
	if _, err := os.Stat(s.Paths.InstallRecord); errors.Is(err, fs.ErrNotExist) {
		return install.Record{}, nil
	}
	return install.LoadRecord(s.Paths.InstallRecord, loaded)
}

// bin is the binary our hook commands run: the record's, else where
// install puts it.
func (s Service) bin(record install.Record) string {
	if record.Bin != "" {
		return record.Bin
	}
	return install.BinPath(s.Paths.Home)
}
