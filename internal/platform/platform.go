// Package platform holds the small interfaces for everything that depends on
// the operating system. Each OS adds its own files next to the macOS ones
// (suffix _darwin, _linux, _windows), so the daemon never imports OS code.
package platform

import "encoding/json"

// SaverWatcher says whether the screen saver is on screen right now.
type SaverWatcher interface {
	Active() bool
}

// SoundPlayer plays a sound file. Play must not block: it starts the sound
// and returns.
type SoundPlayer interface {
	Play(file string) error
}

// ServiceManager makes a process start now and at every login, for the
// daemon and later the tray. The macOS implementation comes with the install
// command.
type ServiceManager interface {
	// Install loads the service; its stderr goes to logFile.
	Install(label string, args []string, logFile string) (path string, err error)
	Uninstall(label string) error
}

// FileCloner makes dst a copy-on-write clone of src that shares its disk
// blocks. It never falls back to a real copy: when cloning is not possible
// (another volume, a file system without clones) it returns an error.
type FileCloner interface {
	Clone(src, dst string) error
}

// PageHandler answers one call from a page shown in a SettingsWindow: the
// method name and its JSON parameters in, a result to encode as JSON or an
// error out. It is called on the main thread, one call at a time.
type PageHandler func(method string, params json.RawMessage) (any, error)

// SettingsWindow shows a local web page in a native window and lets the page
// call Go. Run opens the page (a file inside readAccess, which is the only
// folder the page may load from), blocks until the user closes the window,
// and must be called from the main goroutine.
type SettingsWindow interface {
	Run(page, readAccess string, handle PageHandler) error
}

// FileRequest says what an open-file dialog offers.
type FileRequest struct {
	Prompt  string   // the dialog's message
	Types   []string // file name extensions without the dot; empty means any file
	Folders bool     // folders can be chosen too
}

// FilePicker shows the system's open-file dialog. ok is false when the user
// cancels.
type FilePicker interface {
	PickFile(request FileRequest) (path string, ok bool, err error)
}
