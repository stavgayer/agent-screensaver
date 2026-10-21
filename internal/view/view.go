// Package view turns the user's settings and the installed extensions into
// the exact theme, background, layout, components and options to show.
package view

import (
	"encoding/json"
	"maps"
	"net/url"
	"path/filepath"
	"sort"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
)

// defaultThemeID is the built-in theme used when the chosen one cannot be used.
const defaultThemeID = "builtin.default"

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
}

// layoutModes is the part of a layout manifest that lists components.
type layoutModes struct {
	Modes map[string]map[string][]extensions.ExtRef `json:"modes"`
}

type index map[string]extensions.Installed

// Resolve picks the theme, background, layout and components. Anything
// missing or of the wrong type falls back to builtin.default. If even that
// is missing it returns an empty view, and the runtime shows a plain clock.
func Resolve(cfg config.Config, installed []extensions.Installed) View {
	byID := make(index, len(installed))
	for _, ext := range installed {
		byID[ext.Manifest.ID] = ext
	}
	result := View{Privacy: cfg.Privacy, MaxFPS: cfg.MaxFPS}

	fallback, hasFallback := byID.find(defaultThemeID, extensions.TypeTheme)
	theme, ok := byID.find(cfg.Theme, extensions.TypeTheme)
	if !ok {
		theme, ok = fallback, hasFallback
	}
	if !ok || theme.Manifest.Theme == nil {
		return result
	}
	specs := []extensions.ThemeSpec{*theme.Manifest.Theme}
	if hasFallback && fallback.Manifest.Theme != nil {
		specs = append(specs, *fallback.Manifest.Theme)
	}

	result.Theme = theme.Manifest.ID
	result.Style = maps.Clone(specs[0].Style)
	if background, ok := byID.resolveBackground(cfg, specs); ok {
		result.Background = background
	}
	layout, hasLayout := byID.resolveLayout(specs)
	if hasLayout {
		result.Layout = layout.Manifest.Layout
		result.Components = byID.resolveComponents(cfg, layout.Manifest.Layout)
	}
	return result
}

func (byID index) find(id string, kind extensions.Type) (extensions.Installed, bool) {
	ext, ok := byID[id]
	return ext, ok && ext.Manifest.Type == kind
}

// resolveBackground uses the first spec whose background is installed, so a
// broken theme still gets the default background.
func (byID index) resolveBackground(cfg config.Config, specs []extensions.ThemeSpec) (ResolvedExt, bool) {
	for _, spec := range specs {
		if ext, ok := byID.find(spec.Background.ID, extensions.TypeBackground); ok {
			return resolveExt(ext, cfg, spec.Background.Options), true
		}
	}
	return ResolvedExt{}, false
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
func (byID index) resolveComponents(cfg config.Config, rawLayout json.RawMessage) []ResolvedExt {
	var layout layoutModes
	if err := json.Unmarshal(rawLayout, &layout); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var components []ResolvedExt
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
	file := url.URL{Scheme: "file", Path: filepath.Join(ext.Dir, ext.Manifest.Entry)}
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
