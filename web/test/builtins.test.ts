import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { Card, Limit } from "../src/api.ts";
import { displayName } from "../src/format.ts";
import { approvalMoreLabel, approvalWaitingLabel } from "../src/builtins/approval-card/logic.ts";
import { orderLimits, ringCaption, ringPercent, ringView } from "../src/builtins/limit-rings/logic.ts";
import { advanceDrop, charsetFor, fadeAlpha } from "../src/builtins/matrix/logic.ts";
import { numberOption } from "../src/builtins/shared/options.ts";
import { CONTEXT_HIGH_PERCENT, taskCards, taskContext, taskHeading, taskSource, taskStatus, taskWhere } from "../src/builtins/task-row/logic.ts";

const MINUTE_MS = 60_000;
const NOW_MS = 10_000_000_000;

const card = (overrides: Partial<Card>): Card => ({
  source: "claude",
  session: "s1",
  kind: "running",
  started_at_ms: NOW_MS,
  updated_at_ms: NOW_MS,
  ...overrides,
});

const limit = (source: string, window: Limit["window"], overrides: Partial<Limit> = {}): Limit => ({
  source,
  window,
  used_percent: 10,
  updated_at_ms: NOW_MS,
  ...overrides,
});

describe("displayName", () => {
  const cases: Array<[string, string]> = [
    ["claude", "Claude"],
    ["codex", "Codex"],
    ["aider", "Aider"],
    ["", ""],
  ];
  for (const [source, want] of cases) {
    it(`${JSON.stringify(source)} is ${JSON.stringify(want)}`, () => assert.equal(displayName(source), want));
  }
});

describe("taskStatus", () => {
  const cases: Array<[string, Partial<Card>, { tone: string; text: string }]> = [
    ["running counts from the start", { kind: "running", started_at_ms: NOW_MS - 24 * MINUTE_MS }, { tone: "working", text: "Working · 24m" }],
    ["running over an hour", { kind: "running", started_at_ms: NOW_MS - 72 * MINUTE_MS }, { tone: "working", text: "Working · 1h 12m" }],
    ["running just started", { kind: "running", started_at_ms: NOW_MS - 10_000 }, { tone: "working", text: "Working · <1m" }],
    ["done counts from the finish", { kind: "finished", updated_at_ms: NOW_MS - 3 * MINUTE_MS }, { tone: "done", text: "Done · 3m ago" }],
    ["done under a minute ago", { kind: "finished", updated_at_ms: NOW_MS - 20_000 }, { tone: "done", text: "Done · just now" }],
    ["no activity counts from the last event", { kind: "no_activity", updated_at_ms: NOW_MS - 60 * MINUTE_MS }, { tone: "quiet", text: "No activity · 1h" }],
    ["unknown sends you to the terminal", { kind: "unknown" }, { tone: "check", text: "Check the terminal" }],
    ["approval needs you", { kind: "approval" }, { tone: "check", text: "Needs you" }],
  ];
  for (const [name, overrides, want] of cases) {
    it(name, () => assert.deepEqual(taskStatus(card(overrides), NOW_MS), want));
  }
});

describe("taskHeading", () => {
  const cases: Array<[string, Partial<Card>, string]> = [
    ["the conversation name comes first", { name: "Login fix", title: "Fix the login bug", project: "web-app" }, "Login fix"],
    ["then the prompt title", { title: "Fix the login bug", project: "web-app" }, "Fix the login bug"],
    ["privacy mode: the project", { project: "web-app" }, "web-app"],
    ["nothing else: the agent", { source: "codex" }, "Codex"],
  ];
  for (const [name, overrides, want] of cases) {
    it(name, () => assert.equal(taskHeading(card(overrides)), want));
  }
});

