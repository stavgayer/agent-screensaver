package daemon

import (
	"os"
	"path/filepath"
	"sort"

	"agent-screensaver/internal/aerials"
	"agent-screensaver/internal/platform"
	"agent-screensaver/internal/view"
)

// aerialSyncEveryMs is how often the clones follow new downloads and deleted
// videos while nothing else changes. macOS downloads an aerial in minutes,
// so ten minutes is soon enough, and a sync is only a few stat calls.
const aerialSyncEveryMs = 10 * 60_000

// wantedAerials lists the downloaded videos the active background needs:
// those of the chosen group for the aerials background, none for any other.
func wantedAerials(background view.ResolvedExt, locations aerials.Locations) []aerials.Asset {
	if background.ID != aerials.ExtensionID {
		return nil
	}
	group, ok := aerials.ParseGroup(background.Options[aerials.GroupOption])
	if !ok {
		return nil
	}
	return locations.Downloaded(group)
}

// syncAerialClones makes dir hold exactly one APFS clone per wanted video,
// named <asset id>.mov, and returns the clone paths sorted. A clone is
// removed when its video is no longer wanted, including when its source was
// deleted: a clone of a deleted file keeps the disk blocks alive. A video
// that cannot be cloned is skipped and logged; it is never copied. If dir
// exists but is not a real folder (a symlink, a file), nothing is deleted or
// cloned, so a link can never point the deletion at someone else's files.
func syncAerialClones(wanted []aerials.Asset, dir string, cloner platform.FileCloner, logf func(string, ...any)) []string {
	if info, err := os.Lstat(dir); err == nil && !info.IsDir() {
		logf("aerials: %s is not a folder; leaving it alone", dir)
		return []string{}
	}
	sources := make(map[string]string, len(wanted))
	for _, asset := range wanted {
		sources[asset.ID+".mov"] = asset.Path
	}
	removeUnwanted(dir, sources, logf)
	if len(sources) == 0 {
		return []string{}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logf("aerials: create %s: %v", dir, err)
		return []string{}
	}
	clones := []string{}
	for name, source := range sources {
		clone := filepath.Join(dir, name)
		if _, err := os.Stat(clone); err != nil {
			if cloner == nil {
				continue
			}
			if err := cloner.Clone(source, clone); err != nil {
				logf("aerials: skip %s: %v", name, err)
				continue
			}
		}
		clones = append(clones, clone)
	}
	sort.Strings(clones)
	return clones
}

func removeUnwanted(dir string, wanted map[string]string, logf func(string, ...any)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no folder yet: nothing to remove
	}
	for _, entry := range entries {
		if _, keep := wanted[entry.Name()]; keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			logf("aerials: remove %s: %v", entry.Name(), err)
		}
	}
}
