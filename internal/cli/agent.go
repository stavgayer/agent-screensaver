package cli

import (
	"fmt"
	"io"
	"slices"

	"agent-screensaver/internal/paths"
	"agent-screensaver/internal/settings"
	"agent-screensaver/internal/tray"
)

const agentUsage = "agent list | agent on|off <id> | agent hooks add|remove [--yes] <id>"

// Agent runs `agent-screensaver agent ...` and returns the exit code. It
// shows and changes the same things as the settings window's Connections
// tab, through the same settings.Service methods.
func Agent(args []string, stdin io.Reader, stdout, stderr io.Writer, p paths.Paths, nowMs int64) int {
	service := settings.Service{Paths: p}
	var err error
	switch {
	case len(args) == 1 && args[0] == "list":
		err = listAgents(service, stdout, nowMs)
	case len(args) == 2 && (args[0] == "on" || args[0] == "off"):
		if err = service.SetAgentOn(args[1], args[0] == "on"); err == nil {
			fmt.Fprintf(stdout, "%s: %s\n", args[1], args[0])
		}
	case len(args) >= 3 && args[0] == "hooks" && (args[1] == "add" || args[1] == "remove"):
		return agentHooks(service, args[1] == "add", args[2:], stdin, stdout, stderr)
	default:
		err = usageError(agentUsage)
	}
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver agent: %v\n", err)
		return 1
	}
	return 0
}

// agentHooks shows the plan, asks, and applies the plan it showed: the
// hash makes the apply refuse when a file changed while the user read it.
func agentHooks(service settings.Service, add bool, rest []string, stdin io.Reader, stdout, stderr io.Writer) int {
	yes := slices.Contains(rest, "--yes")
	ids := slices.DeleteFunc(slices.Clone(rest), func(arg string) bool { return arg == "--yes" })
	if len(ids) != 1 {
		fmt.Fprintf(stderr, "agent-screensaver agent: %v\n", usageError(agentUsage))
		return 1
	}
	id := ids[0]
	plan, hash, err := service.PlanAgentHooks(id, add)
	if err != nil {
		fmt.Fprintf(stderr, "agent-screensaver agent: %v\n", err)
		return 1
	}
	if !Confirm(plan, yes, stdin, stdout) {
		return 1
	}
	if err := service.ApplyAgentHooks(id, add, hash); err != nil {
		fmt.Fprintf(stderr, "agent-screensaver agent: %v\n", err)
		return 1
	}
	verb := "removed"
	if add {
		verb = "added"
	}
	fmt.Fprintf(stdout, "hooks %s: %s\n", verb, id)
	return 0
}

// listAgents prints one tab-separated line per agent: id, name, origin,
// state and last event, the same text as the Connections tab.
func listAgents(service settings.Service, stdout io.Writer, nowMs int64) error {
	list, err := service.Connections(nowMs)
	if err != nil {
		return err
	}
	for _, connection := range list {
		origin := "built in"
		if !connection.Builtin {
			origin = "from " + connection.ID + " " + connection.Version
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\n", connection.ID, connection.Name, origin,
			StateText(connection, nowMs), lastEventText(connection, nowMs))
	}
	return nil
}

// StateText is the state as the Connections tab writes it (web/settings/
// connections.ts, which has the same cases): "Active · 4m ago", "Idle · 3h",
// "Idle · no events yet", "Hooks not added", "Not installed", "Off".
func StateText(c settings.Connection, nowMs int64) string {
	var text string
	switch c.State {
	case settings.StateOff:
		return "Off"
	case settings.StateNotInstalled:
		text = "Not installed"
	case settings.StateNoHooks:
		text = "Hooks not added"
	case settings.StateActive:
		text = "Active · " + tray.FormatDuration(nowMs-*c.LastEventMs) + " ago"
	default:
		text = "Idle · no events yet"
		if c.LastEventMs != nil {
			text = "Idle · " + tray.FormatDuration(nowMs-*c.LastEventMs)
		}
	}
	if c.HooksChanged {
		text += " · Hooks were changed outside agent-screensaver"
	}
	return text
}

func lastEventText(c settings.Connection, nowMs int64) string {
	if c.LastEventMs == nil {
		return "no events yet"
	}
	return "last event " + tray.FormatDuration(nowMs-*c.LastEventMs) + " ago"
}
