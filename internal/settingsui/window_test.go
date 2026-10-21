package settingsui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
	"agent-screensaver/internal/settings"
)

// fakeService records every call as "Method arg arg" and fails with err.
type fakeService struct {
	calls []string
	err   error
}

func (f *fakeService) record(call string) error {
	f.calls = append(f.calls, call)
	return f.err
}

func (f *fakeService) Snapshot() (settings.Snapshot, error) {
	return settings.Snapshot{Config: config.Config{Theme: "builtin.default"}}, f.record("Snapshot")
}
func (f *fakeService) SetTheme(id string) error      { return f.record("SetTheme " + id) }
func (f *fakeService) SetBackground(id string) error { return f.record("SetBackground " + id) }
func (f *fakeService) SetOption(extID, key, raw string) error {
	return f.record("SetOption " + extID + " " + key + " " + raw)
}
func (f *fakeService) ResetOption(extID, key string) error {
	return f.record("ResetOption " + extID + " " + key)
}
func (f *fakeService) SetPrivacy(on bool) error { return f.record("SetPrivacy " + yesNo(on)) }
func (f *fakeService) SetSound(on bool, file string) error {
	return f.record("SetSound " + yesNo(on) + " " + file)
}
func (f *fakeService) SetMaxFPS(fps int) error {
	return f.record("SetMaxFPS " + strings.Repeat("|", fps))
}
func (f *fakeService) InstallExtension(path string) (extensions.Manifest, error) {
	return extensions.Manifest{ID: "acme.rain"}, f.record("InstallExtension " + path)
}
func (f *fakeService) RemoveExtension(id string) error { return f.record("RemoveExtension " + id) }
func (f *fakeService) Notice(extID string) string {
	_ = f.record("Notice " + extID)
	return "notice for " + extID
}

func yesNo(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// fakePicker answers every dialog with path, or a cancel when path is "".
type fakePicker struct {
	path     string
	requests []platform.FileRequest
}

func (f *fakePicker) PickFile(request platform.FileRequest) (string, bool, error) {
	f.requests = append(f.requests, request)
	return f.path, f.path != "", nil
}

// fakeWindow calls the page handler with each call, like a page would.
type fakeWindow struct {
	calls      []call
	page       string
	readAccess string
	results    []any
	errs       []error
}

type call struct {
	method string
	params string
}

func (f *fakeWindow) Run(page, readAccess string, handle platform.PageHandler) error {
	f.page, f.readAccess = page, readAccess
	for _, c := range f.calls {
		result, err := handle(c.method, json.RawMessage(c.params))
		f.results = append(f.results, result)
		f.errs = append(f.errs, err)
	}
	return nil
}

func testPaths(t *testing.T) paths.Paths {
	t.Helper()
	p := paths.ForHome(t.TempDir())
	page := filepath.Join(p.RuntimeDir, "settings", "index.html")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenCallsTheService(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		params     string
		pickerPath string
		wantCall   string
		wantResult any
	}{
		{"snapshot", "snapshot", ``, "", "Snapshot", settings.Snapshot{Config: config.Config{Theme: "builtin.default"}}},
		{"set theme", "setTheme", `{"id":"acme.night"}`, "", "SetTheme acme.night", nil},
		{"set background", "setBackground", `{"id":"builtin.matrix"}`, "", "SetBackground builtin.matrix", nil},
		{"back to the theme's background", "setBackground", `{"id":""}`, "", "SetBackground ", nil},
		{"set option", "setOption", `{"extension":"builtin.matrix","key":"fps","value":"60"}`, "", "SetOption builtin.matrix fps 60", nil},
		{"set file option with the picker", "setOption", `{"extension":"builtin.image","key":"file","pick":true}`, "/tmp/a.png", "SetOption builtin.image file /tmp/a.png", nil},
		{"reset option", "resetOption", `{"extension":"builtin.matrix","key":"fps"}`, "", "ResetOption builtin.matrix fps", nil},
		{"privacy", "setPrivacy", `{"on":true}`, "", "SetPrivacy on", nil},
		{"sound keeps the file", "setSound", `{"on":false}`, "", "SetSound off ", nil},
		{"sound file with the picker", "setSound", `{"on":true,"pick":true}`, "/tmp/ding.aiff", "SetSound on /tmp/ding.aiff", nil},
		{"max fps", "setMaxFps", `{"fps":3}`, "", "SetMaxFPS |||", nil},
		{"install with the picker", "installExtension", `{}`, "/tmp/rain.zip", "InstallExtension /tmp/rain.zip", pickResult{Manifest: &extensions.Manifest{ID: "acme.rain"}}},
		{"remove", "removeExtension", `{"id":"acme.rain"}`, "", "RemoveExtension acme.rain", nil},
		{"notice", "notice", `{"extension":"builtin.aerials"}`, "", "Notice builtin.aerials", "notice for builtin.aerials"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, picker := &fakeService{}, &fakePicker{path: tt.pickerPath}
			window := &fakeWindow{calls: []call{{tt.method, tt.params}}}
			if err := Open(testPaths(t), service, picker, window); err != nil {
				t.Fatal(err)
			}
			if window.errs[0] != nil {
				t.Fatalf("%s: %v", tt.method, window.errs[0])
			}
			if want := []string{tt.wantCall}; !reflect.DeepEqual(service.calls, want) {
				t.Fatalf("service calls = %q, want %q", service.calls, want)
			}
			if !reflect.DeepEqual(window.results[0], tt.wantResult) {
				t.Fatalf("result = %#v, want %#v", window.results[0], tt.wantResult)
			}
		})
	}
}

