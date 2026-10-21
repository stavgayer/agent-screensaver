import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { Card } from "../src/api.ts";
import {
  STALE_AFTER_MS,
  backgroundSignature,
  cssVariables,
  diffRegions,
  isServiceLineVisible,
  mergeOptions,
  planRegions,
  resetKey,
  toViewState,
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

describe("planRegions", () => {
  it("merges_options_and_skips_unknown_ids", () => {
    const plan = planRegions(state(), "idle");
    assert.deepEqual(plan.map((r) => r.region), ["center", "bottom"]);
    const [center, bottom] = plan;
    assert.deepEqual(center?.components[0]?.options, { size: "large", color: "red" });
    assert.equal(center?.components[0]?.entryUrl, "file:///a.js");
    assert.deepEqual(bottom?.components.map((c) => c.id), ["b"]);
  });

  it("null_layout_falls_back_to_clock", () => {
    const plan = planRegions(state({}, { layout: null, components: [{ id: "builtin.clock", entry_url: "file:///c.js", options: {} }] }), "idle");
    assert.equal(plan[0]?.region, "center");
    assert.deepEqual(plan[0]?.components[0]?.options, { size: "large" });
  });

  it("follows_the_mode", () => {
    assert.deepEqual(planRegions(state(), "attention").map((r) => r.region), ["top-right"]);
  });
});

describe("diffRegions", () => {
  it("mounts_everything_first_time", () => {
    const plan = planRegions(state(), "idle");
    const diff = diffRegions({}, plan);
    assert.deepEqual(diff.clear, []);
    assert.equal(diff.mount.length, 2);
  });

  it("keeps_unchanged_regions_and_clears_dropped_ones", () => {
    const idle = planRegions(state(), "idle");
    const mounted = Object.fromEntries(idle.map((r) => [r.region, r.signature]));
    const diff = diffRegions(mounted, planRegions(state(), "attention"));
    assert.deepEqual(diff.clear.sort(), ["bottom", "center"]);
    assert.deepEqual(diff.mount.map((r) => r.region), ["top-right"]);
    assert.deepEqual(diffRegions(mounted, idle), { clear: [], mount: [] });
  });

  it("remounts_a_region_whose_options_changed", () => {
    const before = planRegions(state(), "idle");
    const mounted = Object.fromEntries(before.map((r) => [r.region, r.signature]));
    const next = state({}, { components: [{ id: "a", entry_url: "file:///a.js", options: { size: "tiny" } }, { id: "b", entry_url: "file:///b.js", options: {} }] });
    const diff = diffRegions(mounted, planRegions(next, "idle"));
    assert.deepEqual(diff.clear, ["center"]);
    assert.deepEqual(diff.mount.map((r) => r.region), ["center"]);
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
