package settings

import (
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

// backgroundSetting names the user's background choice as a user of a
// part that stays.
const backgroundSetting = "the background setting"

// RemoveTheme removes a theme with the parts it brought: its layout, its
// background and the widgets of that layout. A part stays when it is a
// built-in, when another installed theme or layout uses it, or when it is
// the background the user picked. Any other id is removed alone, as
// RemoveOnly does. When a remove fails, Removed lists what was removed
// before it, and the settings were already fixed for those.
func (s Service) RemoveTheme(id string) (Removal, error) {
	var inUse map[string]string
	if background := config.Load(s.Paths.ConfigFile).Background; background != "" {
		inUse = map[string]string{background: backgroundSetting}
	}
	plan, err := extensions.PlanThemeRemoval(id, extensions.Scan(s.Paths.ExtensionsDir), inUse)
	if err != nil {
		// Not a valid installed extension: RemoveExtension gives the same
		// answer as before (a built-in, a bad id, or a broken folder).
		plan = extensions.ThemeRemoval{Remove: []string{id}}
	}
	return s.removeAll(plan)
}

// RemoveOnly removes one extension, as RemoveExtension does, and fixes the
// settings like RemoveTheme.
func (s Service) RemoveOnly(id string) (Removal, error) {
	return s.removeAll(extensions.ThemeRemoval{Remove: []string{id}})
}

// removeAll removes the ids in order. Right after each remove, a theme in
// use goes back to the default theme, so a later install of a theme with
// the same id does not turn it on again, and a removed background stops
// being the user's choice. So the settings stay valid when a later remove
// fails.
func (s Service) removeAll(plan extensions.ThemeRemoval) (Removal, error) {
	removal := Removal{Kept: plan.Kept}
	for _, id := range plan.Remove {
		if err := s.RemoveExtension(id); err != nil {
			return removal, err
		}
		removal.Removed = append(removal.Removed, id)
		cfg := config.Load(s.Paths.ConfigFile)
		if cfg.Theme != id && cfg.Background != id {
			continue
		}
		removal.ThemeReset = removal.ThemeReset || cfg.Theme == id // SetTheme only stores themes
		err := s.update(func(cfg *config.Config) error {
			if cfg.Theme == id {
				cfg.Theme = config.Default().Theme
			}
			if cfg.Background == id {
				cfg.Background = ""
			}
			return nil
		})
		if err != nil {
			return removal, err
		}
	}
	return removal, nil
}
