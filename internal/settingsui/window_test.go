package settingsui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/install"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
	"agent-screensaver/internal/settings"
	"agent-screensaver/internal/statefile"
)

// fakeService records every call as "Method arg arg" and fails with err.
type fakeService struct {
	calls []string
	err   error
	// review is what ReviewInstall answers; its Manifest is acme.rain.
	review settings.InstallReview
	// warmedUp, when not nil, gets the id of each WarmUpConverters call.
	warmedUp chan string
	// pack is what IsPack answers.
	pack bool
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
func (f *fakeService) SetNowPlaying(on bool) error {
	return f.record("SetNowPlaying " + yesNo(on))
}
func (f *fakeService) SetSound(on bool, file string) error {
	return f.record("SetSound " + yesNo(on) + " " + file)
}
func (f *fakeService) SetMaxFPS(fps int) error {
	return f.record("SetMaxFPS " + strings.Repeat("|", fps))
}
func (f *fakeService) SetIgnoredProjects(projects []string) error {
	return f.record("SetIgnoredProjects " + strings.Join(projects, ","))
}
func (f *fakeService) ReviewInstall(path string) (settings.InstallReview, error) {
	review := f.review
	review.Manifest = extensions.Manifest{ID: "acme.rain"}
	return review, f.record("ReviewInstall " + path)
}
func (f *fakeService) InstallReviewed(path, hash string) (extensions.Manifest, error) {
	return extensions.Manifest{ID: "acme.rain"}, f.record("InstallReviewed " + path + " " + hash)
}
func (f *fakeService) IsPack(path string) (bool, error) { return f.pack, nil }
func (f *fakeService) InstallPack(path string) ([]extensions.Manifest, error) {
	parts := []extensions.Manifest{{ID: "acme.rain", Type: extensions.TypeBackground}, {ID: "acme.dark", Type: extensions.TypeTheme}}
	return parts, f.record("InstallPack " + path)
}

// The warm-ups run in the background, so they report on warmedUp, not in
// calls.
func (f *fakeService) WarmUpConverters(id string) []string {
	f.warmUp("WarmUpConverters " + id)
	return nil
}
func (f *fakeService) WarmUpIfHooked(id string) []string {
	f.warmUp("WarmUpIfHooked " + id)
	return nil
}
func (f *fakeService) warmUp(call string) {
	if f.warmedUp != nil {
		f.warmedUp <- call
	}
}
func (f *fakeService) RemoveExtension(id string) error { return f.record("RemoveExtension " + id) }
func (f *fakeService) Notice(extID string) string {
	_ = f.record("Notice " + extID)
	return "notice for " + extID
}

func (f *fakeService) Connections(nowMs int64) ([]settings.Connection, error) {
	clock := "now"
	if nowMs <= 0 {
		clock = "no clock"
	}
	return []settings.Connection{{ID: "builtin.claude", State: settings.StateIdle}}, f.record("Connections " + clock)
}
func (f *fakeService) SetAgentOn(id string, on bool) error {
	return f.record("SetAgentOn " + id + " " + yesNo(on))
}

// fakePlan is the plan every PlanAgentHooks call returns: a header and one
// converter warning.
var fakePlan = []install.PlannedStep{
	{Name: "hooks-acme", Description: "Add hooks for Acme (from acme.agent 1.0.0)", Warning: true},
	{Name: "hooks-acme", Description: "acme.agent runs its own program bin/convert on each Acme event", Warning: true},
}

func (f *fakeService) PlanAgentHooks(id string, add bool) ([]install.PlannedStep, string, error) {
	return fakePlan, "hash-1", f.record("PlanAgentHooks " + id + " " + addRemove(add))
}
func (f *fakeService) ApplyAgentHooks(id string, add bool, hash string) error {
	return f.record("ApplyAgentHooks " + id + " " + addRemove(add) + " " + hash)
}

func addRemove(add bool) string {
	if add {
		return "add"
	}
	return "remove"
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

// fakeWindow calls the page handler with each call, like a page would. A
// call with method dropMethod is a drop of the file in params instead.
type fakeWindow struct {
	calls      []call
	page       string
	readAccess string
	results    []any
	errs       []error
	handle     platform.PageHandler
	drops      []bool
}

const dropMethod = "<drop>"

type call struct {
	method string
	params string
}

func (f *fakeWindow) Run(page, readAccess string, handle platform.PageHandler, drop platform.DropHandler) error {
	f.page, f.readAccess, f.handle = page, readAccess, handle
	for _, c := range f.calls {
		if c.method == dropMethod {
			f.drops = append(f.drops, drop(c.params))
			continue
		}
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
		{"now playing", "setNowPlaying", `{"on":true}`, "", "SetNowPlaying on", nil},
		{"sound keeps the file", "setSound", `{"on":false}`, "", "SetSound off ", nil},
		{"sound file with the picker", "setSound", `{"on":true,"pick":true}`, "/tmp/ding.aiff", "SetSound on /tmp/ding.aiff", nil},
		{"max fps", "setMaxFps", `{"fps":3}`, "", "SetMaxFPS |||", nil},
		{"ignored projects", "setIgnoredProjects", `{"projects":["observer-sessions","scratch"]}`, "", "SetIgnoredProjects observer-sessions,scratch", nil},
		{"no ignored projects", "setIgnoredProjects", `{"projects":[]}`, "", "SetIgnoredProjects ", nil},
		{"remove", "removeExtension", `{"id":"acme.rain"}`, "", "RemoveExtension acme.rain", nil},
		{"notice", "notice", `{"extension":"builtin.aerials"}`, "", "Notice builtin.aerials", "notice for builtin.aerials"},
		{"connections", "connections", `{}`, "", "Connections now", []settings.Connection{{ID: "builtin.claude", State: settings.StateIdle}}},
		{"turn an agent off", "setAgentOn", `{"id":"builtin.codex","on":false}`, "", "SetAgentOn builtin.codex off", nil},
		{"plan adding hooks", "planAgentHooks", `{"id":"acme.agent","add":true}`, "", "PlanAgentHooks acme.agent add", planResult{
			Steps: []planLine{
				{Text: "Add hooks for Acme (from acme.agent 1.0.0)", Warning: true},
				{Text: "acme.agent runs its own program bin/convert on each Acme event", Warning: true},
			},
			Hash:     "hash-1",
			Warnings: []string{"acme.agent runs its own program bin/convert on each Acme event"},
		}},
		{"apply removing hooks", "applyAgentHooks", `{"id":"builtin.cursor","add":false,"hash":"hash-1"}`, "", "ApplyAgentHooks builtin.cursor remove hash-1", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, picker := &fakeService{}, &fakePicker{path: tt.pickerPath}
			window := &fakeWindow{calls: []call{{tt.method, tt.params}}}
			if err := Open(testPaths(t), "v9.9.9", service, picker, window); err != nil {
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
	if err := Open(p, "v9.9.9", &fakeService{}, &fakePicker{}, window); err != nil {
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
	err := Open(paths.ForHome(t.TempDir()), "v9.9.9", &fakeService{}, &fakePicker{}, &fakeWindow{})
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
			if err := Open(testPaths(t), "v9.9.9", service, &fakePicker{}, window); err != nil {
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

// warmUps runs calls and returns the warm-ups they started, waiting a
// little for the background ones.
func warmUps(t *testing.T, service *fakeService, calls ...call) []string {
	t.Helper()
	service.warmedUp = make(chan string, 4)
	window := &fakeWindow{calls: calls}
	if err := Open(testPaths(t), "v9.9.9", service, &fakePicker{path: "/tmp/rain.zip"}, window); err != nil {
		t.Fatal(err)
	}
	var started []string
	for {
		select {
		case warmUp := <-service.warmedUp:
			started = append(started, warmUp)
		case <-time.After(300 * time.Millisecond):
			return started
		}
	}
}

func TestWarmUpsFromTheWindowNeedTrust(t *testing.T) {
	tests := []struct {
		name   string
		review settings.InstallReview
		calls  []call
		want   []string
	}{
		{"a plain install runs nothing", settings.InstallReview{}, []call{{"installExtension", `{}`}}, nil},
		{"an approved upgrade runs the converters only if hooked", changedConverter,
			[]call{{"installExtension", `{}`}, {"confirmInstall", `{"hash":"review-1"}`}}, []string{"WarmUpIfHooked acme.rain"}},
		{"adding hooks runs the converters", settings.InstallReview{},
			[]call{{"applyAgentHooks", `{"id":"acme.agent","add":true,"hash":"hash-1"}`}}, []string{"WarmUpConverters acme.agent"}},
		{"removing hooks runs nothing", settings.InstallReview{},
			[]call{{"applyAgentHooks", `{"id":"acme.agent","add":false,"hash":"hash-1"}`}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := warmUps(t, &fakeService{review: tt.review}, tt.calls...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("warm-ups = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstallPickerOffersZipAndFolders(t *testing.T) {
	picker := &fakePicker{}
	window := &fakeWindow{calls: []call{{"installExtension", `{}`}}}
	if err := Open(testPaths(t), "v9.9.9", &fakeService{}, picker, window); err != nil {
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
			if err := Open(testPaths(t), "v9.9.9", &fakeService{err: tt.serviceErr}, &fakePicker{}, window); err != nil {
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
	if err := Open(p, "v9.9.9", &fakeService{}, &fakePicker{}, window); err != nil {
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
	if err := Open(p, "v9.9.9", &fakeService{}, &fakePicker{}, window); err != nil {
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
			if err := Open(testPaths(t), "v9.9.9", service, &fakePicker{path: tt.pickerPath}, window); err != nil {
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
	if err := Open(testPaths(t), "v9.9.9", service, picker, window); err != nil {
		t.Fatal(err)
	}
	if picker.nestedErr == nil || !strings.Contains(picker.nestedErr.Error(), "file dialog is open") {
		t.Fatalf("nested call error = %v, want a refusal", picker.nestedErr)
	}
	want := []string{"ReviewInstall /tmp/rain.zip", "InstallReviewed /tmp/rain.zip ", "SetPrivacy on"} // calls work again once it closes
	if !reflect.DeepEqual(service.changes(), want) {
		t.Fatalf("changes = %q, want %q", service.changes(), want)
	}
}

func TestBridgeAgentMethodsTakeIdsOnly(t *testing.T) {
	tests := []struct{ method, params string }{
		{"setAgentOn", `{"id":"builtin.codex","on":false,"path":"/Users/me/.ssh/config"}`},
		{"planAgentHooks", `{"id":"builtin.cursor","add":true,"path":"/Users/me/.zshrc"}`},
		{"applyAgentHooks", `{"id":"builtin.cursor","add":true,"hash":"hash-1","file":"/Users/me/.zshrc"}`},
		{"applyAgentHooks", `{"id":"builtin.cursor","add":true,"hash":"hash-1","command":"rm -rf ~"}`},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.params, func(t *testing.T) {
			service := &fakeService{}
			window := &fakeWindow{calls: []call{{tt.method, tt.params}}}
			if err := Open(testPaths(t), "v9.9.9", service, &fakePicker{}, window); err != nil {
				t.Fatal(err)
			}
			if window.errs[0] == nil || !strings.Contains(window.errs[0].Error(), "unknown field") {
				t.Fatalf("error = %v, want the extra field refused", window.errs[0])
			}
			if len(service.calls) != 0 {
				t.Fatalf("service calls = %q, want none: a path must have no effect", service.calls)
			}
		})
	}
}

func TestBridgeRefusesWhilePicking(t *testing.T) {
	for _, nested := range []call{
		{"applyAgentHooks", `{"id":"builtin.cursor","add":true,"hash":"hash-1"}`},
		{"setAgentOn", `{"id":"builtin.codex","on":false}`},
		{"planAgentHooks", `{"id":"builtin.cursor","add":true}`},
	} {
		t.Run(nested.method, func(t *testing.T) {
			service := &fakeService{}
			window := &fakeWindow{calls: []call{{"installExtension", `{}`}}}
			picker := &fakePicker{path: "/tmp/acme.zip", window: window, nested: &nested}
			if err := Open(testPaths(t), "v9.9.9", service, picker, window); err != nil {
				t.Fatal(err)
			}
			if picker.nestedErr == nil || !strings.Contains(picker.nestedErr.Error(), "file dialog is open") {
				t.Fatalf("nested call error = %v, want a refusal", picker.nestedErr)
			}
			if want := []string{"ReviewInstall /tmp/acme.zip", "InstallReviewed /tmp/acme.zip "}; !reflect.DeepEqual(service.changes(), want) {
				t.Fatalf("changes = %q, want %q", service.changes(), want)
			}
		})
	}
}

func TestDroppedFileInstallsOnce(t *testing.T) {
	drop := call{dropMethod, "/tmp/drop.zip"}
	installDropped := call{"installExtension", `{"dropped":true}`}
	tests := []struct {
		name      string
		service   *fakeService
		calls     []call
		want      []string
		wantDrops []bool
		wantPicks int
		wantErrs  []bool // one per page call; drops are not page calls
	}{
		{"a drop installs without the picker", &fakeService{}, []call{drop, installDropped},
			[]string{"ReviewInstall /tmp/drop.zip", "InstallReviewed /tmp/drop.zip "}, []bool{true}, 0, []bool{false}},
		{"a dropped pack installs as a pack", &fakeService{pack: true}, []call{drop, installDropped},
			[]string{"InstallPack /tmp/drop.zip"}, []bool{true}, 0, []bool{false}},
		{"a drop is used once", &fakeService{}, []call{drop, installDropped, installDropped},
			[]string{"ReviewInstall /tmp/drop.zip", "InstallReviewed /tmp/drop.zip "}, []bool{true}, 0, []bool{false, true}},
		{"no drop, nothing to install", &fakeService{}, []call{installDropped},
			nil, nil, 0, []bool{true}},
		{"the picker clears a drop", &fakeService{}, []call{drop, {"installExtension", `{}`}, installDropped},
			[]string{"ReviewInstall /tmp/rain.zip", "InstallReviewed /tmp/rain.zip "}, []bool{true}, 1, []bool{false, true}},
		{"a drop waits while a review is open", &fakeService{review: changedConverter}, []call{{"installExtension", `{}`}, drop, installDropped},
			[]string{"ReviewInstall /tmp/rain.zip"}, []bool{false}, 1, []bool{false, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			picker := &fakePicker{path: "/tmp/rain.zip"}
			window := &fakeWindow{calls: tt.calls}
			if err := Open(testPaths(t), "v9.9.9", tt.service, picker, window); err != nil {
				t.Fatal(err)
			}
			if got := tt.service.changes(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("calls = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(window.drops, tt.wantDrops) {
				t.Errorf("drops taken = %v, want %v", window.drops, tt.wantDrops)
			}
			if len(picker.requests) != tt.wantPicks {
				t.Errorf("picker shown %d times, want %d", len(picker.requests), tt.wantPicks)
			}
			var gotErrs []bool
			for _, err := range window.errs {
				gotErrs = append(gotErrs, err != nil)
			}
			if !reflect.DeepEqual(gotErrs, tt.wantErrs) {
				t.Errorf("errors = %v, want %v (%v)", gotErrs, tt.wantErrs, window.errs)
			}
		})
	}
}

func TestVersionIsTheOneOpenWasGiven(t *testing.T) {
	service := &fakeService{}
	window := &fakeWindow{calls: []call{{"version", ``}}}
	if err := Open(testPaths(t), "v9.9.9", service, &fakePicker{}, window); err != nil {
		t.Fatal(err)
	}
	if window.errs[0] != nil || window.results[0] != "v9.9.9" || len(service.calls) != 0 {
		t.Fatalf("version = %#v, %v, service calls %q; want v9.9.9 and no service call", window.results[0], window.errs[0], service.calls)
	}
}
