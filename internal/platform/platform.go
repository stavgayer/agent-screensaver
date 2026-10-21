// Package platform holds the small interfaces for everything that depends on
// the operating system. Each OS adds its own files next to the macOS ones
// (suffix _darwin, _linux, _windows), so the daemon never imports OS code.
package platform

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
	Install(label string, args []string) (path string, err error)
	Uninstall(label string) error
}

// FileCloner makes dst a copy-on-write clone of src that shares its disk
// blocks. It never falls back to a real copy: when cloning is not possible
// (another volume, a file system without clones) it returns an error.
type FileCloner interface {
	Clone(src, dst string) error
}
