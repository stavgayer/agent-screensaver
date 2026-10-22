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
	"agent-screensaver/internal/statefile"
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

// fakeSnapshot has one number option and one file option, for the rule that
// a file option is set only through the file dialog.
var fakeSnapshot = settings.Snapshot{
	Config: config.Config{Theme: "builtin.default"},
	Extensions: []settings.ExtensionInfo{
		{Manifest: extensions.Manifest{ID: "builtin.matrix", Options: map[string]extensions.OptionSpec{"fps": {Kind: extensions.OptionNumber}}}},
		{Manifest: extensions.Manifest{ID: "builtin.image", Options: map[string]extensions.OptionSpec{"file": {Kind: extensions.OptionFile}}}},
	},
}

func (f *fakeService) Snapshot() (settings.Snapshot, error) {
	return fakeSnapshot, f.record("Snapshot")
}

// changes are the calls other than Snapshot, which the bridge also uses to
// look up an option's kind.
func (f *fakeService) changes() []string {
	var changes []string
	for _, call := range f.calls {
		if call != "Snapshot" {
			changes = append(changes, call)
		}
	}
	return changes
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
func (f *fakeService) SetIgnoredProjects(projects []string) error {
	return f.record("SetIgnoredProjects " + strings.Join(projects, ","))
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
// While its dialog is open it can make one more call to the page handler,
// like a page that keeps posting while macOS shows the open panel.
type fakePicker struct {
	path      string
	requests  []platform.FileRequest
	window    *fakeWindow
	nested    *call
	nestedErr error
}

func (f *fakePicker) PickFile(request platform.FileRequest) (string, bool, error) {
	f.requests = append(f.requests, request)
	if f.nested != nil {
		_, f.nestedErr = f.window.handle(f.nested.method, json.RawMessage(f.nested.params))
	}
	return f.path, f.path != "", nil
}

// fakeWindow calls the page handler with each call, like a page would.
type fakeWindow struct {
	calls      []call
	page       string
	readAccess string
	results    []any
	errs       []error
	handle     platform.PageHandler
}

type call struct {
	method string
	params string
}

func (f *fakeWindow) Run(page, readAccess string, handle platform.PageHandler) error {
	f.page, f.readAccess, f.handle = page, readAccess, handle
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
		{"snapshot", "snapshot", ``, "", "Snapshot", fakeSnapshot},
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
		{"ignored projects", "setIgnoredProjects", `{"projects":["observer-sessions","scratch"]}`, "", "SetIgnoredProjects observer-sessions,scratch", nil},
		{"no ignored projects", "setIgnoredProjects", `{"projects":[]}`, "", "SetIgnoredProjects ", nil},
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
			got := service.changes()
			if tt.method == "snapshot" {
				got = service.calls
			}
			if want := []string{tt.wantCall}; !reflect.DeepEqual(got, want) {
				t.Fatalf("service calls = %q, want %q", got, want)
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
			if window.errs[0] != nil || len(service.changes()) != 0 {
				t.Fatalf("err = %v, calls = %q, want no error and no change", window.errs[0], service.calls)
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
		{"bad parameters", "setMaxFps", `{"fps":"fast"}`, "parameters: ", nil},
		{"unknown parameter", "setTheme", `{"theme":"x"}`, "unknown field", nil},
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
	if err := (config.Config{Privacy: true, MaxFPS: 30}).Save(p.ConfigFile); err != nil {
		t.Fatal(err)
	}
	window := &fakeWindow{calls: []call{{"state", ``}}}
	if err := Open(p, &fakeService{}, &fakePicker{}, window); err != nil {
		t.Fatal(err)
	}
	// With no daemon there is no state.json: the preview still gets an idle
	// screen built from the settings, marked as from a stopped service.
	text, ok := window.results[0].(string)
	var idle statefile.StateFile
	if window.errs[0] != nil || !ok || json.Unmarshal([]byte(text), &idle) != nil {
		t.Fatalf("missing state = %#v, %v; want state JSON", window.results[0], window.errs[0])
	}
	if idle.ServiceOK || len(idle.Cards) != 0 || !idle.View.Privacy || idle.View.MaxFPS != 30 {
		t.Fatalf("idle state = %+v; want no service, no cards, the settings' view", idle)
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

func TestFileOptionOnlyThroughTheDialog(t *testing.T) {
	tests := []struct {
		name, params, pickerPath, wantErr string
		wantChanges                       []string
	}{
		{"a typed path is refused", `{"extension":"builtin.image","key":"file","value":"/Users/me/.ssh/id_rsa"}`, "", "file dialog", nil},
		{"a picked path is used", `{"extension":"builtin.image","key":"file","pick":true}`, "/tmp/a.png", "", []string{"SetOption builtin.image file /tmp/a.png"}},
		{"the dialog is only for file options", `{"extension":"builtin.matrix","key":"fps","pick":true}`, "/tmp/a.png", "not a file option", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeService{}
			window := &fakeWindow{calls: []call{{"setOption", tt.params}}}
			if err := Open(testPaths(t), service, &fakePicker{path: tt.pickerPath}, window); err != nil {
				t.Fatal(err)
			}
			err := window.errs[0]
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
			if !reflect.DeepEqual(service.changes(), tt.wantChanges) {
				t.Fatalf("changes = %q, want %q", service.changes(), tt.wantChanges)
			}
		})
	}
}

func TestNoCallsWhileTheDialogIsOpen(t *testing.T) {
	service := &fakeService{}
	window := &fakeWindow{calls: []call{{"installExtension", `{}`}, {"setPrivacy", `{"on":true}`}}}
	picker := &fakePicker{path: "/tmp/rain.zip", window: window, nested: &call{"setPrivacy", `{"on":true}`}}
	if err := Open(testPaths(t), service, picker, window); err != nil {
		t.Fatal(err)
	}
	if picker.nestedErr == nil || !strings.Contains(picker.nestedErr.Error(), "file dialog is open") {
		t.Fatalf("nested call error = %v, want a refusal", picker.nestedErr)
	}
	want := []string{"InstallExtension /tmp/rain.zip", "SetPrivacy on"} // calls work again once it closes
	if !reflect.DeepEqual(service.changes(), want) {
		t.Fatalf("changes = %q, want %q", service.changes(), want)
	}
}