describe("taskWhere", () => {
  type Where = ReturnType<typeof taskWhere>;
  const cases: Array<[string, Partial<Card>, Where]> = [
    ["project only", { title: "Fix", project: "web-app" }, { parts: [{ role: "project", text: "web-app" }], background: "" }],
    [
      "project, model and branch",
      { title: "Fix", project: "web-app", model: "Opus 5.5", branch: "feat/v1" },
      { parts: [{ role: "project", text: "web-app" }, { role: "model", text: "Opus 5.5" }, { role: "branch", text: "feat/v1" }], background: "" },
    ],
    ["background jobs are counted", { title: "Fix", project: "web-app", background: 2 }, { parts: [{ role: "project", text: "web-app" }], background: "2 in background" }],
    ["one background job", { title: "Fix", project: "web-app", background: 1 }, { parts: [{ role: "project", text: "web-app" }], background: "1 in background" }],
    ["zero jobs say nothing", { title: "Fix", project: "web-app", background: 0 }, { parts: [{ role: "project", text: "web-app" }], background: "" }],
    ["the project is left out when it is the heading", { project: "web-app", model: "gpt-5.6", background: 3 }, { parts: [{ role: "model", text: "gpt-5.6" }], background: "3 in background" }],
    ["nothing known", { source: "codex" }, { parts: [], background: "" }],
    ["empty fields are left out", { title: "Fix", project: "", model: "", branch: "" }, { parts: [], background: "" }],
  ];
  for (const [name, overrides, want] of cases) {
    it(name, () => assert.deepEqual(taskWhere(card(overrides)), want));
  }
});

describe("taskContext", () => {
  const cases: Array<[string, number | undefined, ReturnType<typeof taskContext>]> = [
    ["unknown hides the bar", undefined, undefined],
    ["empty", 0, { percent: 0, text: "0%", high: false }],
    ["calm below the threshold", 79.4, { percent: 79, text: "79%", high: false }],
    ["high from the threshold", CONTEXT_HIGH_PERCENT, { percent: 80, text: "80%", high: true }],
    ["full", 100, { percent: 100, text: "100%", high: true }],
    ["kept inside 0 to 100", 140, { percent: 100, text: "100%", high: true }],
    ["never below 0", -5, { percent: 0, text: "0%", high: false }],
    ["not a number hides the bar", Number.NaN, undefined],
  ];
  for (const [name, percent, want] of cases) {
    it(name, () => assert.deepEqual(taskContext(card({ context_percent: percent })), want));
  }
});

describe("taskSource", () => {
  const agents = {
    claude: { name: "Claude Code", mark_url: "file:///data/extensions/builtin.claude/mark.svg" },
    codex: { name: "Codex", mark_url: "file:///data/marks/codex.svg" },
  };
  it("card shows the agent mark from state", () => {
    assert.deepEqual(taskSource(card({ source: "claude" }), agents), {
      markUrl: "file:///data/extensions/builtin.claude/mark.svg",
      name: "Claude Code",
    });
  });
  it("a user's own mark comes the same way", () => {
    assert.equal(taskSource(card({ source: "codex" }), agents).markUrl, "file:///data/marks/codex.svg");
  });
  it("card without agent shows a neutral dot and the source name", () => {
    assert.deepEqual(taskSource(card({ source: "jira" }), agents), { markUrl: undefined, name: "Jira" });
  });
  it("an older state file without agents works", () => {
    assert.deepEqual(taskSource(card({ source: "claude" }), {}), { markUrl: undefined, name: "Claude" });
  });
});

describe("taskCards", () => {
  it("leaves out approval cards and keeps the input order", () => {
    const cards = [
      card({ session: "a", kind: "running" }),
      card({ session: "b", kind: "approval" }),
      card({ session: "c", kind: "finished" }),
    ];
    assert.deepEqual(taskCards(cards).map((entry) => entry.session), ["a", "c"]);
  });
});

describe("approval labels", () => {
  it("approval_label_more", () => {
    assert.equal(approvalMoreLabel(2), "+2 more");
    assert.equal(approvalMoreLabel(0), "");
  });

  it("waiting time counts from approval_since_ms, else from the last update", () => {
    const since = card({ kind: "approval", approval_since_ms: NOW_MS - 3 * MINUTE_MS });
    assert.equal(approvalWaitingLabel(since, NOW_MS), "waiting 3m");
    const noSince = card({ kind: "approval", updated_at_ms: NOW_MS - 5 * MINUTE_MS });
    assert.equal(approvalWaitingLabel(noSince, NOW_MS), "waiting 5m");
  });
});

