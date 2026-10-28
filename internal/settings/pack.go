package settings

import (
	"slices"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
)

// IsPack reports whether path is a pack: a folder or a .zip file of
// several extensions (docs/extensions.md).
func (s Service) IsPack(path string) (bool, error) {
	return extensions.IsPack(path)
}

// InstallPack installs every part of a pack. Each part passes the checks
// of InstallExtension, and none is installed unless all pass. A pack may
// not hold an agent: an agent is installed alone, so its review runs.
func (s Service) InstallPack(path string) ([]extensions.Manifest, error) {
	return extensions.InstallPack(path, s.Paths.ExtensionsDir, func(m extensions.Manifest, dir string) error {
		if err := s.checkAgent(m, dir); err != nil {
			return err
		}
		_, err := s.reviewStaged(path, m, dir) // refuses to replace an installed agent
		return err
	})
}

// Removal is what RemoveTheme did.
type Removal struct {
	Removed []string
	Kept    []extensions.KeptPart
	// ThemeReset is true when the removed theme was in use and the
	// settings went back to the default theme.
	ThemeReset bool
}

// RemoveTheme removes a theme with the parts it brought: its layout, its
// background and the widgets of that layout, except built-ins and parts
// another installed theme or layout still uses. Any other id is removed
// alone, as RemoveExtension does. When the removed theme was in use, the
// settings go back to the default theme, so a later install of a theme
// with the same id does not turn it on again. A removed background is no
// longer the user's choice either. When a remove fails, Removed lists
// what was removed before it.
func (s Service) RemoveTheme(id string) (Removal, error) {
	plan, err := extensions.PlanThemeRemoval(id, extensions.Scan(s.Paths.ExtensionsDir), nil)
	if err != nil {
		// Not a valid installed extension: RemoveExtension gives the same
		// answer as before (a built-in, a bad id, or a broken folder).
		plan = extensions.ThemeRemoval{Remove: []string{id}}
	}
	var removal Removal
	for _, part := range plan.Remove {
		if err := s.RemoveExtension(part); err != nil {
			return removal, err
		}
		removal.Removed = append(removal.Removed, part)
	}
	removal.Kept = plan.Kept
	cfg := config.Load(s.Paths.ConfigFile)
	removal.ThemeReset = cfg.Theme == id // SetTheme only stores themes
	if !removal.ThemeReset && !slices.Contains(removal.Removed, cfg.Background) {
		return removal, nil
	}
	return removal, s.update(func(cfg *config.Config) error {
		if cfg.Theme == id {
			cfg.Theme = config.Default().Theme
		}
		if slices.Contains(removal.Removed, cfg.Background) {
			cfg.Background = ""
		}
		return nil
	})
}
