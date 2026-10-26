import assert from "node:assert/strict";
import { before, describe, it } from "node:test";
import type { ComponentExt, Limit, ViewState } from "../src/api.ts";
import { orderLimits, ringView } from "../src/builtins/limit-rings/logic.ts";
import { FakeElement, fakeCtx, loadComponent, settle } from "./fake-page.ts";

const NOW_MS = 10_000_000_000;
const HOUR_S = 3600;

const limit = (source: string, window: Limit["window"], overrides: Partial<Limit> = {}): Limit => ({
  source,
  window,
  used_percent: 42,
  updated_at_ms: NOW_MS,
  ...overrides,
});

const view = (limits: Limit[]): ViewState => ({
  mode: "idle",
  cards: [],
  agents: {},
  agentOrder: ["claude", "codex"],
  limits,
  nowMs: NOW_MS,
  serviceOk: true,
  display: "main",
});

describe("chrome rings", () => {
  let create: () => ComponentExt;

  before(async () => {
    create = await loadComponent("file:///ext/builtin.chrome-rings/index.js", () => import("../src/builtins/chrome-rings/index.ts"));
  });

  function mounted() {
    const root = new FakeElement("div");
    const rings = create();
    rings.mount(root as unknown as HTMLElement, fakeCtx(NOW_MS).ctx);
    return { root, rings };
  }

  const limits = [
    limit("codex", "week", { used_percent: 9, resets_at: NOW_MS / 1000 + 50 * HOUR_S }),
    limit("aider", "5h", { used_percent: 70 }),
    limit("claude", "5h", { used_percent: 61.4, resets_at: NOW_MS / 1000 + 2 * HOUR_S + 600, estimated: true }),
    limit("codex", "5h", { used_percent: 17, resets_at: NOW_MS / 1000 + 3 * HOUR_S }),
  ];

  it("shows one ring per limit in the agents' order", () => {
    const { root, rings } = mounted();
    rings.update(view(limits));
    const names = root.find("ring-name").map((element) => element.textContent);
    assert.deepEqual(names, orderLimits(limits, ["claude", "codex"]).map((one) => ringView(one, NOW_MS).name));
    assert.deepEqual(names, ["Claude 5h", "Codex 5h", "Codex week", "Aider 5h"]);
  });

  it("says the percent, the reset and the estimate mark exactly as the limit rings do", () => {
    const { root, rings } = mounted();
    rings.update(view(limits));
    const ordered = orderLimits(limits, ["claude", "codex"]);
    assert.deepEqual(root.find("ring-value").map((element) => element.textContent), ordered.map((one) => ringView(one, NOW_MS).center));
    assert.deepEqual(
      root.find("ring-detail").map((element) => element.textContent),
      ordered.map((one) => ringView(one, NOW_MS).detail || " "),
    );
    assert.equal(root.find("ring-detail")[0].textContent, "2h 10m · est.");
  });

  it("draws the arc from the percent and dims an old reading", () => {
    const { root, rings } = mounted();
    rings.update(view([limit("claude", "5h", { used_percent: 61.4, updated_at_ms: NOW_MS - 2 * HOUR_S * 1000 })]));
    const [ring] = root.find("ring");
    assert.equal(ring.style.get("--pct"), "61");
    assert.ok(ring.classList.contains("stale"));
    assert.ok(ring.classList.contains("warn"));
  });

  it("no limits, no rings", () => {
    const { root, rings } = mounted();
    rings.update(view([]));
    assert.equal(root.find("ring").length, 0);
  });

  it("still shows its rings when its font does not load", async () => {
    const { root, rings } = mounted();
    await settle();
    rings.update(view(limits));
    assert.equal(root.find("ring").length, limits.length);
    const css = root.children.find((child) => child.tag === "style")?.textContent ?? "";
    assert.match(css, /ui-monospace/, "a system monospace face follows the bundled one");
  });

  it("empties its root on unmount", () => {
    const { root, rings } = mounted();
    rings.update(view(limits));
    rings.unmount();
    assert.equal(root.children.length, 0);
  });
});
