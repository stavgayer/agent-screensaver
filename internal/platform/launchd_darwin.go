package platform

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Launchd runs a process as a launchd user agent: started at login and kept
// alive. Run executes a command and Sleep waits; tests replace both so no
// real launchctl runs and no real time passes.
type Launchd struct {
	Home  string
	Run   func(name string, args ...string) error
	Sleep func(time.Duration)
}

// After bootout, launchd may still be tearing the service down, and a
// bootstrap in that window fails with error 5. We wait until `print` says the
// service is gone, then retry bootstrap a few times before giving up.
const (
	goneCheckInterval = 100 * time.Millisecond
	goneCheckAttempts = 30
	bootstrapAttempts = 5
	bootstrapBackoff  = 750 * time.Millisecond
)

// NewLaunchd returns a Launchd that runs the real launchctl.
func NewLaunchd(home string) Launchd {
	return Launchd{Home: home, Run: runCommand, Sleep: time.Sleep}
}

func runCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, bytes.TrimSpace(output))
	}
	return nil
}

func (l Launchd) plistPath(label string) string {
	return filepath.Join(l.Home, "Library", "LaunchAgents", label+".plist")
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// Start starts the loaded service with this label if it is not running. It
// never restarts a running one (kickstart without -k). A service that is not
// loaded, because agent-screensaver is not installed, is an error.
func (l Launchd) Start(label string) error {
	return l.Run("launchctl", "kickstart", domain()+"/"+label)
}

// Install writes the plist and loads it. args is the full command line;
// the service's stderr goes to logFile.
func (l Launchd) Install(label string, args []string, logFile string) (string, error) {
	plist := l.plistPath(label)
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		return "", fmt.Errorf("create LaunchAgents folder: %w", err)
	}
	if err := os.WriteFile(plist, renderPlist(label, args, logFile), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", plist, err)
	}
	if err := l.unloadIfLoaded(label); err != nil {
		return "", err
	}
	if err := l.bootstrap(plist); err != nil {
		return "", fmt.Errorf("load %s: %w\nif it still fails, run: launchctl bootstrap gui/$(id -u) %s (or re-run install)", label, err, plist)
	}
	return plist, nil
}

// bootstrap loads the plist, retrying with a backoff. The error is the last
// failure.
func (l Launchd) bootstrap(plist string) error {
	var err error
	for attempt := 1; attempt <= bootstrapAttempts; attempt++ {
		if err = l.Run("launchctl", "bootstrap", domain(), plist); err == nil {
			return nil
		}
		if attempt < bootstrapAttempts {
			l.Sleep(bootstrapBackoff)
		}
	}
	return err
}

// Uninstall unloads the service if it is loaded and removes the plist.
func (l Launchd) Uninstall(label string) error {
	if err := l.unloadIfLoaded(label); err != nil {
		return err
	}
	if err := os.Remove(l.plistPath(label)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove plist: %w", err)
	}
	return nil
}

// unloadIfLoaded asks launchctl first, because bootout of a service that is
// not loaded fails and that is not an error for us.
func (l Launchd) unloadIfLoaded(label string) error {
	target := domain() + "/" + label
	if l.Run("launchctl", "print", target) != nil {
		return nil
	}
	if err := l.Run("launchctl", "bootout", target); err != nil {
		return fmt.Errorf("unload %s: %w", label, err)
	}
	l.waitUntilGone(target)
	return nil
}

// waitUntilGone polls until `print` fails, which means launchd no longer
// knows the service. If it never does, we carry on: the bootstrap retries
// cover the rest and report a real failure.
func (l Launchd) waitUntilGone(target string) {
	for attempt := 0; attempt < goneCheckAttempts; attempt++ {
		if l.Run("launchctl", "print", target) != nil {
			return
		}
		l.Sleep(goneCheckInterval)
	}
}

// trayLabelSuffix marks the menu bar app's service. Its Quit item exits with
// code 0, which must stay quit, so launchd restarts it only after a crash.
// The platform package cannot import install, so it recognises the label by
// its end.
const trayLabelSuffix = ".tray"

// keepAliveValue is the plist value of the KeepAlive key: always for the
// daemon, only after a non-zero exit for the tray.
func keepAliveValue(label string) string {
	if strings.HasSuffix(label, trayLabelSuffix) {
		return "\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>"
	}
	return "\t<true/>"
}

func renderPlist(label string, args []string, logFile string) []byte {
	var out bytes.Buffer
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + escapeXML(label) + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, arg := range args {
		out.WriteString("\t\t<string>" + escapeXML(arg) + "</string>\n")
	}
	out.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
` + keepAliveValue(label) + `
	<key>ProcessType</key>
	<string>Background</string>
	<key>LowPriorityIO</key>
	<true/>
	<key>StandardErrorPath</key>
	<string>` + escapeXML(logFile) + `</string>
</dict>
</plist>
`)
	return out.Bytes()
}

func escapeXML(text string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(text)) // writing to a buffer cannot fail
	return out.String()
}
