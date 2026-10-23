import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { CHANGED_TEXT, STATUSLINE_TEXT, connectionRows, focusAfterPlan, isPlanChanged, planDialog, type Connection, type Plan } from "../settings/connections.ts";

const MINUTE = 60_000;
const NOW = 1_000 * MINUTE;

function connection(overrides: Partial<Connection>): Connection {
  return {
    id: "builtin.claude",
    name: "Claude Code",
    source: "claude",
    mark: "file:///data/extensions/builtin.claude/mark.svg",
    version: "1.0.0",
    builtin: true,
    state: "idle",
    last_event_ms: null,
    found: true,
    hooks_added: true,
    hooks_changed: false,
    statusline_own: false,
    warnings: [],
    ...overrides,
  };
}

const rowOf = (overrides: Partial<Connection>) => connectionRows([connection(overrides)], NOW)[0]!;

describe("connectionRows", () => {
  it("active row says Active and minutes", () => {
    const row = rowOf({ state: "active", last_event_ms: NOW - 4 * MINUTE });
    assert.equal(row.stateText, "Active · 4m ago");
    assert.equal(row.lastEvent, "last event 4m ago");
    assert.deepEqual(row.hook, { action: "remove", label: "Remove hooks" });
    assert.deepEqual(row.power, { on: false, label: "Turn off" });
  });

  it("active a moment ago reuses the card's under-a-minute text", () => {
    assert.equal(rowOf({ state: "active", last_event_ms: NOW - 1000 }).stateText, "Active · <1m ago");
  });

  it("idle row says Idle and hours", () => {
    assert.equal(rowOf({ state: "idle", last_event_ms: NOW - 180 * MINUTE }).stateText, "Idle · 3h");
  });

  it("idle with no events says no events yet", () => {
    const row = rowOf({ state: "idle", last_event_ms: null });
    assert.equal(row.stateText, "Idle · no events yet");
    assert.equal(row.lastEvent, "no events yet");
  });

  it("not installed row has no hook buttons", () => {
    const row = rowOf({ state: "not_installed", found: false, hooks_added: false });
    assert.equal(row.stateText, "Not installed");
    assert.equal(row.hook, null);
    assert.equal(row.power, null);
  });

  it("no hooks row offers Add hooks", () => {
    const row = rowOf({ state: "no_hooks", hooks_added: false });
    assert.equal(row.stateText, "Hooks not added");
    assert.deepEqual(row.hook, { action: "add", label: "Add hooks" });
  });

  it("changed hooks row says so and offers Add hooks", () => {
    const row = rowOf({ state: "idle", hooks_changed: true });
    assert.equal(row.changed, CHANGED_TEXT);
    assert.deepEqual(row.hook, { action: "add", label: "Add hooks" });
  });

  it("a user-owned status line says so and offers Add hooks", () => {
    const row = rowOf({ state: "idle", statusline_own: true });
    assert.equal(row.changed, STATUSLINE_TEXT);
    assert.deepEqual(row.hook, { action: "add", label: "Add hooks" });
  });

  it("changed hooks win over the status line text", () => {
    assert.equal(rowOf({ hooks_changed: true, statusline_own: true }).changed, CHANGED_TEXT);
  });

  it("off row offers Turn on and keeps hook buttons", () => {
    const row = rowOf({ state: "off", last_event_ms: NOW - MINUTE });
    assert.equal(row.stateText, "Off");
    assert.deepEqual(row.power, { on: true, label: "Turn on" });
    assert.deepEqual(row.hook, { action: "remove", label: "Remove hooks" });
  });

  it("outside agent shows id and version", () => {
    assert.equal(rowOf({ id: "acme.agent", version: "2.1.0", builtin: false }).badge, "from acme.agent 2.1.0");
    assert.equal(rowOf({}).badge, "built in");
  });
});

describe("planDialog", () => {
  const converter: Plan = {
    steps: [
      { text: "Add hooks for Acme (from acme.agent 1.0.0)", warning: true },
      { text: "Add to /Users/me/.acme/hooks.json for stop: … hook --tool acme", warning: false },
      { text: "acme.agent runs its own program bin/convert on each Acme event", warning: true },
    ],
    hash: "h",
    warnings: ["acme.agent runs its own program bin/convert on each Acme event"],
  };

  it("plan dialog shows warnings and the trust button for an outside converter", () => {
    const dialog = planDialog("Acme", "add", converter);
    assert.equal(dialog.lines.length, 3);
    assert.deepEqual(dialog.lines.map((line) => line.warning), [false, false, true]);
    assert.equal(dialog.confirmLabel, "I trust Acme, add hooks");
    assert.match(dialog.trustNote ?? "", /not built in/);
  });

  it("a built-in agent gets the plain buttons", () => {
    const plan: Plan = { steps: [{ text: "Add hooks for Cursor (built in)", warning: false }], hash: "h", warnings: [] };
    assert.equal(planDialog("Cursor", "add", plan).confirmLabel, "Add hooks");
    assert.equal(planDialog("Cursor", "add", plan).trustNote, null);
    assert.equal(planDialog("Cursor", "remove", plan).confirmLabel, "Remove hooks");
    assert.equal(planDialog("Cursor", "remove", plan).title, "Remove hooks for Cursor?");
  });

  it("knows Go's stale plan error", () => {
    assert.equal(isPlanChanged("the plan changed since it was shown; look at the new plan"), true);
    assert.equal(isPlanChanged("Cursor is not on this Mac"), false);
  });
});

describe("focusAfterPlan", () => {
  it("returns to the button that opened the sheet", () => {
    const ids = new Set(["hook-builtin.cursor", "power-builtin.cursor", "install-agent"]);
    assert.equal(focusAfterPlan("builtin.cursor", (id) => ids.has(id)), "hook-builtin.cursor");
  });

  it("falls back to the row's other button, then the install button", () => {
    assert.equal(focusAfterPlan("builtin.cursor", (id) => id === "power-builtin.cursor" || id === "install-agent"), "power-builtin.cursor");
    assert.equal(focusAfterPlan("acme.gone", (id) => id === "install-agent"), "install-agent");
    assert.equal(focusAfterPlan("acme.gone", () => false), undefined);
  });
});
