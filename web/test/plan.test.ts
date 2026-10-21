import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { Card, Region } from "../src/api.ts";
import {
  STALE_AFTER_MS,
  backgroundSignature,
  cssVariables,
  isServiceLineVisible,
  mergeOptions,
  planComponents,
  reconcileComponents,
  resetKey,
  toViewState,
  type Options,
  type PlannedComponent,
  type RunningComponent,
  type StateFile,
} from "../src/plan.ts";

function state(overrides: Partial<StateFile> = {}, view: Partial<StateFile["view"]> = {}): StateFile {
  return {
    version: 1,
    generated_at_ms: 1_000_000,
    service_ok: true,
    cards: [],
    limits: [],
    view: {
      theme: "builtin.default",
      background: { id: "builtin.plain", entry_url: "file:///bg.js", options: {} },
      layout: {
        modes: {
          idle: { center: [{ id: "a", options: { size: "large" } }], bottom: [{ id: "b" }, { id: "gone" }] },
          attention: { "top-right": [{ id: "a" }] },
        },
      },
      components: [
        { id: "a", entry_url: "file:///a.js", options: { size: "small", color: "red" } },
        { id: "b", entry_url: "file:///b.js", options: {} },
      ],
      style: {},
      privacy: {},
      max_fps: 30,
      ...view,
    },
    ...overrides,
  };
}

const approval: Card = { source: "claude", session: "s", kind: "approval", started_at_ms: 0, updated_at_ms: 0 };

describe("mergeOptions", () => {
  it("placement_wins_over_component_defaults", () => {
    assert.deepEqual(mergeOptions({ size: "small", color: "red" }, { size: "large" }), { size: "large", color: "red" });
  });
  it("missing_placement_options_is_empty", () => {
    assert.deepEqual(mergeOptions({ a: 1 }, undefined), { a: 1 });
  });
});

describe("planComponents", () => {
  it("merges_options_and_skips_unknown_ids", () => {
    const plan = planComponents(state(), "idle");
    assert.deepEqual(plan.map((c) => [c.region, c.index, c.id]), [["center", 0, "a"], ["bottom", 0, "b"]]);
    assert.deepEqual(plan[0]?.options, { size: "large", color: "red" });
    assert.equal(plan[0]?.entryUrl, "file:///a.js");
  });

  it("null_layout_falls_back_to_clock", () => {
    const plan = planComponents(state({}, { layout: null, components: [{ id: "builtin.clock", entry_url: "file:///c.js", options: {} }] }), "idle");
    assert.equal(plan[0]?.region, "center");
    assert.deepEqual(plan[0]?.options, { size: "large" });
  });

  it("follows_the_mode", () => {
    assert.deepEqual(planComponents(state(), "attention").map((c) => c.region), ["top-right"]);
  });
});

interface Fake extends RunningComponent { name: string }
const running = (name: string, id: string, region: Region, options: Options = {}, supportsSetOptions = false): Fake =>
  ({ name, id, entryUrl: `file:///${id}.js`, options, supportsSetOptions, region });
const placed = (id: string, region: Region, options: Options = {}, index = 0): PlannedComponent =>
  ({ region, index, id, entryUrl: `file:///${id}.js`, options });

describe("reconcileComponents", () => {
  it("same id moves region keeps instance", () => {
    const clock = running("clock", "clock", "center");
    const result = reconcileComponents([clock], [placed("clock", "top-left")]);
    assert.deepEqual(result.unmount, []);
    assert.deepEqual(result.mount, []);
    assert.equal(result.keep[0]?.running, clock);
    assert.equal(result.keep[0]?.moved, true);
    assert.equal(result.keep[0]?.setOptions, false);
  });

  it("same place and options keeps without move", () => {
    const result = reconcileComponents([running("c", "c", "center", { a: 1 })], [placed("c", "center", { a: 1 })]);
    assert.deepEqual([result.keep[0]?.moved, result.keep[0]?.setOptions], [false, false]);
  });

  it("options differ with setOptions keeps instance", () => {
    const clock = running("clock", "clock", "center", { size: "large" }, true);
    const result = reconcileComponents([clock], [placed("clock", "top-left", { size: "small" })]);
    assert.equal(result.keep[0]?.running, clock);
    assert.equal(result.keep[0]?.setOptions, true);
    assert.deepEqual(result.mount, []);
  });

  it("options differ without setOptions remounts", () => {
    const clock = running("clock", "clock", "center", { size: "large" });
    const result = reconcileComponents([clock], [placed("clock", "top-left", { size: "small" })]);
    assert.deepEqual(result.keep, []);
    assert.deepEqual(result.unmount, [clock]);
    assert.equal(result.mount.length, 1);
  });

  it("component only in new mode mounts", () => {
    const result = reconcileComponents([], [placed("cards", "bottom")]);
    assert.equal(result.mount[0]?.id, "cards");
  });

  it("component only in old mode unmounts", () => {
    const cards = running("cards", "cards", "bottom");
    const result = reconcileComponents([cards], []);
    assert.deepEqual(result.unmount, [cards]);
  });

  it("first placement of an id pairs with the first running one", () => {
    const first = running("one", "x", "center");
    const second = running("two", "x", "bottom");
    const result = reconcileComponents([first, second], [placed("x", "top-left"), placed("x", "bottom")]);
    assert.deepEqual(result.keep.map((k) => k.running), [first, second]);
  });
});

describe("resetKey and backgroundSignature", () => {
  it("changes_with_theme_or_max_fps", () => {
    const base = resetKey(state().view);
    assert.notEqual(base, resetKey(state({}, { theme: "other" }).view));
    assert.notEqual(base, resetKey(state({}, { max_fps: 60 }).view));
    assert.equal(base, resetKey(state().view));
  });
  it("background_signature_follows_options", () => {
    const changed = state({}, { background: { id: "builtin.plain", entry_url: "file:///bg.js", options: { speed: 2 } } });
    assert.notEqual(backgroundSignature(state().view), backgroundSignature(changed.view));
  });
});

describe("isServiceLineVisible", () => {
  const now = 1_000_000 + STALE_AFTER_MS;
  it("shows_when_service_is_down", () => {
    assert.equal(isServiceLineVisible(state({ service_ok: false }), 1_000_001), true);
  });
  it("hides_when_fresh", () => {
    assert.equal(isServiceLineVisible(state(), now), false);
  });
  it("shows_when_older_than_two_minutes", () => {
    assert.equal(isServiceLineVisible(state(), now + 1), true);
  });
});

describe("toViewState", () => {
  it("derives_mode_and_copies_fields", () => {
    const view = toViewState(state({ cards: [approval] }), "main", 5);
    assert.equal(view.mode, "attention");
    assert.equal(view.nowMs, 5);
    assert.equal(view.serviceOk, true);
    assert.equal(view.display, "main");
    assert.equal(toViewState(state({ cards: [approval] }), "secondary", 5).mode, "secondary");
  });
});

describe("cssVariables", () => {
  it("keeps_only_double_dash_keys", () => {
    assert.deepEqual(cssVariables({ "--accent": "#fff", font: "x", "--size": 3 }), [
      ["--accent", "#fff"],
      ["--size", "3"],
    ]);
  });
});