describe("orderLimits", () => {
  const ORDER = ["claude", "codex"];
  it("follows the installed agents' order from the state", () => {
    const ordered = orderLimits([limit("claude", "5h"), limit("codex", "5h")], ["codex", "claude"]);
    assert.deepEqual(ordered.map((entry) => entry.source), ["codex", "claude"]);
  });
  it("without agents every source is sorted by name", () => {
    const ordered = orderLimits([limit("codex", "5h"), limit("aider", "5h")], []);
    assert.deepEqual(ordered.map((entry) => entry.source), ["aider", "codex"]);
  });

  it("rings_order_claude_then_codex", () => {
    const shuffled = [
      limit("codex", "week"),
      limit("claude", "week"),
      limit("codex", "5h"),
      limit("claude", "5h"),
    ];
    assert.deepEqual(
      orderLimits(shuffled, ORDER).map((entry) => `${entry.source} ${entry.window}`),
      ["claude 5h", "claude week", "codex 5h", "codex week"],
    );
  });

  it("rings_order_includes_plan_after_week", () => {
    const shuffled = [limit("codex", "plan"), limit("claude", "plan"), limit("claude", "week"), limit("claude", "5h")];
    assert.deepEqual(
      orderLimits(shuffled, ORDER).map((entry) => `${entry.source} ${entry.window}`),
      ["claude 5h", "claude week", "claude plan", "codex plan"],
    );
  });

  it("puts unknown sources after Claude and Codex, sorted by name", () => {
    const ordered = orderLimits([limit("zed", "5h"), limit("aider", "week"), limit("codex", "5h"), limit("claude", "5h")], ORDER);
    assert.deepEqual(ordered.map((entry) => entry.source), ["claude", "codex", "aider", "zed"]);
  });

  it("does not change its input", () => {
    const input = [limit("codex", "5h"), limit("claude", "5h")];
    orderLimits(input, ORDER);
    assert.equal(input[0].source, "codex");
  });
});

describe("ringCaption", () => {
  const HOUR_MS = 60 * MINUTE_MS;
  const DAY_MS = 24 * HOUR_MS;
  const utc = (): number => 0;
  const inS = (ms: number): number => (NOW_MS + ms) / 1000;
  const caption = (input: Limit): string => ringCaption(input, NOW_MS, utc);

  it("a 5h ring shows the countdown to the minute", () => {
    assert.equal(caption(limit("claude", "5h", { resets_at: inS(4 * HOUR_MS + 12 * MINUTE_MS) })), "Claude 5h · 4h 12m");
  });

  it("a 5h ring under an hour shows minutes only", () => {
    assert.equal(caption(limit("claude", "5h", { resets_at: inS(47 * MINUTE_MS) })), "Claude 5h · 47m");
  });

  it("a 5h ring under a minute says so", () => {
    assert.equal(caption(limit("claude", "5h", { resets_at: inS(30_000) })), "Claude 5h · under 1m");
  });

  it("a 5h ring without resets_at shows only the name", () => {
    assert.equal(caption(limit("codex", "5h")), "Codex 5h");
  });

  it("a week ring far from its reset shows the day and time", () => {
    // NOW_MS is Sunday 26 April 1970 17:46:40 UTC; three days and two hours later is Wednesday 19:46.
    assert.equal(caption(limit("claude", "week", { resets_at: inS(3 * DAY_MS + 2 * HOUR_MS) })), "Claude week · Wed 19:46");
  });

  it("a week ring within a day shows the countdown", () => {
    assert.equal(caption(limit("claude", "week", { resets_at: inS(20 * HOUR_MS + 5 * MINUTE_MS) })), "Claude week · 20h 5m");
  });

  it("a week ring without resets_at shows only the name", () => {
    assert.equal(caption(limit("claude", "week")), "Claude week");
  });

  it("a plan ring up to 6 days from its reset shows the day and time", () => {
    assert.equal(caption(limit("codex", "plan", { resets_at: inS(6 * DAY_MS) })), "Codex plan · Sat 17:46");
  });

  it("a plan ring more than 6 days from its reset shows the date, so it is not read as this week", () => {
    assert.equal(caption(limit("codex", "plan", { resets_at: inS(13 * DAY_MS) })), "Codex plan · 9 May 17:46");
  });

  it("a plan ring within a day shows the countdown", () => {
    assert.equal(caption(limit("codex", "plan", { resets_at: inS(100 * MINUTE_MS) })), "Codex plan · 1h 40m");
    assert.equal(caption(limit("codex", "plan", { resets_at: inS(DAY_MS) })), "Codex plan · 24h 0m");
  });

  it("a plan ring without resets_at shows only the name", () => {
    assert.equal(caption(limit("codex", "plan")), "Codex plan");
  });

  it("the day and time follow the offset passed in", () => {
    const plus3 = (): number => 180;
    assert.equal(ringCaption(limit("codex", "plan", { resets_at: inS(13 * DAY_MS) }), NOW_MS, plus3), "Codex plan · 9 May 20:46");
  });
});

