// Package paths says where every agent-screensaver file lives. The screen
// saver can read the data directory but not write it (docs/decisions/0001),
// so only the daemon writes here. It also says where macOS keeps the aerial
// videos we read.
package paths

import (
	"path/filepath"

	"agent-screensaver/internal/aerials"
)

// Paths holds the absolute location of each file and folder.
type Paths struct {
	// Home is the user's home folder the other paths were built from.
	Home          string
	DataDir       string
	SpoolDir      string
	StateFile     string
	ConfigFile    string
	ExtensionsDir string
	RuntimeDir    string
	MediaDir      string
	InstallRecord string
	// DaemonLog is where launchd sends the service's stderr.
	DaemonLog string
	// TrayLog is where launchd sends the menu bar app's stderr.
	TrayLog string
	// UsageFile keeps what the Claude limits estimate learned.
	UsageFile string
	// ContextSizesFile keeps the context window size of each model.
	ContextSizesFile string

	// MarksDir holds the user's own agent marks, <source>.svg, which
	// replace the mark of that agent's extension.
	MarksDir string

	// AerialClonesDir holds APFS clones of the downloaded aerials, because
	// the saver's web view can only load files inside DataDir.
	AerialClonesDir string
	Aerials         aerials.Locations
}

// ForHome builds the paths under the data directory of the given home. The
// caller resolves the real home: inside the saver sandbox the usual
// home lookup returns the container, not the real home.
func ForHome(home string) Paths {
	dataDir := filepath.Join(home, "Library", "Application Support", "agent-screensaver")
	mediaDir := filepath.Join(dataDir, "media")
	return Paths{
		Home:             home,
		DataDir:          dataDir,
		SpoolDir:         filepath.Join(dataDir, "spool"),
		StateFile:        filepath.Join(dataDir, "state.json"),
		ConfigFile:       filepath.Join(dataDir, "config.json"),
		ExtensionsDir:    filepath.Join(dataDir, "extensions"),
		RuntimeDir:       filepath.Join(dataDir, "runtime"),
		MediaDir:         mediaDir,
		InstallRecord:    filepath.Join(dataDir, "install-record.json"),
		DaemonLog:        filepath.Join(dataDir, "daemon.log"),
		TrayLog:          filepath.Join(dataDir, "tray.log"),
		UsageFile:        filepath.Join(dataDir, "claude-usage.json"),
		ContextSizesFile: filepath.Join(dataDir, "context-windows.json"),

		MarksDir:        filepath.Join(dataDir, "marks"),
		AerialClonesDir: filepath.Join(mediaDir, "aerials"),
		Aerials:         aerials.LocationsForHome(home),
	}
}
