// Package extensions defines the extension manifest (extension.json) and
// installs, lists and removes extensions on disk. Built-in backgrounds,
// components, layouts and themes use the same format as outside ones.
package extensions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// APIVersion is the only manifest API version this build understands.
const APIVersion = 1

// ManifestFile is the file name every extension folder must contain.
const ManifestFile = "extension.json"

const builtinPrefix = "builtin."

// ErrBuiltin is returned when an install or remove touches a reserved
// built-in id. Outside authors must not shadow or delete built-in parts.
var ErrBuiltin = errors.New("ids starting with \"builtin.\" are reserved")

var idPattern = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)+$`)

// Type is the kind of an extension.
type Type string

const (
	TypeBackground Type = "background"
	TypeComponent  Type = "component"
	TypeLayout     Type = "layout"
	TypeTheme      Type = "theme"
	// TypeAgent teaches the service one coding agent (docs/agents.md). Its
	// body is checked here only for the parts every agent shares: the
	// source name and the mark. Package agents checks the rest.
	TypeAgent Type = "agent"
)

// OptionKind is the kind of value an option holds.
type OptionKind string

const (
	OptionString  OptionKind = "string"
	OptionNumber  OptionKind = "number"
	OptionBoolean OptionKind = "boolean"
	OptionColor   OptionKind = "color"
	OptionEnum    OptionKind = "enum"
	OptionFile    OptionKind = "file"
)

// OptionSpec describes one setting of an extension. Settings screens are
// generated from these specs, so new extensions need no settings code.
type OptionSpec struct {
	Kind    OptionKind      `json:"type"`
	Default json.RawMessage `json:"default"`
	Min     *float64        `json:"min,omitempty"`
	Max     *float64        `json:"max,omitempty"`
	Values  []string        `json:"values,omitempty"`
	Label   string          `json:"label,omitempty"`
}

// ExtRef points at another extension, with option values for that use.
type ExtRef struct {
	ID      string         `json:"id"`
	Options map[string]any `json:"options,omitempty"`
}

// ThemeSpec is the body of a theme: a background, a layout and style values.
type ThemeSpec struct {
	Background ExtRef         `json:"background"`
	Layout     string         `json:"layout"`
	Style      map[string]any `json:"style"`
}

// Manifest is the parsed extension.json.
type Manifest struct {
	ID      string                `json:"id"`
	Name    string                `json:"name"`
	Version string                `json:"version"`
	Type    Type                  `json:"type"`
	API     int                   `json:"api"`
	Entry   string                `json:"entry,omitempty"`
	Options map[string]OptionSpec `json:"options,omitempty"`
	Layout  json.RawMessage       `json:"layout,omitempty"`
	Theme   *ThemeSpec            `json:"theme,omitempty"`
	Agent   json.RawMessage       `json:"agent,omitempty"`
}

// Installed is a valid extension found on disk.
type Installed struct {
	Manifest Manifest
	Dir      string
}

// layoutSpec is the shape of Manifest.Layout: data only, no code.
type layoutSpec struct {
	Modes map[string]map[string][]ExtRef `json:"modes"`
}

var (
	layoutRegions    = map[string]bool{"top-left": true, "top-right": true, "center": true, "bottom": true}
	requiredModes    = []string{"idle", "attention"}
	optionalModes    = []string{"secondary"}
	colorPattern     = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	webEntryTypes    = map[Type]bool{TypeBackground: true, TypeComponent: true}
	knownOptionKinds = map[OptionKind]bool{
		OptionString: true, OptionNumber: true, OptionBoolean: true,
		OptionColor: true, OptionEnum: true, OptionFile: true,
	}
)

// Validate checks a manifest against the rules in docs/extensions.md.
// dir is the extension folder, used to check that the entry file exists.
func Validate(m Manifest, dir string) error {
	if !idPattern.MatchString(m.ID) {
		return fmt.Errorf("id %q must look like \"author.name\" (lowercase letters, digits, dashes, at least one dot)", m.ID)
	}
	if m.API != APIVersion {
		return fmt.Errorf("unsupported api %d, this build supports %d", m.API, APIVersion)
	}
	if err := validateBody(m, dir); err != nil {
		return err
	}
	return validateOptions(m.Options)
}

func validateBody(m Manifest, dir string) error {
	switch m.Type {
	case TypeBackground, TypeComponent:
		return validateEntry(m.Entry, dir)
	case TypeLayout:
		return validateLayout(m.Layout)
	case TypeTheme:
		return validateTheme(m.Theme)
	case TypeAgent:
		return validateAgent(m, dir)
	default:
		return fmt.Errorf("unknown type %q", m.Type)
	}
}

func validateEntry(entry, dir string) error {
	if entry == "" {
		return errors.New("entry is required")
	}
	// IsLocal rejects absolute paths and any path that climbs out of dir.
	if !filepath.IsLocal(entry) {
		return fmt.Errorf("entry %q must be a relative path inside the extension folder", entry)
	}
	info, err := os.Stat(filepath.Join(dir, filepath.Clean(entry)))
	if err != nil {
		return fmt.Errorf("entry file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("entry %q is not a regular file", entry)
	}
	return nil
}

func validateLayout(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("layout is required for a layout extension")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var spec layoutSpec
	if err := decoder.Decode(&spec); err != nil {
		return fmt.Errorf("layout: %w", err)
	}
	for _, mode := range requiredModes {
		if _, ok := spec.Modes[mode]; !ok {
			return fmt.Errorf("layout: missing mode %q", mode)
		}
	}
	for mode, regions := range spec.Modes {
		if !contains(requiredModes, mode) && !contains(optionalModes, mode) {
			return fmt.Errorf("layout: unknown mode %q", mode)
		}
		if err := validateRegions(mode, regions); err != nil {
			return err
		}
	}
	return nil
}

func validateRegions(mode string, regions map[string][]ExtRef) error {
	for region, refs := range regions {
		if !layoutRegions[region] {
			return fmt.Errorf("layout: unknown region %q in mode %q", region, mode)
		}
		for _, ref := range refs {
			if ref.ID == "" {
				return fmt.Errorf("layout: empty component id in %s/%s", mode, region)
			}
		}
	}
	return nil
}

func validateTheme(theme *ThemeSpec) error {
	if theme == nil {
		return errors.New("theme is required for a theme extension")
	}
	if theme.Background.ID == "" || theme.Layout == "" {
		return errors.New("theme needs a background id and a layout id")
	}
	return nil
}

func validateOptions(options map[string]OptionSpec) error {
	names := make([]string, 0, len(options))
	for name := range options {
		names = append(names, name)
	}
	sort.Strings(names) // stable error messages
	for _, name := range names {
		if err := validateOptionDefault(options[name]); err != nil {
			return fmt.Errorf("option %q: %w", name, err)
		}
	}
	return nil
}

func validateOptionDefault(spec OptionSpec) error {
	if !knownOptionKinds[spec.Kind] {
		return fmt.Errorf("unknown type %q", spec.Kind)
	}
	if spec.Kind == OptionEnum && len(spec.Values) == 0 {
		return errors.New("enum needs a non-empty values list")
	}
	if spec.Min != nil && spec.Max != nil && *spec.Min > *spec.Max {
		return errors.New("min is greater than max")
	}
	if len(spec.Default) == 0 {
		return errors.New("default is required")
	}
	var value any
	if err := json.Unmarshal(spec.Default, &value); err != nil {
		return fmt.Errorf("default: %w", err)
	}
	if err := ValidateOption(spec, value); err != nil {
		return fmt.Errorf("default: %w", err)
	}
	return nil
}

// ValidateOption checks a value decoded by encoding/json (float64, string,
// bool) against its spec.
func ValidateOption(spec OptionSpec, value any) error {
	switch spec.Kind {
	case OptionString, OptionFile:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("must be a string, got %T", value)
		}
	case OptionBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("must be true or false, got %T", value)
		}
	case OptionNumber:
		return validateNumber(spec, value)
	case OptionColor:
		text, ok := value.(string)
		if !ok || !colorPattern.MatchString(text) {
			return fmt.Errorf("must be a color like #rrggbb, got %v", value)
		}
	case OptionEnum:
		text, ok := value.(string)
		if !ok || !contains(spec.Values, text) {
			return fmt.Errorf("must be one of %v, got %v", spec.Values, value)
		}
	default:
		return fmt.Errorf("unknown option type %q", spec.Kind)
	}
	return nil
}

func validateNumber(spec OptionSpec, value any) error {
	number, ok := value.(float64)
	if !ok {
		return fmt.Errorf("must be a number, got %T", value)
	}
	if spec.Min != nil && number < *spec.Min {
		return fmt.Errorf("%v is below the minimum %v", number, *spec.Min)
	}
	if spec.Max != nil && number > *spec.Max {
		return fmt.Errorf("%v is above the maximum %v", number, *spec.Max)
	}
	return nil
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

// readManifest parses dir/extension.json and validates it.
func readManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := Validate(manifest, dir); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Scan lists the valid extensions in the direct subfolders of extDir.
// Invalid or unreadable ones are skipped so one bad folder cannot stop the
// saver from starting.
func Scan(extDir string) []Installed {
	found, _ := ScanWithProblems(extDir)
	return found
}

// ScanWithProblems is Scan, and it also says why each skipped folder was
// skipped, for a log.
func ScanWithProblems(extDir string) ([]Installed, error) {
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return nil, nil // no folder yet: nothing is installed
	}
	var found []Installed
	var problems []error
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), stagingPrefix) {
			continue
		}
		dir := filepath.Join(extDir, entry.Name())
		manifest, err := readManifest(dir)
		switch {
		case err != nil:
			problems = append(problems, fmt.Errorf("extension %s: %w", entry.Name(), err))
		case manifest.ID != entry.Name():
			// The folder name must equal the id: Remove looks up extDir/<id>,
			// and folder names are unique, so ids stay unique too.
			problems = append(problems, fmt.Errorf("extension %s: folder holds id %q", entry.Name(), manifest.ID))
		default:
			found = append(found, Installed{Manifest: manifest, Dir: dir})
		}
	}
	return found, errors.Join(problems...)
}

// Remove deletes an installed extension. Built-in ids are refused.
func Remove(id, extDir string) error {
	if strings.HasPrefix(id, builtinPrefix) {
		return fmt.Errorf("remove %q: %w", id, ErrBuiltin)
	}
	// The id becomes a path element, so it must match the strict pattern.
	if !idPattern.MatchString(id) {
		return fmt.Errorf("remove: invalid id %q", id)
	}
	target := filepath.Join(extDir, id)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("remove %q: %w", id, err)
	}
	return os.RemoveAll(target)
}
