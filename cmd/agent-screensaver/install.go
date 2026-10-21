package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agent-screensaver/internal/install"
	"agent-screensaver/internal/platform"
)

// setup holds what install and uninstall depend on, so tests can use a temp
// home and a fake service manager.
type setup struct {
	home       string
	executable string
	services   platform.ServiceManager
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

// runSetup resolves the real environment and runs install or uninstall.
func runSetup(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver %s: %v\n", name, err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver %s: %v\n", name, err)
		return 1
	}
	env := setup{home: home, executable: executable, services: platform.NewLaunchd(home), stdin: stdin, stdout: stdout, stderr: stderr}
	if name == "install" {
		return env.install(args)
	}
	return env.uninstall(args)
}

func (s setup) install(args []string) int {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(s.stderr)
	yes := flags.Bool("yes", false, "skip the confirmation prompt")
	artifacts := flags.String("artifacts", filepath.Dir(s.executable), "folder with runtime, extensions and AgentScreensaver.saver")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if !s.confirm(install.Plan(s.home), *yes) {
		return 1
	}
	if _, err := install.Apply(s.home, s.executable, *artifacts, s.services); err != nil {
		fmt.Fprintf(s.stderr, "agent-screensaver install: %v\n", err)
		return 1
	}
	s.printInstalled()
	return 0
}

func (s setup) uninstall(args []string) int {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(s.stderr)
	yes := flags.Bool("yes", false, "skip the confirmation prompt")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	plan, err := install.PlanUninstall(s.home)
	if err != nil {
		fmt.Fprintf(s.stderr, "agent-screensaver uninstall: %v\n", err)
		return 1
	}
	if !s.confirm(plan, *yes) {
		return 1
	}
	if err := install.Uninstall(s.home, s.services); err != nil {
		fmt.Fprintf(s.stderr, "agent-screensaver uninstall: %v\n", err)
		return 1
	}
	fmt.Fprintln(s.stdout, "Uninstalled. Your settings and extensions in the data folder were kept.")
	return 0
}

// confirm prints the plan and asks to proceed, unless yes is set.
func (s setup) confirm(plan []install.PlannedStep, yes bool) bool {
	fmt.Fprintln(s.stdout, "This will:")
	for _, step := range plan {
		fmt.Fprintf(s.stdout, "  - %s\n", step.Description)
	}
	if yes {
		return true
	}
	fmt.Fprint(s.stdout, "Proceed? [y/N] ")
	answer, _ := bufio.NewReader(s.stdin).ReadString('\n')
	if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
		return true
	}
	fmt.Fprintln(s.stdout, "Cancelled. Nothing was changed.")
	return false
}

func (s setup) printInstalled() {
	fmt.Fprintln(s.stdout, "\nInstalled. Two things are left for you:")
	fmt.Fprintln(s.stdout, "  1. In Codex, run /hooks and approve the new agent-screensaver hooks.")
	fmt.Fprintln(s.stdout, `  2. In System Settings -> Lock Screen, set "Require password after screen saver begins" to Immediately.`)
	fmt.Fprintln(s.stdout, "\nTo select the screen saver: System Settings -> Screen Saver -> AgentScreensaver.")
	fmt.Fprintln(s.stdout, `The first time, open System Settings -> Privacy & Security and click "Open Anyway".`)
}
