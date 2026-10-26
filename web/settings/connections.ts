// The Agents tab's text and buttons, as pure functions of Go's
// settings.Connection list. The state text is the same as `agent-screensaver
// agent list` writes (internal/cli/agent.go StateText): change both together.

import { formatDuration } from "../src/format.ts";

export type ConnectionState = "not_installed" | "no_hooks" | "active" | "idle" | "off";

/** One agent extension, as Go's `connections` call answers it. */
export interface Connection {
  id: string;
  name: string;
  source: string;
  /** The mark's file URL inside the data folder, as the cards use it. */
  mark: string;
  version: string;
  builtin: boolean;
  state: ConnectionState;
  /** null: no event yet. */
  last_event_ms: number | null;
  found: boolean;
  hooks_added: boolean;
  hooks_changed: boolean;
  /** The hooks are in place, but the status line runs the user's own command. */
  statusline_own: boolean;
  /** Converter and plugin file lines of the add plan. */
  warnings: string[];
}

export type HookAction = "add" | "remove";

export interface ConnectionRow {
  id: string;
  name: string;
  mark: string;
  state: ConnectionState;
  /** "built in" or "from <id> <version>". */
  badge: string;
  /** "Active · 4m ago", "Idle · 3h", "Idle · no events yet", "Hooks not added", "Not installed", "Off". */
  stateText: string;
  /** "last event 4m ago" or "no events yet". */
  lastEvent: string;
  /** Shown when our entries were removed from the tool's file by someone else. */
  changed: string | null;
  hook: { action: HookAction; label: string } | null;
  power: { on: boolean; label: string } | null;
}

export const CHANGED_TEXT = "Hooks were changed outside agent-screensaver";
export const STATUSLINE_TEXT = "Status line points to your own command";

export function connectionRows(list: Connection[], nowMs: number): ConnectionRow[] {
  return list.map((connection) => ({
    id: connection.id,
    name: connection.name,
    mark: connection.mark,
    state: connection.state,
    badge: connection.builtin ? "built in" : `from ${connection.id} ${connection.version}`,
    stateText: stateText(connection, nowMs),
    lastEvent: connection.last_event_ms === null ? "no events yet" : `last event ${ago(connection.last_event_ms, nowMs)}`,
    changed: connection.hooks_changed ? CHANGED_TEXT : connection.statusline_own ? STATUSLINE_TEXT : null,
    hook: hookButton(connection),
    power: powerButton(connection),
  }));
}

function ago(atMs: number, nowMs: number): string {
  return `${formatDuration(nowMs - atMs)} ago`;
}

function stateText(connection: Connection, nowMs: number): string {
  const last = connection.last_event_ms;
  switch (connection.state) {
    case "off":
      return "Off";
    case "not_installed":
      return "Not installed";
    case "no_hooks":
      return "Hooks not added";
    case "active":
      return `Active · ${ago(last ?? nowMs, nowMs)}`;
    case "idle":
      return last === null ? "Idle · no events yet" : `Idle · ${formatDuration(nowMs - last)}`;
  }
}

/**
 * Remove when our hooks are in place; Add when the tool is here and they
 * are missing or were changed, or the status line was pointed elsewhere
 * (adding them again puts ours back and saves the user's command). An Off row keeps its hook button: off only
 * silences the agent. A tool that is not on this Mac gets none, unless an
 * older install left hooks that can still be removed.
 */
function hookButton(connection: Connection): ConnectionRow["hook"] {
  const intact = !connection.hooks_changed && !connection.statusline_own;
  if (connection.hooks_added && intact) return { action: "remove", label: "Remove hooks" };
  if (connection.found) return { action: "add", label: "Add hooks" };
  if (connection.hooks_added) return { action: "remove", label: "Remove hooks" };
  return null;
}

/** A tool that is not here has nothing to turn off; an Off row can always be turned on. */
function powerButton(connection: Connection): ConnectionRow["power"] {
  if (connection.state === "off") return { on: true, label: "Turn on" };
  if (connection.state === "not_installed") return null;
  return { on: false, label: "Turn off" };
}

/** A hook plan, as Go's `planAgentHooks` call answers it. */
export interface Plan {
  steps: { text: string; warning: boolean }[];
  hash: string;
  warnings: string[];
}

export interface PlanDialog {
  title: string;
  /** Every plan line; the trust warning lines are marked to stand out. */
  lines: { text: string; warning: boolean }[];
  /** Shown above the lines when the agent brings code of its own. */
  trustNote: string | null;
  confirmLabel: string;
}

/**
 * The dialog for one plan. For an outside agent that runs its own program or
 * copies a plugin file, the confirm button names the trust it asks for.
 */
export function planDialog(name: string, action: HookAction, plan: Plan): PlanDialog {
  const trusting = action === "add" && plan.warnings.length > 0;
  return {
    title: action === "add" ? `Add hooks for ${name}?` : `Remove hooks for ${name}?`,
    // Only the lines that ask for trust stand out; the first line of an
    // outside agent is a warning in Go's plan too, but the note above says it.
    lines: plan.steps.map((step) => ({ text: step.text, warning: step.warning && plan.warnings.includes(step.text) })),
    trustNote: trusting
      ? `${name} is not built in. It runs code of its own on your Mac, outside the screen saver. Add its hooks only if you trust where it came from.`
      : null,
    confirmLabel: trusting ? `I trust ${name}, add hooks` : action === "add" ? "Add hooks" : "Remove hooks",
  };
}

/** Go's message when the plan changed between showing and applying (settings.ErrPlanChanged). */
export function isPlanChanged(message: string): boolean {
  return message.includes("the plan changed");
}

/** The element ids of one row's controls, as the page names them. */
export const hookButtonId = (id: string): string => `hook-${id}`;
export const powerButtonId = (id: string): string => `power-${id}`;

/**
 * Where focus goes when the plan sheet closes: back to the button that
 * opened it, else (the row is gone or lost that button) the row's other
 * button, else the install button, so focus never falls to the page body.
 */
export function focusAfterPlan(id: string, exists: (elementId: string) => boolean): string | undefined {
  return [hookButtonId(id), powerButtonId(id), "install-agent"].find(exists);
}
