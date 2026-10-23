// Package view turns the user's settings and the installed extensions into
// the exact theme, background, layout, components and options to show.
package view

import (
	"encoding/json"
	"maps"
	"net/url"
	"path/filepath"
	"sort"

	"agent-screensaver/internal/aerials"
	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
)

const (
	// defaultThemeID is the built-in theme used when the chosen one cannot be used.
	defaultThemeID = "builtin.default"
	// matrixID is the background shown when the aerials have no videos.
	matrixID = "builtin.matrix"
	// filesOption carries the aerial videos to the aerials background. It is
	// not in the manifest, so a user can never set it.
	filesOption = "files"
)

// ResolvedExt is an extension ready for the web runtime to load.
type ResolvedExt struct {
	ID       string         `json:"id"`
	EntryURL string         `json:"entry_url"`
	Options  map[string]any `json:"options"`
}

// View is everything the runtime needs to draw one screen.
type View struct {
	Theme      string          `json:"theme"`
	Background ResolvedExt     `json:"background"`
	Layout     json.RawMessage `json:"layout"`
	Components []ResolvedExt   `json:"components"`
	Style      map[string]any  `json:"style"`
	Privacy    bool            `json:"privacy"`
	MaxFPS     int             `json:"max_fps"` // 0 = no ceiling beyond the display
	// Agents names each agent and its mark, by event source. A card whose
	// source is not here shows a neutral dot and the source name.
	Agents map[string]AgentView `json:"agents"`
	// AgentOrder lists the agents' sources, built-ins first, in the order
	// limits and rows show them.
	AgentOrder []string `json:"agent_order"`
}

// AgentView is how a card shows its agent.
type AgentView struct {
	Name    string `json:"name"`
	MarkURL string `json:"mark_url"`
}

// layoutModes is the part of a layout manifest that lists components.
type layoutModes struct {
	Modes map[string]map[string][]extensions.ExtRef `json:"modes"`
}

type index map[string]extensions.Installed

// Resolve picks the theme, background, layout and components. Anything
// missing or of the wrong type falls back to builtin.default. If even that
// is missing it returns an empty view, and the runtime shows a plain clock.
// aerialClones are the aerial videos the aerials background may play.
func Resolve(cfg config.Config, installed []extensions.Installed, aerialClones []string) View {
	byID := newIndex(installed)
	result := emptyView(cfg)
	theme, specs, ok := byID.theme(cfg)
	if !ok {
		return result
	}
	result.Theme = theme.Manifest.ID
	if specs[0].Style != nil {
		result.Style = maps.Clone(specs[0].Style)
	}
	if background, ok := byID.resolveBackground(cfg, specs); ok {
		result.Background = byID.useAerialClones(cfg, background, aerialClones)
	}
	if layout, ok := byID.resolveLayout(specs); ok {
		byID.applyLayout(&result, cfg, layout.Manifest.Layout)
	}
	return result
}

// Background is the background the theme asks for, with its options, before
// the aerials fallback. The daemon reads it to know which aerials to clone.
// With no usable theme the ID is empty.
func Background(cfg config.Config, installed []extensions.Installed) ResolvedExt {
	byID := newIndex(installed)
	if _, specs, ok := byID.theme(cfg); ok {
		if background, ok := byID.resolveBackground(cfg, specs); ok {
			return background
		}
	}
	return ResolvedExt{Options: map[string]any{}}
}

func newIndex(installed []extensions.Installed) index {
	byID := make(index, len(installed))
	for _, ext := range installed {
		byID[ext.Manifest.ID] = ext
	}
	return byID
}

// theme returns the chosen theme, or builtin.default, and the theme specs to
// try in order: the theme's own, then the default's.
func (byID index) theme(cfg config.Config) (extensions.Installed, []extensions.ThemeSpec, bool) {
	fallback, hasFallback := byID.find(defaultThemeID, extensions.TypeTheme)
	theme, ok := byID.find(cfg.Theme, extensions.TypeTheme)
	if !ok {
		theme, ok = fallback, hasFallback
	}
	if !ok || theme.Manifest.Theme == nil {
		return extensions.Installed{}, nil, false
	}
	specs := []extensions.ThemeSpec{*theme.Manifest.Theme}
	if hasFallback && fallback.Manifest.Theme != nil {
		specs = append(specs, *fallback.Manifest.Theme)
	}
	return theme, specs, true
}

