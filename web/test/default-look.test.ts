// Pins what the Default and Calm themes draw: the task row and the approval card
// they use must come out exactly as before when another theme (Chrome) changes
// its own components. A change here is a visible change to the default look.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { before, describe, it } from "node:test";
import type { Card, ComponentExt, ViewState } from "../src/api.ts";
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
  mode: cards.some((one) => one.kind === "approval") ? "attention" : "idle",
  cards,
  agents: { claude: { name: "Claude Code", mark_url: "file:///marks/claude.svg" }, codex: { name: "Codex", mark_url: "" } },
  agentOrder: ["claude", "codex"],
  limits: [],
  nowMs: NOW_MS,
  serviceOk: true,
  display: "main",
});

const sha = (text: string): string => createHash("sha256").update(text).digest("hex").slice(0, 16);

/** One line per element: tag, classes, the attributes that show, its own text; children indented. */
function dump(element: FakeElement, depth = 0): string {
  const pad = "  ".repeat(depth);
  if (element.tag === "style") return `${pad}style ${sha(element.textContent)}`;
  const parts = [element.tag + (element.className === "" ? "" : `.${element.className.replaceAll(" ", ".")}`)];
  if (element.dir !== "") parts.push(`dir=${element.dir}`);
  for (const [name, value] of element.attributes) parts.push(`${name}=${value}`);
  for (const [name, value] of Object.entries(element.dataset)) parts.push(`data-${name}=${value}`);
  if (element.src !== "") parts.push(`src=${element.src}`);
  if (element.alt !== "") parts.push(`alt=${element.alt}`);
  if (element.innerHTML !== "") parts.push(`html=${sha(element.innerHTML)}`);
  const width = (element.style as unknown as { width?: string }).width;
  if (width !== undefined) parts.push(`width=${width}`);
  if (element.textContent !== "") parts.push(JSON.stringify(element.textContent));
  return [pad + parts.join(" "), ...element.children.map((child) => dump(child, depth + 1))].join("\n");
}

function render(create: () => ComponentExt, cards: Card[]): string {
  const root = new FakeElement("div");
  const component = create();
  component.mount(root as unknown as HTMLElement, fakeCtx(NOW_MS).ctx);
  component.update(view(cards));
  return dump(root);
}

const working = card({ name: "refactor auth middleware", project: "web-app", model: "Opus 5.5", branch: "feat/auth", context_percent: 34, background: 2 });
const done = card({ source: "codex", session: "s2", kind: "finished", title: "add rate limit tests", project: "web-app", model: "gpt-5.6", updated_at_ms: NOW_MS - 20_000 });
const quiet = card({ source: "aider", session: "s3", kind: "no_activity", project: "api", updated_at_ms: NOW_MS - 61 * MINUTE_MS });
const approval = card({ kind: "approval", project: "web-app", request: "Bash: npm run migrate", approval_since_ms: NOW_MS - 3 * MINUTE_MS });
const queued = card({ source: "codex", session: "s4", kind: "approval", project: "api", approval_since_ms: NOW_MS - MINUTE_MS });

const TASK_ROW = `div
  style 440b93031f87f06b
  div.tasks
    div.task.working
      div.top data-source=claude
        img.agent.source-mark src=file:///marks/claude.svg alt=Claude Code
        span.status "Working · 10m"
          span.mark html=3b6927d293aedcd2
        span.source-name "Claude Code"
      div.heading dir=auto "refactor auth middleware"
      div.context
        span.label "Context"
        span.track
          span.fill width=34%
        span.value "34%"
      div.where.counted
        span.project dir=auto "web-app"
        span.model dir=auto "Opus 5.5"
        span.branch dir=auto "feat/auth"
        span.background "2 in background"
    div.task.done
      div.top data-source=codex
        span.agent.source-dot aria-hidden=true
        span.status "Done · just now"
          span.mark html=0f77d79e41578f15
        span.source-name "Codex"
      div.heading dir=auto "add rate limit tests"
      div.where
        span.project dir=auto "web-app"
        span.model dir=auto "gpt-5.6"
    div.task.quiet
      div.top data-source=aider
        span.agent.source-dot aria-hidden=true
        span.status "No activity · 1h 1m"
          span.mark html=c130095b601fbaca
        span.source-name "Aider"
      div.heading dir=auto "api"
      div.where`;

const APPROVAL_CARD = `div
  style 570f8818aa740f11
  div.approval
    div.meta dir=auto "Claude Code · waiting 3m"
    div.headline dir=auto "Needs approval"
      span.icon html=7065f007a2ba3a62
    div.where dir=auto "web-app"
    div.request dir=auto "Bash: npm run migrate"
    div.more dir=auto "+1 more"`;

describe("the default look stays as it was", () => {
  let taskRow: () => ComponentExt;
  let approvalCard: () => ComponentExt;

  before(async () => {
    taskRow = await loadComponent("file:///ext/builtin.task-row/index.js", () => import("../src/builtins/task-row/index.ts"));
    approvalCard = await loadComponent("file:///ext/builtin.approval-card/index.js", () => import("../src/builtins/approval-card/index.ts"));
  });

  it("the task row draws the same elements, text and style", () => {
    assert.equal(render(taskRow, [working, done, quiet]), TASK_ROW);
  });

  it("the approval card draws the same elements, text and style", () => {
    assert.equal(render(approvalCard, [working, approval, queued]), APPROVAL_CARD);
  });

  it("the Default and Calm themes keep their style values", () => {
    const style = (id: string): unknown => JSON.parse(readFileSync(new URL(`../../extensions/${id}/extension.json`, import.meta.url), "utf8")).theme.style;
    assert.deepEqual(style("builtin.default"), { "--accent": "#00e07a", "--attention": "#ffb020", "--done": "#78c8ff", "--card-opacity": "0.82" });
    assert.deepEqual(style("builtin.calm"), { "--accent": "#6cc5b9", "--attention": "#ff9a4d", "--done": "#a9c4f5", "--card-opacity": "0.86" });
  });

  it("the Default and Calm themes keep the spotlight layout with the shared components", () => {
    const layout = readFileSync(new URL("../../extensions/builtin.spotlight/extension.json", import.meta.url), "utf8");
    assert.equal(sha(layout), "49e0b337ec1f2778");
  });
});
