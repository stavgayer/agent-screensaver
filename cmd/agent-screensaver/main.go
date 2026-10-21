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
	"syscall"
	"time"

	"agent-screensaver/internal/cli"
	"agent-screensaver/internal/daemon"
	"agent-screensaver/internal/limits"
	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/platform"
)

// subcommands lists every subcommand, in the order shown in the usage text.
var subcommands = []string{
	"hook", "statusline", "daemon", "install", "uninstall", "theme",
	"option", "privacy", "sound", "fps", "ext", "tray", "settings",
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches to a subcommand and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
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
	providers := limits.All(exec.LookPath)
	if err := daemon.Run(ctx, service, platform.NewSaverWatcher(), platform.NewSoundPlayer(), providers); err != nil {
		fmt.Fprintf(stderr, "agent-screensaver daemon: %v\n", err)
		return 1
	}
	return 0
}
