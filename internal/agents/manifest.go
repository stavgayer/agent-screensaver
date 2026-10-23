package agents

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/extensions"
)

// The Go providers a manifest may name. They read what the mapping
// language cannot describe: a JSON-RPC session, or a value that needs the
// window sizes the service learned.
const (
	// LimitsCodexAppServer reads limits from the `codex app-server` program.
	LimitsCodexAppServer = "codex-app-server"
	// LimitsClaudeUsageEstimate keeps limits live between exact readings,
	// from the tool's own usage files and transcripts.
	LimitsClaudeUsageEstimate = "claude-usage-estimate"
	// DetailsClaudeTranscriptContext works out context use from a Claude
	// transcript and the window sizes learned from the status line.
	DetailsClaudeTranscriptContext = "claude-transcript-context"
)

var (
	limitsProviders  = []string{LimitsCodexAppServer, LimitsClaudeUsageEstimate}
	detailsProviders = []string{DetailsClaudeTranscriptContext}
)

// agentManifest is the "agent" object of extension.json.
type agentManifest struct {
	Source     string          `json:"source"`
	Mark       string          `json:"mark"`
	Hooks      json.RawMessage `json:"hooks"`
	Statusline json.RawMessage `json:"statusline,omitempty"`
	Install    *Recipe         `json:"install"`
	Details    *Details        `json:"details,omitempty"`
	Limits     *struct {
		Provider string `json:"provider"`
	} `json:"limits,omitempty"`
}

// Validate checks every rule of an agent manifest: the common extension
// rules and the agent body. dir is the extension folder.
func Validate(m extensions.Manifest, dir string) error {
	if err := extensions.Validate(m, dir); err != nil {
		return err
	}
	_, err := FromManifest(m, dir)
	return err
}

// FromManifest turns a manifest that passed extensions.Validate into an
// Agent, checking the agent body on the way.
func FromManifest(m extensions.Manifest, dir string) (Agent, error) {
	if m.Type != extensions.TypeAgent {
		return Agent{}, fmt.Errorf("%s is a %s extension, not an agent", m.ID, m.Type)
	}
	var body agentManifest
	if err := decodeStrict(m.Agent, &body); err != nil {
		return Agent{}, fmt.Errorf("agent: %w", err)
	}
	agent := Agent{
		ID: m.ID, Name: m.Name, Version: m.Version, Source: body.Source, Dir: dir,
		Mark: filepath.Clean(body.Mark), Details: body.Details,
		Builtin: strings.HasPrefix(m.ID, "builtin."),
	}
	var err error
	if agent.Hooks, err = parseMapping(body.Hooks, body.Source, dir, hookMappingKind); err != nil {
		return Agent{}, fmt.Errorf("agent.hooks: %w", err)
	}
	if len(body.Statusline) > 0 {
		statusline, err := parseMapping(body.Statusline, body.Source, dir, statuslineMappingKind)
		if err != nil {
			return Agent{}, fmt.Errorf("agent.statusline: %w", err)
		}
		agent.Statusline = &statusline
	}
	if body.Install == nil {
		return Agent{}, errors.New("agent.install is required")
	}
	if err := body.Install.check(agent); err != nil {
		return Agent{}, fmt.Errorf("agent.install: %w", err)
	}
	agent.Install = *body.Install
	if body.Details != nil {
		if err := body.Details.check(dir); err != nil {
			return Agent{}, fmt.Errorf("agent.details: %w", err)
		}
	}
	if body.Limits != nil {
		if !slices.Contains(limitsProviders, body.Limits.Provider) {
			return Agent{}, fmt.Errorf("agent.limits: unknown limits provider %q (known: %s)",
				body.Limits.Provider, strings.Join(limitsProviders, ", "))
		}
		agent.Limits = body.Limits.Provider
	}
	return agent, nil
}

// decodeStrict decodes JSON and refuses unknown fields, so a typo in a
// manifest is an error and not a silently ignored rule.
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("extra data after the JSON value")
	}
	return nil
}

// Recipe says how to add and remove our hooks for one agent. It is data;
// package install applies it and checks its safety rules.
type Recipe struct {
	// Detect is a path that exists when the tool is on this Mac.
	Detect     string            `json:"detect,omitempty"`
	Hooks      *HookRecipe       `json:"hooks,omitempty"`
	Statusline *StatuslineRecipe `json:"statusline,omitempty"`
	Files      []FileCopy        `json:"files,omitempty"`
	Notes      []string          `json:"notes,omitempty"`
}

// HookRecipe is the hook config part of a recipe. Group, Handler and
// NewFile are kept raw, so their keys keep the manifest's order.
type HookRecipe struct {
	File    string          `json:"file"`
	Format  string          `json:"format"`
	Shape   string          `json:"shape"`
	Events  []string        `json:"events"`
	Group   json.RawMessage `json:"group,omitempty"`
	Handler json.RawMessage `json:"handler,omitempty"`
	NewFile json.RawMessage `json:"new_file,omitempty"`
}

