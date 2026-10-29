package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"agent-screensaver/internal/config"
	"agent-screensaver/internal/extensions"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/settings"
)

// commandHandler runs one subcommand and writes one line on success. Only
// a command that asks a question reads stdin.
type commandHandler func(service settings.Service, args []string, stdin io.Reader, stdout io.Writer) error

// settingsCommands maps each top-level settings command to its handler.
var settingsCommands = map[string]commandHandler{
	"theme":      runTheme,
	"background": runBackground,
	"option":     runOption,
	"privacy":    runPrivacy,
	"sound":      runSound,
	"fps":        runFPS,
	"ext":        runExt,
	"demo":       runDemo,
}

// IsSettingsCommand reports whether name is handled by Settings.
func IsSettingsCommand(name string) bool {
	_, ok := settingsCommands[name]
	return ok
}

// Settings runs one settings command (theme, background, option, privacy,
// sound, fps, ext or demo) and returns the exit code: 0 on success, 1 with a message on stderr
// when it fails. The logic lives in settings.Service; this is only parsing
// and printing.
func Settings(command string, args []string, stdin io.Reader, stdout, stderr io.Writer, p paths.Paths) int {
	handler, ok := settingsCommands[command]
	if !ok {
		fmt.Fprintf(stderr, "agent-screensaver: unknown command %q\n", command)
		return 1
	}
	err := handler(settings.Service{Paths: p}, args, stdin, stdout)
	if errors.Is(err, errCancelled) {
		return 1 // the question already said that nothing changed
	}
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver %s: %v\n", command, err)
		return 1
	}
	return 0
}

func usageError(usage string) error {
	return fmt.Errorf("usage: %s", usage)
}

func runTheme(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	switch {
	case len(args) == 1 && args[0] == "list":
		return listExtensions(service, stdout, "theme")
	case len(args) == 2 && args[0] == "set":
		if err := service.SetTheme(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "theme: %s\n", args[1])
		return nil
	}
	return usageError("theme list | theme set <id>")
}

// defaultBackground is the word that goes back to the theme's background.
const defaultBackground = "default"

func runBackground(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	switch {
	case len(args) == 1 && args[0] == "list":
		return listBackgrounds(service, stdout)
	case len(args) == 2 && args[0] == "set":
		id := args[1]
		if id == defaultBackground {
			id = ""
		}
		if err := service.SetBackground(id); err != nil {
			return err
		}
		if id == "" {
			fmt.Fprintln(stdout, "background: default (the theme's)")
		} else {
			fmt.Fprintf(stdout, "background: %s\n", id)
		}
		return nil
	}
	return usageError("background list | background set <id> | background set default")
}

// listBackgrounds prints one background id per line, with a "*" after the
// one in effect.
func listBackgrounds(service settings.Service, stdout io.Writer) error {
	snapshot, err := service.Snapshot()
	if err != nil {
		return err
	}
	for _, info := range snapshot.Extensions {
		if info.Manifest.Type != extensions.TypeBackground {
			continue
		}
		marker := ""
		if info.Manifest.ID == snapshot.Background {
			marker = " *"
		}
		fmt.Fprintf(stdout, "%s%s\n", info.Manifest.ID, marker)
	}
	return nil
}

func runOption(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	switch {
	case len(args) == 4 && args[0] == "set":
		return setOption(service, args[1], args[2], args[3], stdout)
	case (len(args) == 2 || len(args) == 3) && args[0] == "reset":
		key := ""
		if len(args) == 3 {
			key = args[2]
		}
		if err := service.ResetOption(args[1], key); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "option: %s %s reset\n", args[1], resetTarget(key))
		return nil
	}
	return usageError("option set <ext-id> <key> <value> | option reset <ext-id> [key]")
}

func resetTarget(key string) string {
	if key == "" {
		return "(all options)"
	}
	return key
}

// setOption stores the value, then reads it back so the line shows what was
// really stored (for a file option that is the copied path). A notice from
// the service, such as how to get aerial videos, follows on its own line.
func setOption(service settings.Service, extID, key, raw string, stdout io.Writer) error {
	if err := service.SetOption(extID, key, raw); err != nil {
		return err
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "option: %s %s = %v\n", extID, key, snapshot.Config.Options[extID][key])
	if notice := service.Notice(extID); notice != "" {
		fmt.Fprintln(stdout, notice)
	}
	return nil
}

