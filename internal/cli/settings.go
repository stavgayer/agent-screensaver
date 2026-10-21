package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/settings"
)

// commandHandler runs one subcommand and writes one line on success.
type commandHandler func(service settings.Service, args []string, stdout io.Writer) error

// settingsCommands maps each top-level settings command to its handler.
var settingsCommands = map[string]commandHandler{
	"theme":   runTheme,
	"option":  runOption,
	"privacy": runPrivacy,
	"sound":   runSound,
	"fps":     runFPS,
	"ext":     runExt,
}

// IsSettingsCommand reports whether name is handled by Settings.
func IsSettingsCommand(name string) bool {
	_, ok := settingsCommands[name]
	return ok
}

// Settings runs one settings command (theme, option, privacy, sound, fps or
// ext) and returns the exit code: 0 on success, 1 with a message on stderr
// when it fails. The logic lives in settings.Service; this is only parsing
// and printing.
func Settings(command string, args []string, stdout, stderr io.Writer, p paths.Paths) int {
	handler, ok := settingsCommands[command]
	if !ok {
		fmt.Fprintf(stderr, "agent-screensaver: unknown command %q\n", command)
		return 1
	}
	if err := handler(settings.Service{Paths: p}, args, stdout); err != nil {
		fmt.Fprintf(stderr, "agent-screensaver %s: %v\n", command, err)
		return 1
	}
	return 0
}

func usageError(usage string) error {
	return fmt.Errorf("usage: %s", usage)
}

func runTheme(service settings.Service, args []string, stdout io.Writer) error {
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

func runOption(service settings.Service, args []string, stdout io.Writer) error {
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
// really stored (for a file option that is the copied path).
func setOption(service settings.Service, extID, key, raw string, stdout io.Writer) error {
	if err := service.SetOption(extID, key, raw); err != nil {
		return err
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "option: %s %s = %v\n", extID, key, snapshot.Config.Options[extID][key])
	return nil
}

func runPrivacy(service settings.Service, args []string, stdout io.Writer) error {
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

func runSound(service settings.Service, args []string, stdout io.Writer) error {
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

func runFPS(service settings.Service, args []string, stdout io.Writer) error {
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

func runExt(service settings.Service, args []string, stdout io.Writer) error {
	switch {
	case len(args) == 1 && args[0] == "list":
		return listExtensions(service, stdout, "")
	case len(args) == 2 && args[0] == "install":
		manifest, err := service.InstallExtension(args[1])
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "installed: %s %s\n", manifest.ID, manifest.Version)
		return nil
	case len(args) == 2 && args[0] == "remove":
		if err := service.RemoveExtension(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "removed: %s\n", args[1])
		return nil
	}
	return usageError("ext list | ext install <path> | ext remove <id>")
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