// useAerialClones gives the aerials background its videos as file URLs.
// With none, the screen shows Matrix with the user's Matrix options instead
// of black, so a missing or moved Apple folder fails quietly.
func (byID index) useAerialClones(cfg config.Config, background ResolvedExt, clones []string) ResolvedExt {
	if background.ID != aerials.ExtensionID {
		return background
	}
	if matrix, ok := byID.find(matrixID, extensions.TypeBackground); ok && len(clones) == 0 {
		return resolveExt(matrix, cfg, nil)
	}
	files := make([]string, 0, len(clones))
	for _, clone := range clones {
		files = append(files, fileURL(clone))
	}
	background.Options[filesOption] = files // a fresh map from mergeOptions
	return background
}

// maxFPSLimit is the highest frame-rate ceiling the runtime accepts.
const maxFPSLimit = 240

// emptyView has no null collections, because the runtime trusts the shape.
func emptyView(cfg config.Config) View {
	maxFPS := cfg.MaxFPS
	if maxFPS < 0 || maxFPS > maxFPSLimit {
		maxFPS = 0 // out of range means no ceiling
	}
	return View{
		Background: ResolvedExt{Options: map[string]any{}},
		Components: []ResolvedExt{},
		Style:      map[string]any{},
		Privacy:    cfg.Privacy,
		MaxFPS:     maxFPS,
		Agents:     map[string]AgentView{},
		AgentOrder: []string{},
	}
}

// AgentOrder is the sources of the agents in the order they were loaded:
// built-ins first, each group by folder name.
func AgentOrder(installed []agents.Agent) []string {
	order := make([]string, 0, len(installed))
	for _, agent := range installed {
		order = append(order, agent.Source)
	}
	return order
}

// AgentViews names the agents and their marks. A user's own mark,
// marksDir/<source>.svg, replaces the extension's when it passes the same
// checks (extensions.CheckMark); the runtime shows marks with <img>.
func AgentViews(installed []agents.Agent, marksDir string) map[string]AgentView {
	views := make(map[string]AgentView, len(installed))
	for _, agent := range installed {
		mark := agent.MarkPath()
		if own := filepath.Join(marksDir, agent.Source+".svg"); extensions.CheckMark(own) == nil {
			mark = own
		}
		name := agent.Name
		if name == "" {
			name = agent.Source
		}
		views[agent.Source] = AgentView{Name: name, MarkURL: fileURL(mark)}
	}
	return views
}

// applyLayout stores the sanitized layout and the components it uses. A
// layout that cannot be decoded leaves both empty.
func (byID index) applyLayout(result *View, cfg config.Config, rawLayout json.RawMessage) {
	var layout layoutModes
	if err := json.Unmarshal(rawLayout, &layout); err != nil {
		return
	}
	layout = byID.sanitizeLayout(layout)
	encoded, err := json.Marshal(layout)
	if err != nil {
		return
	}
	result.Layout = encoded
	result.Components = byID.resolveComponents(cfg, layout)
}

// sanitizeLayout keeps only placements of installed components, and only the
// placement options the component declares with a valid value. Slices are
// never nil so they encode as [] and not null.
func (byID index) sanitizeLayout(layout layoutModes) layoutModes {
	clean := layoutModes{Modes: make(map[string]map[string][]extensions.ExtRef, len(layout.Modes))}
	for mode, regions := range layout.Modes {
		clean.Modes[mode] = make(map[string][]extensions.ExtRef, len(regions))
		for region, refs := range regions {
			kept := make([]extensions.ExtRef, 0, len(refs))
			for _, ref := range refs {
				if ext, ok := byID.find(ref.ID, extensions.TypeComponent); ok {
					kept = append(kept, extensions.ExtRef{ID: ref.ID, Options: validOptions(ext.Manifest.Options, ref.Options)})
				}
			}
			clean.Modes[mode][region] = kept
		}
	}
	return clean
}

