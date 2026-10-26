import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { before, describe, it } from "node:test";
import type { Card, ComponentExt, ViewState } from "../src/api.ts";
import { FRAGMENT_SHADER, VERTEX_SHADER } from "../src/builtins/chrome-scene/shader.ts";
import { liftShift } from "../src/builtins/chrome-scene/logic.ts";
import { FakeElement, fakeCtx, loadComponent } from "./fake-page.ts";

const NOW_MS = 10_000_000_000;
const MINUTE_MS = 60_000;

const card = (overrides: Partial<Card>): Card => ({
  source: "claude",
  session: "s1",
  kind: "running",
  started_at_ms: NOW_MS - 10 * MINUTE_MS,
  updated_at_ms: NOW_MS,
  ...overrides,
});

const view = (cards: Card[]): ViewState => ({
  mode: "idle",
  cards,
  agents: { claude: { name: "Claude Code", mark_url: "file:///marks/claude.svg" } },
  agentOrder: ["claude"],
  limits: [],
  nowMs: NOW_MS,
  serviceOk: true,
  display: "main",
});

function render(create: () => ComponentExt, cards: Card[]): FakeElement {
  const root = new FakeElement("div");
  const component = create();
  component.mount(root as unknown as HTMLElement, fakeCtx(NOW_MS).ctx);
  component.update(view(cards));
  return root;
}

const styleOf = (root: FakeElement): string => root.children.find((child) => child.tag === "style")?.textContent ?? "";
const source = (name: string): string => readFileSync(new URL(`../src/builtins/${name}/index.ts`, import.meta.url), "utf8");

describe("chrome tasks", () => {
  let create: () => ComponentExt;

  before(async () => {
    create = await loadComponent("file:///ext/builtin.chrome-tasks/index.js", () => import("../src/builtins/chrome-tasks/index.ts"));
  });

  it("sets the state and its time apart instead of joining them with a dot", () => {
    const root = render(create, [card({ name: "refactor auth" }), card({ session: "s2", kind: "unknown" })]);
    assert.deepEqual(root.find("status").map((element) => element.textContent), ["Working", "Check the terminal"]);
    assert.deepEqual(root.find("since").map((element) => element.textContent), ["10m"]);
    assert.ok(!root.text().includes("·"), root.text());
    assert.ok(!styleOf(root).includes('content: "·"'));
  });

  it("names the agent by its mark only: the chrome sign already carries the name", () => {
    const root = render(create, [card({})]);
    assert.equal(root.find("source-name").length, 0);
    assert.equal(root.find("source-mark")[0]?.alt, "Claude Code");
  });

  it("keeps every color and face in its token block, with no pure white", () => {
    const style = styleOf(render(create, [card({})]));
    const rules = style.split("\n").filter((one) => !one.startsWith(":root") && !one.startsWith("  --"));
    for (const rule of rules) assert.doesNotMatch(rule, /#[0-9a-f]{3,8}\b|rgba?\(/i, rule);
    assert.doesNotMatch(style, /#fff\b/i);
  });
});

describe("chrome approval", () => {
  let create: () => ComponentExt;

  before(async () => {
    create = await loadComponent("file:///ext/builtin.chrome-approval/index.js", () => import("../src/builtins/chrome-approval/index.ts"));
  });

  const approval = card({ kind: "approval", project: "web-app", request: "Bash: npm run migrate", approval_since_ms: NOW_MS - 3 * MINUTE_MS });

  it("puts the agent and the wait in one line of labels, the project and the request in a labelled sheet", () => {
    const root = render(create, [approval, card({ session: "s2", kind: "approval" })]);
    assert.deepEqual(root.find("meta")[0]?.children.map((element) => element.textContent), ["Claude Code", "waiting 3m", "+1 more"]);
    assert.equal(root.find("headline")[0]?.textContent, "Needs approval");
    assert.deepEqual(root.find("label").map((element) => element.textContent), ["Project", "Request"]);
    assert.equal(root.find("where")[0]?.textContent, "web-app");
    assert.equal(root.find("request")[0]?.textContent, "Bash: npm run migrate");
    assert.equal(root.find("more")[0]?.textContent, "+1 more");
    assert.equal(root.find("icon").length, 0);
  });

  it("leaves out the rows privacy mode empties", () => {
    const root = render(create, [card({ kind: "approval" })]);
    assert.equal(root.find("facts").length, 0);
    assert.equal(root.find("more").length, 0);
  });

  it("keeps the headline on one line and the wait in its own case", () => {
    const style = styleOf(render(create, [approval]));
    assert.match(style, /\.approval \.headline \{[^}]*font-size: min\([^)]*vw\);[^}]*white-space: nowrap;/);
    assert.match(style, /\.approval \.meta > \.waiting \{[^}]*text-transform: none;/);
  });

  it("pulses its own border, holds it lit under reduced motion, and breaks only the command anywhere", () => {
    const style = styleOf(render(create, [approval]));
    assert.match(style, /\.approval::after \{[^}]*inset: -2px; border: 2px solid/);
    assert.match(style, /prefers-reduced-motion: reduce\) \{ \.approval::after \{ animation: none; opacity: 1; \}/);
    assert.equal(style.split("overflow-wrap: anywhere").length, 2);
    assert.match(style, /\.approval \.request \{[^}]*overflow-wrap: anywhere/);
    assert.match(style, /\.approval > div \{ overflow-wrap: break-word; \}/);
  });
});

describe("chrome palette", () => {
  it("the rings take the warn color from the theme, not a stray amber", () => {
    const rings = source("chrome-rings");
    assert.match(rings, /\.ring\.warn \{ --ring: var\(--warn, /);
    assert.doesNotMatch(rings, /#ffd23f/i);
    const theme = JSON.parse(readFileSync(new URL("../../extensions/builtin.chrome/extension.json", import.meta.url), "utf8")).theme.style;
    assert.ok(theme["--warn"] !== undefined);
  });

  it("done has its own tone: lime stays for live work", () => {
    const theme = JSON.parse(readFileSync(new URL("../../extensions/builtin.chrome/extension.json", import.meta.url), "utf8")).theme.style;
    assert.notEqual(theme["--done"], theme["--accent"]);
  });

  it("the Chrome layout uses the chrome cards, the default themes keep the shared ones", () => {
    const layout = readFileSync(new URL("../../extensions/builtin.chrome-layout/extension.json", import.meta.url), "utf8");
    assert.match(layout, /builtin\.chrome-tasks/);
    assert.match(layout, /builtin\.chrome-approval/);
    assert.doesNotMatch(layout, /builtin\.task-row|builtin\.approval-card/);
  });
});

describe("chrome scene shader", () => {
  it("moves the lifted shape left of the rings only on a narrow screen", () => {
    assert.equal(liftShift(1512 / 982), 0);
    assert.equal(liftShift(16 / 9), 0);
    assert.ok(liftShift(4 / 3) < -0.15, String(liftShift(4 / 3)));
    assert.equal(liftShift(0), 0);
  });

  it("keeps the shading out of the vertex shader", () => {
    assert.doesNotMatch(VERTEX_SHADER, /surface|normalAt/);
    assert.equal((FRAGMENT_SHADER.match(/vec3 surface\(/g) ?? []).length, 1);
  });

  it("lifts the shape with uLift and softens its outline from the nearest miss", () => {
    assert.match(FRAGMENT_SHADER, /uniform float uLift;/);
    assert.match(FRAGMENT_SHADER, /mix\(SHAPE_CENTER, vec3\(uLiftX, SHAPE_LIFTED_YZ\), uLift\)/);
    assert.match(FRAGMENT_SHADER, /float cover = /);
  });
});
