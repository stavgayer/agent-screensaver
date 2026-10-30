package install

import (
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	// LauncherBundleID is the bundle id of the launcher app. It is also how
	// install and uninstall tell our app from one that is not ours.
	LauncherBundleID = "dev.agentscreensaver.launcher"

	launcherName       = "Agent Screensaver.app"
	launcherExecutable = "Agent Screensaver"
	lsregisterPath     = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// launcherIcon is the Dock and Finder icon of the launcher app. Make it again
// with scripts/make-app-icon.sh after internal/platform/appicon.png changes.
//
//go:embed AppIcon.icns
var launcherIcon []byte

// Options are the parts of install that the caller sets.
type Options struct {
	// Version goes into the launcher app's Info.plist. Empty means "dev".
	Version string
	// Register tells Launch Services about the launcher app once it exists,
	// so Spotlight finds it at once. A failure is ignored. Nil skips it.
	Register func(app string)
}

// RegisterLauncher asks Launch Services to index the app. Best effort: when
// lsregister is missing or fails, Spotlight finds the app a little later.
func RegisterLauncher(app string) {
	_ = exec.Command(lsregisterPath, "-f", app).Run()
}

// launcherState says what is at the launcher path.
type launcherState int

const (
	launcherAbsent  launcherState = iota // nothing there
	launcherOurs                         // our app, to replace or remove
	launcherForeign                      // anything else, never touched
)

// launcherStateOf reads what is at app. A file, a link, or a folder whose
// Info.plist is missing, unreadable or has another bundle id is not ours.
func launcherStateOf(app string) launcherState {
	info, err := os.Lstat(app)
	if errors.Is(err, os.ErrNotExist) {
		return launcherAbsent
	}
	if err != nil || !info.IsDir() {
		return launcherForeign
	}
	id, err := bundleID(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil || id != LauncherBundleID {
		return launcherForeign
	}
	return launcherOurs
}

// bundleID reads CFBundleIdentifier from an XML Info.plist. A binary plist
// cannot be read here, so it counts as not ours.
func bundleID(plistPath string) (string, error) {
	file, err := os.Open(plistPath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	decoder := xml.NewDecoder(file)
	decoder.Strict = false
	wantValue := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return "", errors.New("no CFBundleIdentifier")
		}
		if err != nil {
			return "", err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		var text string
		switch start.Name.Local {
		case "key":
			if err := decoder.DecodeElement(&text, &start); err != nil {
				return "", err
			}
			wantValue = text == "CFBundleIdentifier"
		case "string":
			if err := decoder.DecodeElement(&text, &start); err != nil {
				return "", err
			}
			if wantValue {
				return text, nil
			}
		default:
			wantValue = false
		}
	}
}

// launcherInfoPlist is the Info.plist of the launcher app. LSUIElement keeps
// the launcher out of the Dock: the settings window shows its own icon.
func launcherInfoPlist(version string) string {
	if version == "" {
		version = "dev"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>%s</string>
	<key>CFBundleName</key>
	<string>Agent Screensaver</string>
	<key>CFBundleDisplayName</key>
	<string>Agent Screensaver</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleExecutable</key>
	<string>%s</string>
	<key>CFBundleIconFile</key>
	<string>AppIcon</string>
	<key>CFBundleShortVersionString</key>
	<string>%s</string>
	<key>CFBundleVersion</key>
	<string>%s</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>LSMinimumSystemVersion</key>
	<string>14.0</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`, LauncherBundleID, launcherExecutable, html.EscapeString(version), html.EscapeString(version))
}

// launcherScript runs the installed binary, the one launchd runs. The path
// holds a space ("Application Support"), so it is quoted. exec keeps the
// process the one Launch Services started, so a second click on the app
// finds the running one.
func launcherScript(bin string) string {
	return "#!/bin/sh\nexec " + shellQuote(bin) + " settings\n"
}

// installLauncher creates the launcher app, or replaces ours. It leaves an
// app that is not ours alone. The app is built in a hidden folder next to
// the final place and moved in whole, so a failure never leaves half an app.
func installLauncher(where locations, opts Options) error {
	if launcherStateOf(where.launcher) == launcherForeign {
		return nil
	}
	parent := filepath.Dir(where.launcher)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	stage, err := os.MkdirTemp(parent, ".agent-screensaver-launcher-")
	if err != nil {
		return fmt.Errorf("create launcher app: %w", err)
	}
	defer os.RemoveAll(stage)
	app := filepath.Join(stage, launcherName)
	files := []struct {
		path string
		data []byte
		perm os.FileMode
	}{
		{filepath.Join("Contents", "Info.plist"), []byte(launcherInfoPlist(opts.Version)), 0o644},
		{filepath.Join("Contents", "MacOS", launcherExecutable), []byte(launcherScript(where.bin)), 0o755},
		{filepath.Join("Contents", "Resources", "AppIcon.icns"), launcherIcon, 0o644},
	}
	for _, file := range files {
		if err := writeFileAtomic(filepath.Join(app, file.path), file.data, file.perm); err != nil {
			return fmt.Errorf("create launcher app: %w", err)
		}
	}
	if err := os.RemoveAll(where.launcher); err != nil {
		return fmt.Errorf("replace %s: %w", where.launcher, err)
	}
	if err := os.Rename(app, where.launcher); err != nil {
		return fmt.Errorf("create launcher app: %w", err)
	}
	if opts.Register != nil {
		opts.Register(where.launcher)
	}
	return nil
}

// removeLauncher deletes the launcher app when it is ours.
func removeLauncher(where locations) error {
	if launcherStateOf(where.launcher) != launcherOurs {
		return nil
	}
	if err := os.RemoveAll(where.launcher); err != nil {
		return fmt.Errorf("remove %s: %w", where.launcher, err)
	}
	return nil
}

// planLauncher is the install plan line for the launcher app.
func planLauncher(where locations) PlannedStep {
	switch launcherStateOf(where.launcher) {
	case launcherForeign:
		return PlannedStep{Name: "launcher", Warning: true, Description: "Skip the launcher app: " + where.launcher +
			" already exists and is not ours, so it stays as it is"}
	case launcherOurs:
		return PlannedStep{Name: "launcher", Description: "Replace the launcher app at " + where.launcher +
			" (Spotlight and Launchpad find it as Agent Screensaver)"}
	}
	description := "Create the launcher app at " + where.launcher + " (Spotlight and Launchpad find it as Agent Screensaver)"
	if _, err := os.Stat(filepath.Dir(where.launcher)); err != nil {
		description += "; the folder " + filepath.Dir(where.launcher) + " does not exist yet and is created too"
	}
	return PlannedStep{Name: "launcher", Description: description}
}

// planRemoveLauncher is the uninstall plan line for the launcher app, empty
// when there is no app at the path.
func planRemoveLauncher(where locations) []PlannedStep {
	switch launcherStateOf(where.launcher) {
	case launcherOurs:
		return []PlannedStep{{Name: "launcher", Description: "Delete the launcher app " + where.launcher}}
	case launcherForeign:
		return []PlannedStep{{Name: "launcher", Warning: true, Description: "Leave " + where.launcher +
			" where it is: it is not the launcher app of agent-screensaver"}}
	}
	return nil
}

// LauncherInstalled reports whether our launcher app is in place in home,
// so install only points to Spotlight when there is something to find.
func LauncherInstalled(home string) bool {
	return launcherStateOf(locationsFor(home).launcher) == launcherOurs
}

// InstalledBinary is the path of the binary that install copies and launchd
// runs, which stays after the downloaded folder is gone.
func InstalledBinary(home string) string {
	return locationsFor(home).bin
}