describe("ringView", () => {
  const HOUR_MS = 60 * MINUTE_MS;
  const DAY_MS = 24 * HOUR_MS;
  const inS = (ms: number) => (NOW_MS + ms) / 1000;
  const utc = (): number => 0;
  type View = ReturnType<typeof ringView>;
  const cases: Array<[string, Limit, View]> = [
    ["a fresh 5h reading", limit("claude", "5h", { used_percent: 62, resets_at: inS(100 * MINUTE_MS) }),
      { look: "fresh", percent: 62, center: "62%", name: "Claude 5h", detail: "1h 40m" }],
    ["a fresh week reading with no reset time has no detail", limit("claude", "week", { used_percent: 38 }),
      { look: "fresh", percent: 38, center: "38%", name: "Claude week", detail: "" }],
    ["a week reading more than a day from its reset says day and time", limit("claude", "week", { used_percent: 38, resets_at: inS(2 * DAY_MS) }),
      { look: "fresh", percent: 38, center: "38%", name: "Claude week", detail: "Tue 17:46" }],
    ["a 5h reading of exactly 30 minutes is still fresh", limit("claude", "5h", { updated_at_ms: NOW_MS - 30 * MINUTE_MS }),
      { look: "fresh", percent: 10, center: "10%", name: "Claude 5h", detail: "" }],
    ["a 5h reading older than 30 minutes is stale", limit("claude", "5h", { updated_at_ms: NOW_MS - 31 * MINUTE_MS, resets_at: inS(100 * MINUTE_MS) }),
      { look: "stale", percent: 10, center: "10%", name: "Claude 5h", detail: "1h 40m · 31m ago" }],
    ["a week reading of 5 hours is still fresh", limit("claude", "week", { updated_at_ms: NOW_MS - 5 * HOUR_MS }),
      { look: "fresh", percent: 10, center: "10%", name: "Claude week", detail: "" }],
    ["a week reading older than 6 hours is stale, in whole hours", limit("claude", "week", { updated_at_ms: NOW_MS - (20 * HOUR_MS + 40 * MINUTE_MS) }),
      { look: "stale", percent: 10, center: "10%", name: "Claude week", detail: "20h ago" }],
    ["a plan reading older than 6 hours is stale", limit("codex", "plan", { updated_at_ms: NOW_MS - 7 * HOUR_MS }),
      { look: "stale", percent: 10, center: "10%", name: "Codex plan", detail: "7h ago" }],
    ["a stale reading days old says days", limit("codex", "plan", { updated_at_ms: NOW_MS - 50 * HOUR_MS }),
      { look: "stale", percent: 10, center: "10%", name: "Codex plan", detail: "2d ago" }],
    ["after the reset the old percent is not shown, the caption says Reset", limit("claude", "5h", { used_percent: 20, resets_at: inS(-MINUTE_MS), updated_at_ms: NOW_MS - 20 * HOUR_MS }),
      { look: "reset", percent: 0, center: "0%", name: "Claude 5h", detail: "Reset" }],
    ["the reset moment itself counts as reset", limit("claude", "week", { used_percent: 90, resets_at: inS(0) }),
      { look: "reset", percent: 0, center: "0%", name: "Claude week", detail: "Reset" }],
    ["a reset estimate does not say est.", limit("claude", "5h", { used_percent: 90, resets_at: inS(-1), estimated: true }),
      { look: "reset", percent: 0, center: "0%", name: "Claude 5h", detail: "Reset" }],
    ["a reading newer than the reset is shown again", limit("claude", "5h", { used_percent: 4, resets_at: inS(5 * HOUR_MS) }),
      { look: "fresh", percent: 4, center: "4%", name: "Claude 5h", detail: "5h 0m" }],
    ["an estimate says so after the reset time", limit("claude", "5h", { used_percent: 47.6, resets_at: inS(100 * MINUTE_MS), estimated: true }),
      { look: "fresh", percent: 48, center: "48%", name: "Claude 5h", detail: "1h 40m · est." }],
    ["an estimate with nothing else to say", limit("claude", "week", { used_percent: 61, estimated: true }),
      { look: "fresh", percent: 61, center: "61%", name: "Claude week", detail: "est." }],
    ["an old estimate is stale too", limit("claude", "5h", { updated_at_ms: NOW_MS - 31 * MINUTE_MS, estimated: true }),
      { look: "stale", percent: 10, center: "10%", name: "Claude 5h", detail: "31m ago · est." }],
    ["a far reset with more to say leaves the time out", limit("codex", "plan", { used_percent: 19, resets_at: inS(8 * DAY_MS), estimated: true }),
      { look: "fresh", percent: 19, center: "19%", name: "Codex plan", detail: "4 May · est." }],
    ["a far reset of a stale reading leaves the time out", limit("codex", "plan", { resets_at: inS(8 * DAY_MS), updated_at_ms: NOW_MS - 7 * HOUR_MS }),
      { look: "stale", percent: 10, center: "10%", name: "Codex plan", detail: "4 May · 7h ago" }],
    ["a near reset with more to say keeps the day and time", limit("claude", "week", { resets_at: inS(2 * DAY_MS), estimated: true }),
      { look: "fresh", percent: 10, center: "10%", name: "Claude week", detail: "Tue 17:46 · est." }],
    ["under a minute to the reset", limit("claude", "5h", { resets_at: inS(20_000) }),
      { look: "fresh", percent: 10, center: "10%", name: "Claude 5h", detail: "under 1m" }],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => assert.deepEqual(ringView(input, NOW_MS, utc), want));
  }
});

