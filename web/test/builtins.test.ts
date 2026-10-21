import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { Card, Limit } from "../src/api.ts";
import { displayName, formatResetDate } from "../src/format.ts";
import { approvalMoreLabel, approvalWaitingLabel } from "../src/builtins/approval-card/logic.ts";
import { orderLimits, ringCaption, ringPercent } from "../src/builtins/limit-rings/logic.ts";
import { advanceDrop, charsetFor, fadeAlpha, numberOption } from "../src/builtins/matrix/logic.ts";
import { taskCards, taskLabel } from "../src/builtins/task-row/logic.ts";

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

describe("taskLabel", () => {
  it("task_row_label_running", () => {
    const running = card({ kind: "running", started_at_ms: NOW_MS - 24 * MINUTE_MS });
    assert.equal(taskLabel(running, NOW_MS), "running 24m");
  });

  it("task_row_label_finished", () => {
    const finished = card({ kind: "finished", updated_at_ms: NOW_MS - 8 * MINUTE_MS });
    assert.equal(taskLabel(finished, NOW_MS), "finished 8m ago");
  });

  it("unknown and no_activity have fixed labels", () => {
    assert.equal(taskLabel(card({ kind: "unknown" }), NOW_MS), "check the terminal");
    assert.equal(taskLabel(card({ kind: "no_activity" }), NOW_MS), "no activity");
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
  it("rings_order_claude_then_codex", () => {
    const shuffled = [
      limit("codex", "week"),
      limit("claude", "week"),
      limit("codex", "5h"),
      limit("claude", "5h"),
    ];
    assert.deepEqual(
      orderLimits(shuffled).map((entry) => `${entry.source} ${entry.window}`),
      ["claude 5h", "claude week", "codex 5h", "codex week"],
    );
  });

  it("rings_order_includes_plan_after_week", () => {
    const shuffled = [limit("codex", "plan"), limit("claude", "plan"), limit("claude", "week"), limit("claude", "5h")];
    assert.deepEqual(
      orderLimits(shuffled).map((entry) => `${entry.source} ${entry.window}`),
      ["claude 5h", "claude week", "claude plan", "codex plan"],
    );
  });

  it("puts unknown sources after Claude and Codex, sorted by name", () => {
    const ordered = orderLimits([limit("zed", "5h"), limit("aider", "week"), limit("codex", "5h"), limit("claude", "5h")]);
    assert.deepEqual(ordered.map((entry) => entry.source), ["claude", "codex", "aider", "zed"]);
  });

  it("does not change its input", () => {
    const input = [limit("codex", "5h"), limit("claude", "5h")];
    orderLimits(input);
    assert.equal(input[0].source, "codex");
  });
});

describe("ringCaption", () => {
  const resetsAtS = (NOW_MS + 100 * MINUTE_MS) / 1000;

  it("a 5h ring shows the time until reset", () => {
    assert.equal(ringCaption(limit("claude", "5h", { resets_at: resetsAtS }), NOW_MS), "Claude 5h · 1h40m");
  });

  it("a 5h ring without resets_at shows only the name", () => {
    assert.equal(ringCaption(limit("codex", "5h"), NOW_MS), "Codex 5h");
  });

  it("a week ring shows no reset time", () => {
    assert.equal(ringCaption(limit("claude", "week", { resets_at: resetsAtS }), NOW_MS), "Claude week");
  });

  const DAY_MS = 24 * 60 * MINUTE_MS;
  const planAt = (offsetMs: number): Limit => limit("codex", "plan", { resets_at: (NOW_MS + offsetMs) / 1000 });

  it("plan_caption_is_source_plan", () => {
    assert.ok(ringCaption(limit("codex", "plan"), NOW_MS).startsWith("Codex plan"));
  });

  it("plan_reset_far_shows_date", () => {
    const far = planAt(13 * DAY_MS);
    const date = new Date(NOW_MS + 13 * DAY_MS);
    assert.equal(ringCaption(far, NOW_MS), `Codex plan · ${formatResetDate(date.getTime() / 1000)}`);
    assert.match(ringCaption(far, NOW_MS), /^Codex plan · resets \d{1,2} [A-Z][a-z]{2}$/);
  });

  it("plan_reset_near_shows_countdown", () => {
    assert.equal(ringCaption(planAt(100 * MINUTE_MS), NOW_MS), "Codex plan · 1h40m");
    assert.equal(ringCaption(planAt(DAY_MS), NOW_MS), "Codex plan · 24h");
  });

  it("plan_without_reset_shows_percent_only", () => {
    assert.equal(ringCaption(limit("codex", "plan"), NOW_MS), "Codex plan");
  });

  it("a ring older than 15 minutes says how old it is", () => {
    const stale = limit("claude", "week", { updated_at_ms: NOW_MS - 20 * MINUTE_MS });
    assert.equal(ringCaption(stale, NOW_MS), "Claude week · updated 20m ago");
  });

  it("a ring of exactly 15 minutes is not stale yet", () => {
    const edge = limit("claude", "week", { updated_at_ms: NOW_MS - 15 * MINUTE_MS });
    assert.equal(ringCaption(edge, NOW_MS), "Claude week");
  });
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