// StatuslineRecipe names the key whose command becomes our status line.
type StatuslineRecipe struct {
	Key string `json:"key"`
}

// FileCopy is a file copied from the extension folder to the user's home.
type FileCopy struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// check holds the shape rules of a recipe. The safety rules (where it may
// write) need the home folder and live in package install (CheckRecipe).
func (r Recipe) check(agent Agent) error {
	if r.Hooks == nil && len(r.Files) == 0 {
		return errors.New("a recipe needs hooks or files")
	}
	if r.Hooks != nil && len(r.Hooks.Events) == 0 {
		return errors.New("hooks.events must name at least one event")
	}
	if r.Statusline != nil {
		if r.Hooks == nil || r.Statusline.Key == "" {
			return errors.New("statusline needs a key and a hooks file to hold it")
		}
		if agent.Statusline == nil {
			return errors.New("install.statusline needs an agent.statusline mapping")
		}
	}
	for _, file := range r.Files {
		if !filepath.IsLocal(file.From) {
			return fmt.Errorf("files: %q must be a path inside the extension folder", file.From)
		}
	}
	return nil
}

// Details says where the service reads extra details for a card.
type Details struct {
	// ReadOn lists the event kinds after which the details are read.
	ReadOn         []event.Kind   `json:"read_on,omitempty"`
	Name           []DetailSource `json:"name,omitempty"`
	ContextPercent []DetailSource `json:"context_percent,omitempty"`
	Model          []DetailSource `json:"model,omitempty"`
}

// DefaultReadOn is when details are read if a manifest does not say.
var DefaultReadOn = []event.Kind{event.KindStarted, event.KindFinished, event.KindEnded}

// ReadsOn reports whether details are read after an event of this kind.
func (d *Details) ReadsOn(kind event.Kind) bool {
	if d == nil || len(d.ReadOn) == 0 {
		return slices.Contains(DefaultReadOn, kind)
	}
	return slices.Contains(d.ReadOn, kind)
}

// Tail sizes of a JSON lines file, in KiB.
const (
	DefaultTailKB = 256
	MaxTailKB     = 1024
)

// DetailSource is one place a detail can come from. Exactly one of JSONL,
// JSON, Converter and Provider is set.
type DetailSource struct {
	JSONL     string             `json:"jsonl,omitempty"`
	JSON      string             `json:"json,omitempty"`
	TailKB    int                `json:"tail_kb,omitempty"`
	Last      Conditions         `json:"last,omitempty"`
	Value     *Value             `json:"value,omitempty"`
	Converter *converterManifest `json:"converter,omitempty"`
	Provider  string             `json:"provider,omitempty"`

	converter *Converter
}

// DetailsConverter is the converter of a details source, nil for others.
func (s DetailSource) DetailsConverter() *Converter { return s.converter }

// TailBytes is how much of the end of a JSON lines file is read.
func (s DetailSource) TailBytes() int64 {
	if s.TailKB == 0 {
		return DefaultTailKB << 10
	}
	return int64(s.TailKB) << 10
}

func (d *Details) check(dir string) error {
	for _, kind := range d.ReadOn {
		if !knownKinds[kind] || kind == kindIgnore {
			return fmt.Errorf("read_on: unknown kind %q", kind)
		}
	}
	for field, sources := range map[string][]DetailSource{"name": d.Name, "context_percent": d.ContextPercent, "model": d.Model} {
		for index := range sources {
			if err := sources[index].check(dir); err != nil {
				return fmt.Errorf("%s %d: %w", field, index, err)
			}
		}
	}
	return nil
}

func (s *DetailSource) check(dir string) error {
	set := 0
	for _, filled := range []bool{s.JSONL != "", s.JSON != "", s.Converter != nil, s.Provider != ""} {
		if filled {
			set++
		}
	}
	if set != 1 {
		return errors.New("a details source needs exactly one of jsonl, json, converter and provider")
	}
	if s.TailKB < 0 || s.TailKB > MaxTailKB {
		return fmt.Errorf("tail_kb is %d, allowed 1 to %d", s.TailKB, MaxTailKB)
	}
	switch {
	case s.Provider != "":
		if !slices.Contains(detailsProviders, s.Provider) {
			return fmt.Errorf("unknown details provider %q", s.Provider)
		}
	case s.Converter != nil:
		converter, err := parseConverter(*s.Converter, dir, MaxDetailsConverterTimeout)
		if err != nil {
			return err
		}
		if s.Converter.TimeoutMs == 0 {
			converter.Timeout = DefaultDetailsConverterTimeout
		}
		s.converter = &converter
	default:
		if s.Value == nil {
			return errors.New("a file source needs a value")
		}
		if !placeholderPath(s.JSONL + s.JSON) {
			return fmt.Errorf("path %q must start with ~/, {transcript} or {cwd}", s.JSONL+s.JSON)
		}
	}
	return nil
}

func placeholderPath(path string) bool {
	for _, prefix := range []string{"~/", "{transcript}", "{cwd}"} {
		if strings.HasPrefix(path, prefix) {
			return !strings.Contains(path, "..")
		}
	}
	return false
}