func TestOpenLoadsThePageWithDataDirAccess(t *testing.T) {
	p := testPaths(t)
	window := &fakeWindow{}
	if err := Open(p, &fakeService{}, &fakePicker{}, window); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(p.RuntimeDir, "settings", "index.html"); window.page != want {
		t.Fatalf("page = %q, want %q", window.page, want)
	}
	if window.readAccess != p.DataDir {
		t.Fatalf("read access = %q, want %q", window.readAccess, p.DataDir)
	}
}

func TestOpenFailsWithoutThePage(t *testing.T) {
	err := Open(paths.ForHome(t.TempDir()), &fakeService{}, &fakePicker{}, &fakeWindow{})
	if err == nil || !strings.Contains(err.Error(), "agent-screensaver install") {
		t.Fatalf("Open error = %v, want a hint to run install", err)
	}
}

func TestCancelledPickerChangesNothing(t *testing.T) {
	tests := []struct{ method, params string }{
		{"installExtension", `{}`},
		{"setOption", `{"extension":"builtin.image","key":"file","pick":true}`},
		{"setSound", `{"on":true,"pick":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			service := &fakeService{}
			window := &fakeWindow{calls: []call{{tt.method, tt.params}}}
			if err := Open(testPaths(t), service, &fakePicker{}, window); err != nil {
				t.Fatal(err)
			}
			if window.errs[0] != nil || len(service.calls) != 0 {
				t.Fatalf("err = %v, calls = %q, want no error and no call", window.errs[0], service.calls)
			}
			if window.results[0] != (pickResult{Cancelled: true}) {
				t.Fatalf("result = %#v, want cancelled", window.results[0])
			}
		})
	}
}

func TestInstallPickerOffersZipAndFolders(t *testing.T) {
	picker := &fakePicker{}
	window := &fakeWindow{calls: []call{{"installExtension", `{}`}}}
	if err := Open(testPaths(t), &fakeService{}, picker, window); err != nil {
		t.Fatal(err)
	}
	got := picker.requests[0]
	if !got.Folders || !reflect.DeepEqual(got.Types, []string{"zip"}) {
		t.Fatalf("picker request = %+v, want zip files and folders", got)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name, method, params, want string
		serviceErr                 error
	}{
		{"unknown method", "dropTables", `{}`, `unknown method "dropTables"`, nil},
		{"bad parameters", "setMaxFps", `{"fps":"fast"}`, "setMaxFps: ", nil},
		{"unknown parameter", "setTheme", `{"theme":"x"}`, "setTheme: ", nil},
		{"service error comes back", "setTheme", `{"id":"acme.night"}`, "not installed", errors.New(`extension "acme.night" is not installed`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := &fakeWindow{calls: []call{{tt.method, tt.params}}}
			if err := Open(testPaths(t), &fakeService{err: tt.serviceErr}, &fakePicker{}, window); err != nil {
				t.Fatal(err)
			}
			if window.errs[0] == nil || !strings.Contains(window.errs[0].Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", window.errs[0], tt.want)
			}
		})
	}
}

func TestState(t *testing.T) {
	p := testPaths(t)
	window := &fakeWindow{calls: []call{{"state", ``}}}
	if err := Open(p, &fakeService{}, &fakePicker{}, window); err != nil {
		t.Fatal(err)
	}
	if window.errs[0] != nil || window.results[0] != nil {
		t.Fatalf("missing state = %#v, %v; want nil, nil", window.results[0], window.errs[0])
	}

	const content = `{"version":1,"cards":[]}`
	if err := os.WriteFile(p.StateFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	window = &fakeWindow{calls: []call{{"state", `{}`}}}
	if err := Open(p, &fakeService{}, &fakePicker{}, window); err != nil {
		t.Fatal(err)
	}
	if window.errs[0] != nil || window.results[0] != content {
		t.Fatalf("state = %#v, %v; want the file text", window.results[0], window.errs[0])
	}
}
