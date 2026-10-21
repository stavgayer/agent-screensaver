package tray

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/settings"
	"agent-screensaver/internal/statefile"
)

// Both icons are template images (black with alpha), so macOS tints them for
// light and dark menu bars. The attention one adds a dot in the corner.
var (
	//go:embed icons/normal.png
	normalIcon []byte
	//go:embed icons/attention.png
	attentionIcon []byte
)

const refreshEvery = 2 * time.Second

// Run shows the menu bar icon and blocks until the user quits or ctx ends. It
// must run on the main goroutine (the platform package locks it there).
//
// systray does not call its exit callback on macOS (fyne-io/systray#128), so
// nothing here depends on it: the loop is stopped before systray.Quit.
func Run(ctx context.Context, p paths.Paths, svc settings.Service, openSettings func() error) error {
	app := &app{paths: p, svc: svc, openSettings: openSettings, state: stateReader{path: p.StateFile}}
	loopCtx, stopLoop := context.WithCancel(ctx)
	app.stopLoop = stopLoop
	go func() {
		<-loopCtx.Done()
		systray.Quit()
	}()
	systray.Run(func() { go app.loop(loopCtx) }, nil)
	stopLoop()
	return nil
}

type app struct {
	paths        paths.Paths
	svc          settings.Service
	openSettings func() error
	state        stateReader
	stopLoop     context.CancelFunc

	menu    menuView
	current Model
	clicks  chan string
}

// loop rebuilds the model every refreshEvery and after every click, and
// touches the system menu only when the model changed.
func (a *app) loop(ctx context.Context) {
	a.clicks = make(chan string)
	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()
	a.refresh()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case id := <-a.clicks:
			a.handle(id)
		}
		a.refresh()
	}
}

func (a *app) refresh() {
	model := BuildMenu(a.state.read(), config.Load(a.paths.ConfigFile), time.Now().UnixMilli())
	if reflect.DeepEqual(model, a.current) {
		return
	}
	a.menu.apply(a.current, model, a.clicks)
	a.current = model
	log.Printf("tray: menu attention=%t items=%s", model.Attention, describe(model.Items))
}

func describe(items []Item) string {
	var labels []string
	for _, item := range items {
		if item.Label != "" {
			labels = append(labels, fmt.Sprintf("%q", item.Label))
		}
	}
	return strings.Join(labels, ", ")
}

func (a *app) handle(id string) {
	cfg := config.Load(a.paths.ConfigFile)
	var err error
	switch id {
	case IDPreview:
		err = exec.Command("open", "-a", "ScreenSaverEngine").Run()
	case IDSettings:
		err = a.openSettings()
	case IDSound:
		err = a.svc.SetSound(!cfg.Sound, "")
	case IDPrivacy:
		err = a.svc.SetPrivacy(!cfg.Privacy)
	case IDQuit:
		a.stopLoop() // systray.Quit runs from the goroutine started in Run
	}
	if err != nil {
		log.Printf("tray: %s: %v", id, err)
	}
}

// stateReader returns the last good state file. It reads the file again only
// when its modification time changed, so the 2 second refresh costs one stat.
type stateReader struct {
	path    string
	modTime time.Time
	file    statefile.StateFile
}

func (r *stateReader) read() statefile.StateFile {
	info, err := os.Stat(r.path)
	if errors.Is(err, fs.ErrNotExist) {
		r.modTime, r.file = time.Time{}, statefile.StateFile{}
		return r.file
	}
	if err != nil || info.ModTime().Equal(r.modTime) {
		return r.file
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		return r.file
	}
	var file statefile.StateFile
	if err := json.Unmarshal(data, &file); err != nil {
		return r.file // the daemon writes atomically; keep the last good one anyway
	}
	r.modTime, r.file = info.ModTime(), file
	return file
}

// menuView is the glue to systray. It keeps the system items of the model on
// screen and changes them in place when only text or checks changed, because
// rebuilding a menu the user has open makes it flicker.
type menuView struct {
	items []*systray.MenuItem // nil for a separator
}

func (v *menuView) apply(old, model Model, clicks chan<- string) {
	if len(v.items) == 0 || model.Attention != old.Attention {
		icon := normalIcon
		if model.Attention {
			icon = attentionIcon
		}
		systray.SetTemplateIcon(icon, icon)
	}
	if sameShape(old, model) && len(v.items) == len(model.Items) {
		for i, item := range model.Items {
			update(v.items[i], item)
		}
		return
	}
	systray.ResetMenu() // closes every ClickedCh, which ends the forwarders below
	v.items = v.items[:0]
	for _, item := range model.Items {
		v.items = append(v.items, add(item, clicks))
	}
}

// sameShape is true when both menus have the same lines of the same kind, so
// one can be turned into the other by changing items in place.
func sameShape(a, b Model) bool {
	if len(a.Items) != len(b.Items) {
		return false
	}
	for i := range a.Items {
		if a.Items[i].ID != b.Items[i].ID || (a.Items[i].Label == "") != (b.Items[i].Label == "") {
			return false
		}
	}
	return true
}

func add(item Item, clicks chan<- string) *systray.MenuItem {
	if item.ID == "" && item.Label == "" {
		systray.AddSeparator()
		return nil
	}
	var entry *systray.MenuItem
	if item.ID == IDSound || item.ID == IDPrivacy {
		entry = systray.AddMenuItemCheckbox(item.Label, "", item.Checked)
	} else {
		entry = systray.AddMenuItem(item.Label, "")
	}
	if !item.Enabled {
		entry.Disable()
	}
	if item.ID != "" {
		go forward(entry, item.ID, clicks)
	}
	return entry
}

func forward(entry *systray.MenuItem, id string, clicks chan<- string) {
	for range entry.ClickedCh {
		clicks <- id
	}
}

func update(entry *systray.MenuItem, item Item) {
	if entry == nil {
		return // a separator
	}
	entry.SetTitle(item.Label)
	if item.Checked {
		entry.Check()
	} else {
		entry.Uncheck()
	}
}

// NewSettingsLauncher returns a function that starts "<executable> settings"
// as a child process, at most one at a time: while one runs, it does nothing.
func NewSettingsLauncher(executable string) func() error {
	var mutex sync.Mutex
	running := false
	return func() error {
		mutex.Lock()
		defer mutex.Unlock()
		if running {
			return nil
		}
		child := exec.Command(executable, "settings")
		if err := child.Start(); err != nil {
			return fmt.Errorf("start settings window: %w", err)
		}
		running = true
		go func() {
			_ = child.Wait()
			mutex.Lock()
			running = false
			mutex.Unlock()
		}()
		return nil
	}
}
