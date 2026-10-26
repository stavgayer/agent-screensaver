import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { BackgroundExt, ViewState } from "../src/api.ts";
import { bindBackground, createKeeper, type Instance } from "../src/instance.ts";

const view = (mode: ViewState["mode"]): ViewState => ({
  mode,
  cards: [],
  agents: {},
  agentOrder: [],
  limits: [],
  nowMs: 0,
  serviceOk: true,
  display: "main",
});

function newItem(key: string): Instance {
  return { key, root: { remove() {} }, cancels: new Set(), stop() {}, dead: false };
}

function keeper() {
  const failed = new Set<string>();
  return { failed, ...createKeeper(failed, () => {}) };
}

// Mirrors launch(): bind the extension, start it, then the runtime fans state out.
function startBackground(ext: BackgroundExt) {
  const { guard, updateAll, failed } = keeper();
  const item = newItem("background");
  bindBackground(item, ext);
  guard(item, "start", () => ext.start({} as HTMLElement, {} as never));
  return { item, updateAll, failed };
}

describe("bindBackground", () => {
  it("background with update gets the state after start", () => {
    const calls: string[] = [];
    const { item, updateAll } = startBackground({
      start: () => void calls.push("start"),
      update: (state) => void calls.push(`update:${state.mode}`),
      stop() {},
    });
    updateAll([item], view("attention"));
    assert.deepEqual(calls, ["start", "update:attention"]);
  });

  it("background with update gets every later state", () => {
    const seen: string[] = [];
    const { item, updateAll } = startBackground({ start() {}, update: (state) => void seen.push(state.mode), stop() {} });
    updateAll([item], view("idle"));
    updateAll([item], view("attention"));
    updateAll([item], view("secondary"));
    assert.deepEqual(seen, ["idle", "attention", "secondary"]);
  });

  it("background without update is started and stopped as before and never called with state", () => {
    const calls: string[] = [];
    const { item, updateAll } = startBackground({ start: () => void calls.push("start"), stop: () => void calls.push("stop") });
    updateAll([item], view("attention"));
    item.stop();
    assert.equal(item.update, undefined);
    assert.deepEqual(calls, ["start", "stop"]);
  });

  it("background update that throws does not stop components from updating", () => {
    const { item, updateAll, failed } = startBackground({
      start() {},
      update: () => {
        throw new Error("x");
      },
      stop() {},
    });
    let updated = 0;
    const component: Instance = { ...newItem("clock"), update: () => void updated++ };
    updateAll([item, component], view("idle"));
    updateAll([item, component], view("idle"));
    assert.equal(updated, 2);
    assert.equal(item.dead, true);
    assert.deepEqual([...failed], ["background"]);
  });
});
