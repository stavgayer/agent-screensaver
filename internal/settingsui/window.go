// Package settingsui is the settings window: it opens the settings page in a
// platform.SettingsWindow and answers the page's calls. Each call maps onto
// one settings.Service method, so no settings rule lives here.
package settingsui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
	"agent-screensaver/internal/settings"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/statefile"
	"agent-screensaver/internal/view"
)

// Service is the part of settings.Service the window uses.
type Service interface {
	Snapshot() (settings.Snapshot, error)
	SetTheme(id string) error
	SetBackground(id string) error
	SetOption(extID, key, raw string) error
	ResetOption(extID, key string) error
	SetPrivacy(on bool) error
	SetSound(on bool, file string) error
	SetMaxFPS(fps int) error
	SetIgnoredProjects(projects []string) error
	InstallExtension(path string) (extensions.Manifest, error)
	RemoveExtension(id string) error
	Notice(extID string) string
}

// pickResult answers a call that shows a file dialog first.
type pickResult struct {
	Cancelled bool                 `json:"cancelled"`
	Manifest  *extensions.Manifest `json:"manifest,omitempty"`
}

var (
	extensionFiles = platform.FileRequest{Prompt: "Choose an extension folder or .zip file", Types: []string{"zip"}, Folders: true}
	soundFiles     = platform.FileRequest{Prompt: "Choose the attention sound", Types: []string{"aiff", "aif", "wav", "mp3", "m4a", "caf"}}
	optionFiles    = platform.FileRequest{Prompt: "Choose a file"}
)

// Open shows the settings page (runtime/settings/index.html) and blocks
// until the window closes. The page may read the whole data directory: the
// preview loads the runtime and the extensions from there.
func Open(p paths.Paths, svc Service, picker platform.FilePicker, window platform.SettingsWindow) error {
	page := filepath.Join(p.RuntimeDir, "settings", "index.html")
	if _, err := os.Stat(page); err != nil {
		return fmt.Errorf("settings page is missing, run agent-screensaver install: %w", err)
	}
	b := &bridge{svc: svc, picker: picker, paths: p, now: func() int64 { return time.Now().UnixMilli() }}
	return window.Run(page, p.DataDir, b.handle)
}

type bridge struct {
	svc    Service
	picker platform.FilePicker
	paths  paths.Paths
	now    func() int64 // milliseconds, for the idle state
	// picking is true while the open panel is up. Its modal loop still
	// delivers the page's messages, so a call could arrive in the middle of
	// another; those calls are refused instead of nesting.
	picking bool
}

func (b *bridge) handle(method string, params json.RawMessage) (any, error) {
	if b.picking {
		return nil, errors.New("a file dialog is open")
	}
	call, ok := b.methods()[method]
	if !ok {
		return nil, fmt.Errorf("unknown method %q", method)
	}
	// Service errors already say what is wrong; they go to the page as they are.
	return call(params)
}

type method func(params json.RawMessage) (any, error)

// methods maps each page call onto the service. A setter answers null.
func (b *bridge) methods() map[string]method {
	return map[string]method{
		"snapshot": func(json.RawMessage) (any, error) { return b.svc.Snapshot() },
		"setTheme": withParams(func(p struct{ ID string }) (any, error) { return nil, b.svc.SetTheme(p.ID) }),
		"setBackground": withParams(func(p struct{ ID string }) (any, error) {
			return nil, b.svc.SetBackground(p.ID)
		}),
		"setOption": withParams(b.setOption),
		"resetOption": withParams(func(p struct{ Extension, Key string }) (any, error) {
			return nil, b.svc.ResetOption(p.Extension, p.Key)
		}),
		"setPrivacy": withParams(func(p struct{ On bool }) (any, error) { return nil, b.svc.SetPrivacy(p.On) }),
		"setSound":   withParams(b.setSound),
		"setMaxFps":  withParams(func(p struct{ FPS int }) (any, error) { return nil, b.svc.SetMaxFPS(p.FPS) }),
		"setIgnoredProjects": withParams(func(p struct{ Projects []string }) (any, error) {
			return nil, b.svc.SetIgnoredProjects(p.Projects)
		}),
		"installExtension": withParams(func(struct{}) (any, error) { return b.installExtension() }),
		"removeExtension": withParams(func(p struct{ ID string }) (any, error) {
			return nil, b.svc.RemoveExtension(p.ID)
		}),
		"notice": withParams(func(p struct{ Extension string }) (any, error) { return b.svc.Notice(p.Extension), nil }),
		"state":  func(json.RawMessage) (any, error) { return b.state() },
	}
}