func runPrivacy(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	on, err := parseOnOff(args, "privacy on|off")
	if err != nil {
		return err
	}
	if err := service.SetPrivacy(on); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "privacy: %s\n", onOff(on))
	return nil
}

func runSound(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	const usage = "sound on|off [file]"
	if len(args) < 1 || len(args) > 2 {
		return usageError(usage)
	}
	on, err := parseOnOff(args[:1], usage)
	if err != nil {
		return err
	}
	file := ""
	if len(args) == 2 {
		file = args[1]
	}
	if err := service.SetSound(on, file); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "sound: %s\n", onOff(on))
	return nil
}

func runFPS(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	const usage = "fps max <1..240|display>"
	if len(args) != 2 || args[0] != "max" {
		return usageError(usage)
	}
	fps := 0 // "display" means no ceiling beyond the display refresh rate
	if args[1] != "display" {
		parsed, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("%q is not a number or \"display\"; %s", args[1], usage)
		}
		fps = parsed
	}
	if err := service.SetMaxFPS(fps); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "fps max: %s\n", args[1])
	return nil
}

// runDemo arms the scripted demo, or stops it with "stop".
func runDemo(service settings.Service, args []string, _ io.Reader, stdout io.Writer) error {
	const usage = "demo [--seconds N] | demo stop"
	if len(args) == 1 && args[0] == "stop" {
		if err := service.StopDemo(); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "Demo stopped.")
		return nil
	}
	flags := flag.NewFlagSet("demo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	seconds := flags.Int("seconds", config.DemoDefaultSeconds, "")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return usageError(usage)
	}
	if err := service.ArmDemo(*seconds, time.Now().UnixMilli()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Demo ready. Start the screen saver now; it plays for %d seconds.\n", *seconds)
	return nil
}

func runExt(service settings.Service, args []string, stdin io.Reader, stdout io.Writer) error {
	switch {
	case len(args) == 1 && args[0] == "list":
		return listExtensions(service, stdout, "")
	case len(args) >= 2 && args[0] == "install":
		return installExtension(service, args[1:], stdin, stdout)
	case len(args) == 2 && args[0] == "remove":
		return removeTheme(service, args[1], stdout)
	case len(args) == 3 && args[0] == "remove" && args[1] == "--only":
		removal, err := service.RemoveOnly(args[2])
		return printRemoval(stdout, removal, err)
	}
	return usageError(extUsage)
}

// removeTheme removes an extension, and for a theme also the parts it
// brought that nothing else uses.
func removeTheme(service settings.Service, id string, stdout io.Writer) error {
	removal, err := service.RemoveTheme(id)
	return printRemoval(stdout, removal, err)
}

// printRemoval prints one line per removed id, then one per part that
// stays. The removed lines and the theme line come even when a later
// remove failed, so the user sees what is gone, and err is returned after.
func printRemoval(stdout io.Writer, removal settings.Removal, err error) error {
	for _, removed := range removal.Removed {
		fmt.Fprintf(stdout, "removed: %s\n", removed)
	}
	if err == nil {
		for _, kept := range removal.Kept {
			fmt.Fprintf(stdout, "kept: %s (used by %s)\n", kept.ID, kept.UsedBy)
		}
	}
	if removal.ThemeReset {
		fmt.Fprintln(stdout, "The removed theme was in use, so the screen saver now uses the default theme.")
	}
	return err
}

// listExtensions prints one "<id> <type>" line per extension, with a "*"
// after the active theme. onlyType filters by type; empty lists everything.
func listExtensions(service settings.Service, stdout io.Writer, onlyType string) error {
	snapshot, err := service.Snapshot()
	if err != nil {
		return err
	}
	for _, info := range snapshot.Extensions {
		kind := string(info.Manifest.Type)
		if onlyType != "" && kind != onlyType {
			continue
		}
		marker := ""
		if info.Manifest.ID == snapshot.Config.Theme && kind == "theme" {
			marker = " *"
		}
		fmt.Fprintf(stdout, "%s %s%s\n", info.Manifest.ID, kind, marker)
	}
	return nil
}

func parseOnOff(args []string, usage string) (bool, error) {
	if len(args) != 1 {
		return false, usageError(usage)
	}
	switch strings.ToLower(args[0]) {
	case "on":
		return true, nil
	case "off":
		return false, nil
	}
	return false, usageError(usage)
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}