describe("ringPercent", () => {
  const cases: Array<[number, number]> = [[61.6, 62], [-5, 0], [140, 100], [Number.NaN, 0]];
  for (const [input, want] of cases) {
    it(`${input} becomes ${want}`, () => assert.equal(ringPercent(input), want));
  }
});

describe("matrix logic", () => {
  it("charsetFor maps names to fixed sets and falls back to katakana", () => {
    assert.equal(charsetFor("digits").join(""), "0123456789");
    assert.equal(charsetFor("latin").length, 26);
    assert.equal(charsetFor("hebrew").length, 27);
    assert.equal(charsetFor("hebrew")[0], "א");
    assert.equal(charsetFor("hebrew")[26], "ת");
    assert.deepEqual(charsetFor("nonsense"), charsetFor("katakana"));
    assert.ok(charsetFor("katakana").every((char) => char >= "ｦ" && char <= "ﾝ"));
  });

  it("numberOption clamps and falls back", () => {
    assert.equal(numberOption(5, 1, 0.25, 4), 4);
    assert.equal(numberOption(0, 1, 0.25, 4), 0.25);
    assert.equal(numberOption("fast", 1, 0.25, 4), 1);
    assert.equal(numberOption(2, 1, 0.25, 4), 2);
  });

  it("fadeAlpha is the same fade per second at any frame time", () => {
    const frame24 = fadeAlpha(1000 / 24);
    assert.ok(Math.abs(frame24 - 0.08) < 1e-9);
    const twoHalfFrames = 1 - (1 - fadeAlpha(1000 / 48)) ** 2;
    assert.ok(Math.abs(twoHalfFrames - frame24) < 1e-9);
  });

  it("advanceDrop moves by the given rows and wraps only below the screen", () => {
    assert.equal(advanceDrop(3, 50, 2, () => 0), 5);
    assert.equal(advanceDrop(60, 50, 1, () => 0.99), 61);
    assert.equal(advanceDrop(60, 50, 1, () => 0), 0);
  });
});

