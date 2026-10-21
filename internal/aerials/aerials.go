// Package aerials lists the Apple aerial videos that macOS has already
// downloaded. It only reads; it never downloads or copies a video. The
// folders it reads are not an official API, so anything missing, moved or
// broken gives an empty list and never an error.
package aerials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// ExtensionID is the built-in background that plays these videos, and
// GroupOption is its option that picks the group.
const (
	ExtensionID = "builtin.aerials"
	GroupOption = "group"
)

// Group is one of the four aerial groups macOS shows in its settings.
type Group string

const (
	Landscapes Group = "landscapes"
	Cities     Group = "cities"
	Space      Group = "space"
	Underwater Group = "underwater"
)

// groupByCategoryKey maps the manifest's category name key to our group.
// Other categories (for example AerialCategoryMac) are not offered.
var groupByCategoryKey = map[string]Group{
	"AerialCategoryLandscapes": Landscapes,
	"AerialCategoryCities":     Cities,
	"AerialCategorySpace":      Space,
	"AerialCategoryUnderwater": Underwater,
}

// ParseGroup accepts a stored option value. Anything that is not one of the
// four group names is not a group.
func ParseGroup(value any) (Group, bool) {
	text, _ := value.(string)
	for _, group := range groupByCategoryKey {
		if string(group) == text {
			return group, true
		}
	}
	return "", false
}

// Asset is one downloaded aerial video.
type Asset struct {
	ID   string
	Path string // absolute path of the downloaded .mov
}

// Locations says where macOS keeps the aerials.
type Locations struct {
	Manifest string // the wallpaper manifest (macOS 26)
	Catalog  string // the older idleassetsd catalog, used only when Manifest is missing
	Videos   string // the downloaded videos
}

// LocationsForHome builds the locations for the given real home. The catalog
// is a system folder and does not depend on the home. The idleassetsd folder
// holds no videos on macOS 26, so videos are always read from the wallpaper
// folder (checked on the owner's Mac on 21/10/2026).
func LocationsForHome(home string) Locations {
	wallpaper := filepath.Join(home, "Library", "Application Support", "com.apple.wallpaper", "aerials")
	return Locations{
		Manifest: filepath.Join(wallpaper, "manifest", "entries.json"),
		Catalog:  "/Library/Application Support/com.apple.idleassetsd/Customer/entries.json",
		Videos:   filepath.Join(wallpaper, "videos"),
	}
}

// Downloaded lists the downloaded videos of group g, read from these locations.
func (l Locations) Downloaded(g Group) []Asset {
	return Downloaded(l.manifest(), l.Videos, g)
}

func (l Locations) manifest() string {
	if _, err := os.Stat(l.Manifest); err != nil {
		return l.Catalog
	}
	return l.Manifest
}

// manifestFile is the part of entries.json we read. Assets name their
// categories by id; categories carry the name key that says which group.
type manifestFile struct {
	Categories []struct {
		ID      string `json:"id"`
		NameKey string `json:"localizedNameKey"`
	} `json:"categories"`
	Assets []struct {
		ID         string   `json:"id"`
		Categories []string `json:"categories"`
	} `json:"assets"`
}

// safeID keeps an asset id usable as a file name: the manifest is not ours,
// so an id like "../x" must never reach the file system.
var safeID = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// Downloaded lists the assets of group g in the manifest at manifestPath
// whose video exists in videosDir, sorted by id. macOS names a downloaded
// video `<asset id>.mov` (checked against the 9 downloaded videos on the
// owner's Mac, 21/10/2026). The result is never nil.
func Downloaded(manifestPath, videosDir string, g Group) []Asset {
	assets := []Asset{}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return assets
	}
	var manifest manifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return assets
	}
	inGroup := map[string]bool{}
	for _, category := range manifest.Categories {
		if group, known := groupByCategoryKey[category.NameKey]; known && group == g {
			inGroup[category.ID] = true
		}
	}
	for _, asset := range manifest.Assets {
		if !safeID.MatchString(asset.ID) || !anyIn(asset.Categories, inGroup) {
			continue
		}
		path := filepath.Join(videosDir, asset.ID+".mov")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			assets = append(assets, Asset{ID: asset.ID, Path: path})
		}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].ID < assets[j].ID })
	return assets
}

func anyIn(categories []string, wanted map[string]bool) bool {
	for _, category := range categories {
		if wanted[category] {
			return true
		}
	}
	return false
}