type optionParams struct {
	Extension, Key, Value string
	Pick                  bool // choose Value with the file dialog
}

// setOption sets a file option only with a path the user chose in the file
// dialog. The settings service copies the file into the data directory, which
// the screen saver can read, so a typed path must never reach it.
func (b *bridge) setOption(p optionParams) (any, error) {
	isFile, err := b.isFileOption(p.Extension, p.Key)
	if err != nil {
		return nil, err
	}
	if isFile && !p.Pick {
		return nil, fmt.Errorf("option %s %s is a file: choose it in the file dialog", p.Extension, p.Key)
	}
	if !isFile && p.Pick {
		return nil, fmt.Errorf("option %s %s is not a file option", p.Extension, p.Key)
	}
	if p.Pick {
		path, ok, err := b.pick(optionFiles)
		if err != nil || !ok {
			return pickResult{Cancelled: err == nil}, err
		}
		p.Value = path
	}
	return nil, b.svc.SetOption(p.Extension, p.Key, p.Value)
}

type soundParams struct {
	On   bool
	Pick bool // also choose a new sound file
}

func (b *bridge) setSound(p soundParams) (any, error) {
	file := ""
	if p.Pick {
		path, ok, err := b.pick(soundFiles)
		if err != nil || !ok {
			return pickResult{Cancelled: err == nil}, err
		}
		file = path
	}
	return nil, b.svc.SetSound(p.On, file)
}

func (b *bridge) installExtension() (any, error) {
	path, ok, err := b.pick(extensionFiles)
	if err != nil || !ok {
		return pickResult{Cancelled: err == nil}, err
	}
	manifest, err := b.svc.InstallExtension(path)
	if err != nil {
		return nil, err
	}
	return pickResult{Manifest: &manifest}, nil
}

// isFileOption reports whether the option is of kind file. An option the
// snapshot does not know is not a file; the service reports it as unknown.
func (b *bridge) isFileOption(extID, key string) (bool, error) {
	snapshot, err := b.svc.Snapshot()
	if err != nil {
		return false, err
	}
	for _, ext := range snapshot.Extensions {
		if ext.Manifest.ID == extID {
			return ext.Manifest.Options[key].Kind == extensions.OptionFile, nil
		}
	}
	return false, nil
}

// pick shows the file dialog with other calls held off until it closes.
func (b *bridge) pick(request platform.FileRequest) (string, bool, error) {
	b.picking = true
	defer func() { b.picking = false }()
	return b.picker.PickFile(request)
}

// state returns state.json as text, ready for the runtime's setState. When
// the daemon has not written one, it is an idle screen built from the
// settings, so the preview still shows the background and the clock.
func (b *bridge) state() (any, error) {
	data, err := os.ReadFile(b.paths.StateFile)
	if errors.Is(err, fs.ErrNotExist) {
		return b.idleState()
	}
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// idleState is what the daemon would write with no agent activity, marked as
// from a stopped service. Without the daemon there are no aerial clones, so
// the aerials background falls back to Matrix, as on the real screen.
func (b *bridge) idleState() (any, error) {
	cfg := config.Load(b.paths.ConfigFile)
	resolved := view.Resolve(cfg, extensions.Scan(b.paths.ExtensionsDir), nil)
	file := statefile.Build(state.State{}, resolved, b.now())
	file.ServiceOK = false
	data, err := json.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("encode idle state: %w", err)
	}
	return string(data), nil
}

// withParams decodes the call's parameters into P before calling call.
// Missing parameters are the zero value; unknown ones are refused, so a typo
// in the page fails loudly instead of doing the wrong thing.
func withParams[P any](call func(P) (any, error)) method {
	return func(raw json.RawMessage) (any, error) {
		var params P
		if len(bytes.TrimSpace(raw)) > 0 && string(raw) != "null" {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&params); err != nil {
				return nil, fmt.Errorf("parameters: %w", err)
			}
		}
		return call(params)
	}
}
