package aerials

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const fixtureManifest = "testdata/entries.json"

// videoFolder makes an empty .mov file for each id, the way macOS names a
// downloaded aerial, and returns the folder.
func videoFolder(t *testing.T, ids ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, id := range ids {
		if err := os.WriteFile(filepath.Join(dir, id+".mov"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func assetsIn(dir string, ids ...string) []Asset {
	assets := []Asset{}
	for _, id := range ids {
		assets = append(assets, Asset{ID: id, Path: filepath.Join(dir, id+".mov")})
	}
	return assets
}

func TestDownloaded(t *testing.T) {
	every := []string{"C1-DUBAI", "C2-NEW-YORK", "L1-TAHOE", "S1-EARTH", "U1-DOLPHINS", "M1-MACOS", "X1-BOTH"}
	allVideos := videoFolder(t, every...)
	someVideos := videoFolder(t, "C2-NEW-YORK", "S1-EARTH")
	withFolder := videoFolder(t)
	if err := os.Mkdir(filepath.Join(withFolder, "C1-DUBAI.mov"), 0o755); err != nil {
		t.Fatal(err)
	}
	brokenManifest := filepath.Join(t.TempDir(), "entries.json")
	if err := os.WriteFile(brokenManifest, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "gone")

	tests := []struct {
		name     string
		manifest string
		videos   string
		group    Group
		want     []Asset
	}{
		{"reads cities from fixture, sorted by id", fixtureManifest, allVideos, Cities, assetsIn(allVideos, "C1-DUBAI", "C2-NEW-YORK")},
		{"reads landscapes from fixture", fixtureManifest, allVideos, Landscapes, assetsIn(allVideos, "L1-TAHOE", "X1-BOTH")},
		{"reads space from fixture", fixtureManifest, allVideos, Space, assetsIn(allVideos, "S1-EARTH")},
		{"reads underwater from fixture", fixtureManifest, allVideos, Underwater, assetsIn(allVideos, "U1-DOLPHINS", "X1-BOTH")},
		{"only downloaded files listed", fixtureManifest, someVideos, Cities, assetsIn(someVideos, "C2-NEW-YORK")},
		{"group with nothing downloaded is empty", fixtureManifest, someVideos, Underwater, []Asset{}},
		{"a folder named like a video is not a video", fixtureManifest, withFolder, Cities, []Asset{}},
		{"unknown group gives empty", fixtureManifest, allVideos, Group("mac"), []Asset{}},
		{"empty group gives empty, not the unknown categories", fixtureManifest, allVideos, Group(""), []Asset{}},
		{"missing folder gives empty", fixtureManifest, missing, Cities, []Asset{}},
		{"missing manifest gives empty", missing, allVideos, Cities, []Asset{}},
		{"broken manifest gives empty", brokenManifest, allVideos, Cities, []Asset{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Downloaded(tt.manifest, tt.videos, tt.group)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Downloaded = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDownloadedSkipsUnsafeIDs(t *testing.T) {
	videos := t.TempDir()
	// "../C9-ESCAPE.mov" seen from videos/inner is this file.
	if err := os.WriteFile(filepath.Join(videos, "C9-ESCAPE.mov"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(videos, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := Downloaded(fixtureManifest, inner, Cities); len(got) != 0 {
		t.Errorf("Downloaded = %+v, want no asset outside the videos folder", got)
	}
}

func TestLocationsManifestFallback(t *testing.T) {
	home := t.TempDir()
	catalog := filepath.Join(t.TempDir(), "entries.json")
	locations := LocationsForHome(home)
	locations.Catalog = catalog

	if got := locations.manifest(); got != catalog {
		t.Errorf("without the wallpaper manifest: manifest = %q, want the catalog %q", got, catalog)
	}
	if err := os.MkdirAll(filepath.Dir(locations.Manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(locations.Manifest, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := locations.manifest(); got != locations.Manifest {
		t.Errorf("with the wallpaper manifest: manifest = %q, want %q", got, locations.Manifest)
	}
}

func TestLocationsDownloadedUsesCatalogForCategories(t *testing.T) {
	home := t.TempDir()
	locations := LocationsForHome(home)
	locations.Catalog = fixtureManifest
	if err := os.MkdirAll(locations.Videos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locations.Videos, "S1-EARTH.mov"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got := locations.Downloaded(Space)

	if want := assetsIn(locations.Videos, "S1-EARTH"); !reflect.DeepEqual(got, want) {
		t.Errorf("Downloaded = %+v, want %+v", got, want)
	}
}

func TestLocationsForHome(t *testing.T) {
	got := LocationsForHome("/Users/me")
	want := Locations{
		Manifest: "/Users/me/Library/Application Support/com.apple.wallpaper/aerials/manifest/entries.json",
		Catalog:  "/Library/Application Support/com.apple.idleassetsd/Customer/entries.json",
		Videos:   "/Users/me/Library/Application Support/com.apple.wallpaper/aerials/videos",
	}
	if got != want {
		t.Errorf("LocationsForHome = %+v, want %+v", got, want)
	}
}

func TestParseGroup(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		want   Group
		wantOK bool
	}{
		{"cities", "cities", Cities, true},
		{"underwater", "underwater", Underwater, true},
		{"unknown text", "mac", "", false},
		{"not text", 3.0, "", false},
		{"nothing", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseGroup(tt.value)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("ParseGroup(%v) = %q, %v; want %q, %v", tt.value, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
