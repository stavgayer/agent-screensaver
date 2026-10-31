package settingsui

import (
	"reflect"
	"strings"
	"testing"

	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/install"
	"agent-screensaver/internal/settings"
)

var changedConverter = settings.InstallReview{
	Upgrade: true, FromVersion: "1.0.0", Hash: "review-1",
	Changes: []install.FileChange{{Path: "bin/convert", Kind: install.FileChanged, Diff: "--- a/bin/convert\n+++ b/bin/convert\n"}},
}

func runCalls(t *testing.T, service *fakeService, calls ...call) *fakeWindow {
	t.Helper()
	window := &fakeWindow{calls: calls}
	if err := Open(testPaths(t), "v9.9.9", service, &fakePicker{path: "/tmp/acme.zip"}, window); err != nil {
		t.Fatal(err)
	}
	return window
}

func TestInstallWithNoRunnableChangesInstallsAtOnce(t *testing.T) {
	service := &fakeService{review: settings.InstallReview{Upgrade: true, Changes: []install.FileChange{}}}

	window := runCalls(t, service, call{"installExtension", `{}`})

	if window.errs[0] != nil {
		t.Fatal(window.errs[0])
	}
	if want := []string{"ReviewInstall /tmp/acme.zip", "InstallReviewed /tmp/acme.zip "}; !reflect.DeepEqual(service.changes(), want) {
		t.Errorf("calls = %q, want %q", service.changes(), want)
	}
	want := pickResult{Manifest: &extensions.Manifest{ID: "acme.rain"}, Unchanged: true}
	if !reflect.DeepEqual(window.results[0], want) {
		t.Errorf("result = %#v", window.results[0])
	}
}

func TestInstallWithRunnableChangesReturnsThePlanAndWaits(t *testing.T) {
	service := &fakeService{review: changedConverter}

	window := runCalls(t, service, call{"installExtension", `{}`})

	if window.errs[0] != nil {
		t.Fatal(window.errs[0])
	}
	if want := []string{"ReviewInstall /tmp/acme.zip"}; !reflect.DeepEqual(service.changes(), want) {
		t.Errorf("calls = %q, want only the review", service.changes())
	}
	result := window.results[0].(pickResult)
	if result.Review == nil || result.Review.Hash != "review-1" || result.Review.Changes[0].Path != "bin/convert" || result.Manifest != nil {
		t.Errorf("result = %#v", result)
	}
}

func TestConfirmInstallInstallsThePickedFileWithItsHash(t *testing.T) {
	service := &fakeService{review: changedConverter}

	window := runCalls(t, service, call{"installExtension", `{}`}, call{"confirmInstall", `{"hash":"review-1"}`})

	if window.errs[1] != nil {
		t.Fatal(window.errs[1])
	}
	if want := []string{"ReviewInstall /tmp/acme.zip", "InstallReviewed /tmp/acme.zip review-1"}; !reflect.DeepEqual(service.changes(), want) {
		t.Errorf("calls = %q, want %q", service.changes(), want)
	}
	if result := window.results[1].(pickResult); result.Manifest == nil || result.Manifest.ID != "acme.rain" {
		t.Errorf("result = %#v", result)
	}
}

func TestConfirmInstallRefusesWithoutAWaitingReview(t *testing.T) {
	tests := []struct {
		name  string
		calls []call
	}{
		{"nothing picked", []call{{"confirmInstall", `{"hash":"review-1"}`}}},
		{"another hash", []call{{"installExtension", `{}`}, {"confirmInstall", `{"hash":"review-2"}`}}},
		{"cancelled", []call{{"installExtension", `{}`}, {"cancelInstall", `{}`}, {"confirmInstall", `{"hash":"review-1"}`}}},
		{"confirmed once already", []call{{"installExtension", `{}`}, {"confirmInstall", `{"hash":"review-1"}`}, {"confirmInstall", `{"hash":"review-1"}`}}},
		{"a path is refused", []call{{"installExtension", `{}`}, {"confirmInstall", `{"hash":"review-1","path":"/tmp/other.zip"}`}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeService{review: changedConverter}

			window := runCalls(t, service, tt.calls...)

			last := len(tt.calls) - 1
			if window.errs[last] == nil {
				t.Fatalf("confirm answered %#v, want an error", window.results[last])
			}
			installs := 0
			for _, made := range service.changes() {
				if strings.HasPrefix(made, "InstallReviewed") {
					installs++
				}
			}
			if want := map[bool]int{true: 1, false: 0}[tt.name == "confirmed once already"]; installs != want {
				t.Errorf("calls = %q, want %d installs", service.changes(), want)
			}
		})
	}
}

func TestInstallAPackInstallsEveryPartAndShowsTheTheme(t *testing.T) {
	service := &fakeService{pack: true}

	window := runCalls(t, service, call{"installExtension", `{}`})

	if window.errs[0] != nil {
		t.Fatal(window.errs[0])
	}
	if want := []string{"InstallPack /tmp/acme.zip"}; !reflect.DeepEqual(service.changes(), want) {
		t.Errorf("calls = %q, want %q", service.changes(), want)
	}
	want := pickResult{Manifest: &extensions.Manifest{ID: "acme.dark", Type: extensions.TypeTheme}}
	if !reflect.DeepEqual(window.results[0], want) {
		t.Errorf("result = %#v", window.results[0])
	}
}