// validOptions drops undeclared options and values that fail validation.
func validOptions(specs map[string]extensions.OptionSpec, options map[string]any) map[string]any {
	var kept map[string]any
	for name, value := range options {
		if spec, ok := specs[name]; ok && extensions.ValidateOption(spec, value) == nil {
			if kept == nil {
				kept = map[string]any{}
			}
			kept[name] = value
		}
	}
	return kept // nil is omitted by the omitempty tag
}

func (byID index) find(id string, kind extensions.Type) (extensions.Installed, bool) {
	ext, ok := byID[id]
	return ext, ok && ext.Manifest.Type == kind
}

// resolveBackground uses the user's background when it is installed, then
// the first spec whose background is installed, so a broken theme still gets
// the default background. Options merge the same way for both: manifest
// defaults, the theme's values for that background, then the user's.
func (byID index) resolveBackground(cfg config.Config, specs []extensions.ThemeSpec) (ResolvedExt, bool) {
	if ext, ok := byID.find(cfg.Background, extensions.TypeBackground); ok {
		return resolveExt(ext, cfg, themeOptionsFor(cfg.Background, specs)), true
	}
	for _, spec := range specs {
		if ext, ok := byID.find(spec.Background.ID, extensions.TypeBackground); ok {
			return resolveExt(ext, cfg, spec.Background.Options), true
		}
	}
	return ResolvedExt{}, false
}

// themeOptionsFor returns the options the theme sets for background id, if
// the theme uses that background.
func themeOptionsFor(id string, specs []extensions.ThemeSpec) map[string]any {
	for _, spec := range specs {
		if spec.Background.ID == id {
			return spec.Background.Options
		}
	}
	return nil
}

func (byID index) resolveLayout(specs []extensions.ThemeSpec) (extensions.Installed, bool) {
	for _, spec := range specs {
		if ext, ok := byID.find(spec.Layout, extensions.TypeLayout); ok {
			return ext, true
		}
	}
	return extensions.Installed{}, false
}

// resolveComponents lists each installed component used by any mode once.
// Per-placement options stay inside the layout, so only manifest defaults
// and the user's options are merged here.
func (byID index) resolveComponents(cfg config.Config, layout layoutModes) []ResolvedExt {
	seen := map[string]bool{}
	components := []ResolvedExt{}
	for _, regions := range layout.Modes {
		for _, refs := range regions {
			for _, ref := range refs {
				ext, ok := byID.find(ref.ID, extensions.TypeComponent)
				if !ok || seen[ref.ID] {
					continue
				}
				seen[ref.ID] = true
				components = append(components, resolveExt(ext, cfg, nil))
			}
		}
	}
	sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
	return components
}

func resolveExt(ext extensions.Installed, cfg config.Config, refOptions map[string]any) ResolvedExt {
	id := ext.Manifest.ID
	return ResolvedExt{
		ID:       id,
		EntryURL: entryURL(ext),
		Options:  mergeOptions(ext.Manifest.Options, refOptions, cfg.Options[id]),
	}
}

func entryURL(ext extensions.Installed) string {
	return fileURL(filepath.Join(ext.Dir, ext.Manifest.Entry))
}

func fileURL(path string) string {
	file := url.URL{Scheme: "file", Path: path}
	return file.String()
}

// mergeOptions applies manifest defaults, then the theme or layout values,
// then the user's values. A value that fails validation, or names an option
// the manifest does not declare, is dropped so the lower layer wins.
func mergeOptions(specs map[string]extensions.OptionSpec, layers ...map[string]any) map[string]any {
	merged := make(map[string]any, len(specs))
	for name, spec := range specs {
		var value any
		if err := json.Unmarshal(spec.Default, &value); err == nil && extensions.ValidateOption(spec, value) == nil {
			merged[name] = value
		}
	}
	for _, layer := range layers {
		for name, value := range layer {
			if spec, ok := specs[name]; ok && extensions.ValidateOption(spec, value) == nil {
				merged[name] = value
			}
		}
	}
	return merged
}
