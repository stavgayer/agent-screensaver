import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { Mode } from "../src/api.ts";
import { regionsFor } from "../src/layout.ts";

const CLOCK_FALLBACK = { center: [{ id: "builtin.clock", options: { size: "large" } }] };
const MODES: Mode[] = ["idle", "attention", "secondary"];

describe("regionsFor", () => {
  it("regions_for_idle", () => {
    const layout = {
      idle: {
        "top-left": [{ id: "builtin.cards" }],
        center: [{ id: "builtin.clock", options: { size: "small" } }],
      },
    };
    assert.deepEqual(regionsFor(layout, "idle"), {
      "top-left": [{ id: "builtin.cards" }],
      center: [{ id: "builtin.clock", options: { size: "small" } }],
    });
  });

  it("drops_unknown_region", () => {
    const layout = { idle: { "top-left": [{ id: "a" }], middle: [{ id: "b" }] } };
    assert.deepEqual(regionsFor(layout, "idle"), { "top-left": [{ id: "a" }] });
  });

  it("drops_ext_refs_without_string_id", () => {
    const layout = {
      idle: { bottom: [{ id: "ok" }, { id: 5 }, {}, null, "text", { options: {} }] },
    };
    assert.deepEqual(regionsFor(layout, "idle"), { bottom: [{ id: "ok" }] });
  });

  it("drops_region_whose_value_is_not_a_list", () => {
    const layout = { idle: { bottom: { id: "a" }, center: [{ id: "b" }] } };
    assert.deepEqual(regionsFor(layout, "idle"), { center: [{ id: "b" }] });
  });

  it("passes_options_through_as_given", () => {
    const options = { anything: [1, { nested: true }], size: 42 };
    const layout = { idle: { center: [{ id: "a", options }] } };
    assert.deepEqual(regionsFor(layout, "idle"), { center: [{ id: "a", options }] });
  });

  it("missing_secondary_mode_falls_back_to_clock", () => {
    assert.deepEqual(regionsFor({ idle: { center: [{ id: "a" }] } }, "secondary"), CLOCK_FALLBACK);
  });

  it("mode_value_not_an_object_falls_back", () => {
    const layout = { idle: "nope", attention: null, secondary: [1] };
    for (const mode of MODES) assert.deepEqual(regionsFor(layout, mode), CLOCK_FALLBACK);
  });

  it("broken_layout_falls_back_to_clock", () => {
    for (const layout of [undefined, null, 42, "text", [], true]) {
      for (const mode of MODES) {
        assert.deepEqual(regionsFor(layout, mode), CLOCK_FALLBACK);
      }
    }
  });

  it("fallback_is_a_fresh_object_each_call", () => {
    const first = regionsFor(null, "idle");
    first.center?.push({ id: "extra" });
    assert.deepEqual(regionsFor(null, "idle"), CLOCK_FALLBACK);
  });
});
