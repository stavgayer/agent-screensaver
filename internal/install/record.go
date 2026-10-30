package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"agent-screensaver/internal/agents"
)

// RecordVersion is the install record format this build writes.
const RecordVersion = 2

// Record says what Apply did, so Uninstall can reverse exactly that.
type Record struct {
	Version  int           `json:"version"`
	Bin      string        `json:"bin"`
	Agents   []AgentRecord `json:"agents"`
	Services []string      `json:"services"` // service labels
	Saver    string        `json:"saver"`
	// Launcher is the launcher app install made or replaced. Empty when an
	// app that is not ours was in the way. Uninstall still checks the app's
	// bundle id and takes the path from the home folder, not from here.
	Launcher string `json:"launcher,omitempty"`
}

// AgentRecord is what install wrote for one agent. Uninstall uses only
// this, never the manifest, which may have changed or be gone.
type AgentRecord struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	// File is the hook file as the recipe names it ("~/..."), "" when the
	// agent has no hook file.
	File   string   `json:"file,omitempty"`
	Shape  string   `json:"shape,omitempty"`
	Events []string `json:"events,omitempty"`
	// Commands are the exact hook commands written.
	Commands    []string `json:"commands,omitempty"`
	CreatedFile bool     `json:"created_file,omitempty"`
	// NewFileKeys are the keys written only because the file was new.
	NewFileKeys       []string     `json:"new_file_keys,omitempty"`
	StatuslineKey     string       `json:"statusline_key,omitempty"`
	StatuslineCommand string       `json:"statusline_command,omitempty"`
	PreviousCommand   string       `json:"previous_command,omitempty"`
	Files             []CopiedFile `json:"files,omitempty"`
}

// CopiedFile is a file install copied, by its "~/" path and SHA-256.
type CopiedFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// agentRecord finds the entry of an agent id.
func (r Record) agentRecord(id string) (AgentRecord, bool) {
	index := slices.IndexFunc(r.Agents, func(entry AgentRecord) bool { return entry.ID == id })
	if index < 0 {
		return AgentRecord{}, false
	}
	return r.Agents[index], true
}

// setAgent replaces or appends the entry of an agent.
func (r *Record) setAgent(entry AgentRecord) {
	r.dropAgent(entry.ID)
	r.Agents = append(r.Agents, entry)
}

func (r *Record) dropAgent(id string) {
	r.Agents = slices.DeleteFunc(r.Agents, func(entry AgentRecord) bool { return entry.ID == id })
}

// recordV1 is the record written before agent extensions: agent names and
// one saved status line command.
type recordV1 struct {
	Bin                string   `json:"bin"`
	Agents             []string `json:"agents"`
	PreviousStatusline string   `json:"previous_statusline"`
	Services           []string `json:"services"`
	Saver              string   `json:"saver"`
}

// LoadRecord reads the install record. A version 1 record is read as the
// entries the built-in agents in known had then: the same events and files,
// the command `'<bin>' hook --tool <source>`, and `'<bin>' statusline` for
// the status line.
func LoadRecord(path string, known []agents.Agent) (Record, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, fmt.Errorf("agent-screensaver is not installed (no record at %s)", path)
	}
	if err != nil {
		return Record{}, fmt.Errorf("read install record: %w", err)
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return Record{}, fmt.Errorf("install record %s is damaged: %w", path, err)
	}
	if probe.Version >= RecordVersion {
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return Record{}, fmt.Errorf("install record %s is damaged: %w", path, err)
		}
		return record, nil
	}
	var old recordV1
	if err := json.Unmarshal(data, &old); err != nil {
		return Record{}, fmt.Errorf("install record %s is damaged: %w", path, err)
	}
	return upgradeRecord(old, known), nil
}

func upgradeRecord(old recordV1, known []agents.Agent) Record {
	record := Record{Version: RecordVersion, Bin: old.Bin, Services: old.Services, Saver: old.Saver}
	for _, source := range old.Agents {
		agent, ok := agents.BySource(known, source)
		if !ok || !agent.Builtin || agent.Install.Hooks == nil {
			continue
		}
		hooks := agent.Install.Hooks
		entry := AgentRecord{
			ID: agent.ID, Source: source, File: hooks.File, Shape: hooks.Shape,
			Events:   hooks.Events,
			Commands: []string{hookCommand(old.Bin, source, "", "")},
		}
		if agent.Install.Statusline != nil {
			entry.StatuslineKey = agent.Install.Statusline.Key
			entry.StatuslineCommand = shellQuote(old.Bin) + " statusline"
			entry.PreviousCommand = old.PreviousStatusline
		}
		record.Agents = append(record.Agents, entry)
	}
	return record
}

// SaveRecord writes the record.
func SaveRecord(path string, record Record) error {
	record.Version = RecordVersion
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install record: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'), 0o644)
}
