import assert from "node:assert/strict";
import { before, describe, it } from "node:test";
import type { ComponentExt } from "../src/api.ts";
import { formatClock, formatDate } from "../src/format.ts";
import { FakeElement, fakeCtx, loadComponent, settle } from "./fake-page.ts";

const NOW_MS = new Date(2026, 9, 26, 9, 5, 30).getTime();
const MINUTE_MS = 60_000;

describe("chrome clock", () => {
  let create: () => ComponentExt;

  before(async () => {
    create = await loadComponent("file:///ext/builtin.chrome-clock/index.js", () => import("../src/builtins/chrome-clock/index.ts"));
  });

  function mounted(options: Record<string, unknown> = {}) {
    const root = new FakeElement("div");
    const { ctx, tick, frames } = fakeCtx(NOW_MS, options);
    const clock = create();
    clock.mount(root as unknown as HTMLElement, ctx);
    const [box] = root.find("chrome-clock");
    assert.ok(box, "the clock draws a box");
    return { root, box, clock, tick, frames };
  }

  it("shows the same time and date as the built-in clock", () => {
    const { box } = mounted();
    assert.equal(box.find("time")[0].textContent, formatClock(new Date(NOW_MS)));
    assert.equal(box.find("date")[0].textContent, formatDate(new Date(NOW_MS)));
  });

  it("follows the minute on each tick", () => {
    const { box, tick } = mounted();
    tick(NOW_MS + MINUTE_MS);
    assert.equal(box.find("time")[0].textContent, formatClock(new Date(NOW_MS + MINUTE_MS)));
  });

  it("is large by default and small when asked", () => {
    assert.equal(mounted().box.classList.contains("small"), false);
    assert.equal(mounted({ size: "large" }).box.classList.contains("small"), false);
    assert.equal(mounted({ size: "small" }).box.classList.contains("small"), true);
  });

  it("changes size when its options change", () => {
    const { box, clock } = mounted({ size: "large" });
    clock.setOptions?.({ size: "small" });
    assert.equal(box.classList.contains("small"), true);
    clock.setOptions?.({ size: "large" });
    assert.equal(box.classList.contains("small"), false);
  });

  it("keeps the system font when its font does not load", async () => {
    const { root, box } = mounted();
    await settle();
    assert.equal(box.classList.contains("font-ready"), false);
    assert.equal(box.find("time")[0].textContent, formatClock(new Date(NOW_MS)));
    const css = root.children.find((child) => child.tag === "style")?.textContent ?? "";
    assert.match(css, /font-family: "Helvetica Neue"/, "a system face is set before the font loads");
  });

  it("stops its tick and empties its root on unmount", () => {
    const { root, clock, frames } = mounted();
    clock.unmount();
    assert.equal(frames.cancelled, frames.started);
    assert.equal(root.children.length, 0);
  });
});
