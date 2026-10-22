// Command agent-screensaver is the service and CLI behind the screen saver.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"agent-screensaver/internal/cli"
	"agent-screensaver/internal/daemon"
	"agent-screensaver/internal/limits"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
	"agent-screensaver/internal/settings"
	"agent-screensaver/internal/settingsui"
	"agent-screensaver/internal/tray"
)

// subcommands lists every subcommand, in the order shown in the usage text.
var subcommands = []string{
	"hook", "statusline", "daemon", "install", "uninstall", "theme",
	"background", "option", "privacy", "sound", "fps", "ext", "tray", "settings",
}

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches to a subcommand and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if len(args) == 0 || !isSubcommand(args[0]) {
		printUsage(stderr)
		return 2
	}
	name := args[0]
	switch name {
	case "hook", "statusline":
		return runTool(name, args[1:], os.Stdin, stdout)
	case "daemon":
		return runDaemon(args[1:], stderr)
	case "install", "uninstall":
		return runSetup(name, args[1:], os.Stdin, stdout, stderr)
	case "tray":
		return runTray(args[1:], stderr)
	case "settings":
		return runSettingsWindow(args[1:], stderr)
	}
	if cli.IsSettingsCommand(name) {
		return runSettings(name, args[1:], stdout, stderr)
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	// Every subcommand is a stub until its task is built.
	fmt.Fprintf(stderr, "agent-screensaver %s: not implemented\n", name)
	return 1
}

func isSubcommand(name string) bool {
	for _, known := range subcommands {
		if name == known {
			return true
		}
	}
	return false
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "usage: agent-screensaver <command> [flags]")
	fmt.Fprintln(out, "commands:")
	for _, name := range subcommands {
		fmt.Fprintf(out, "  %s\n", name)
	}
}

// runTool runs a command that coding tools call. Those must exit 0 even when
// the home cannot be resolved, so they never report a failure.
func runTool(name string, args []string, stdin io.Reader, stdout io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = io.Copy(io.Discard, stdin)
		return 0
	}
	p := paths.ForHome(home)
	now := func() int64 { return time.Now().UnixMilli() }
	if name == "hook" {
		return cli.Hook(args, stdin, p, now)
	}
	return cli.RunStatusline(stdin, stdout, p, now)
}

// runSettings runs a settings command against the real home.
func runSettings(name string, args []string, stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver %s: %v\n", name, err)
		return 1
	}
	return cli.Settings(name, args, stdout, stderr, paths.ForHome(home))
}

// runDaemon runs the service until SIGINT or SIGTERM. It logs to stderr,
// which launchd captures.
func runDaemon(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("daemon", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver daemon: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service := daemon.New(paths.ForHome(home))
	service.Cloner = platform.NewFileCloner()
	providers := limits.All(limits.WithFallbackDirs(exec.LookPath, limits.ToolDirs(home, filepath.Glob)), os.Environ())
	// The daemon runs beside the main goroutine, which must serve the main
	// run loop for the screen saver notifications.
	finished := make(chan struct{})
	var runErr error
	go func() {
		defer close(finished)
		runErr = daemon.Run(ctx, service, platform.NewSaverWatcher(), platform.NewSoundPlayer(), providers)
	}()
	platform.RunMainLoop(finished)
	if runErr != nil {
		fmt.Fprintf(stderr, "agent-screensaver daemon: %v\n", runErr)
		return 1
	}
	return 0
}

// runTray shows the menu bar icon until the user quits it or the process gets
// SIGINT or SIGTERM. The main goroutine is already locked to the main thread
// (the platform package does that in init), which the tray needs.
func runTray(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("tray", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver tray: %v\n", err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver tray: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	p := paths.ForHome(home)
	if err := tray.Run(ctx, p, settings.Service{Paths: p}, tray.NewSettingsLauncher(executable)); err != nil {
		fmt.Fprintf(stderr, "agent-screensaver tray: %v\n", err)
		return 1
	}
	return 0
}

// runSettingsWindow shows the settings window until the user closes it. Like
// the tray, it runs on the main goroutine, which the window needs.
func runSettingsWindow(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("settings", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver settings: %v\n", err)
		return 1
	}
	p := paths.ForHome(home)
	err = settingsui.Open(p, settings.Service{Paths: p}, platform.NewFilePicker(), platform.NewSettingsWindow())
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver settings: %v\n", err)
		return 1
	}
	return 0
}
