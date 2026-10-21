// Package paths says where every agent-screensaver file lives. The screen
// saver can read the data directory but not write it (docs/decisions/0001),
// so only the daemon writes here.
package paths

import "path/filepath"

// Paths holds the absolute location of each file and folder.
type Paths struct {
	DataDir       string
	SpoolDir      string
	StateFile     string
	ConfigFile    string
	ExtensionsDir string
	RuntimeDir    string
	MediaDir      string
	InstallRecord string
}

// ForHome builds the paths under the data directory of the given home. The
// caller resolves the real home: inside the saver sandbox the usual
// home lookup returns the container, not the real home.
func ForHome(home string) Paths {
	dataDir := filepath.Join(home, "Library", "Application Support", "agent-screensaver")
	return Paths{
		DataDir:       dataDir,
		SpoolDir:      filepath.Join(dataDir, "spool"),
		StateFile:     filepath.Join(dataDir, "state.json"),
		ConfigFile:    filepath.Join(dataDir, "config.json"),
		ExtensionsDir: filepath.Join(dataDir, "extensions"),
		RuntimeDir:    filepath.Join(dataDir, "runtime"),
		MediaDir:      filepath.Join(dataDir, "media"),
		InstallRecord: filepath.Join(dataDir, "install-record.json"),
	}
}
