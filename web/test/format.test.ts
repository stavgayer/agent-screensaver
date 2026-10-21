import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  formatClock,
  formatDate,
  formatDuration,
  formatResetIn,
  ringLevel,
} from "../src/format.ts";

const MINUTE_MS = 60_000;

describe("formatDuration", () => {
  const cases: Array<[string, number, string]> = [
    ["negative", -1, "<1m"],
    ["zero", 0, "<1m"],
    ["under a minute", 59_999, "<1m"],
    ["three minutes", 3 * MINUTE_MS, "3m"],
    ["floors partial minutes", 24 * MINUTE_MS + 59_000, "24m"],
    ["just under an hour", 59 * MINUTE_MS, "59m"],
    ["exactly an hour", 60 * MINUTE_MS, "1h"],
    ["hour and minutes", 72 * MINUTE_MS, "1h 12m"],
    ["whole hours", 120 * MINUTE_MS, "2h"],
  ];
  for (const [name, ms, want] of cases) {
    it(name, () => assert.equal(formatDuration(ms), want));
  }
});

describe("formatResetIn", () => {
  const nowMs = 1_800_000_000_000;
  const inMinutes = (minutes: number): number => (nowMs + minutes * MINUTE_MS) / 1000;

  const cases: Array<[string, number | undefined, string]> = [
    ["hour and minutes", inMinutes(100), "1h40m"],
    ["minutes only", inMinutes(35), "35m"],
    ["whole hours", inMinutes(120), "2h"],
    ["rounds up so it never shows 0m", nowMs / 1000 + 1, "1m"],
    ["rounds a partial minute up", nowMs / 1000 + 61, "2m"],
    ["reset_in_past_says_waiting", inMinutes(-5), "reset, waiting for new data"],
    ["reset exactly now says waiting", nowMs / 1000, "reset, waiting for new data"],
    ["undefined is empty", undefined, ""],
  ];
  for (const [name, resetsAtS, want] of cases) {
    it(name, () => assert.equal(formatResetIn(resetsAtS, nowMs), want));
  }
});

describe("ringLevel", () => {
  const cases: Array<[number, "ok" | "warn" | "high"]> = [
    [0, "ok"],
    [49.9, "ok"],
    [50, "warn"],
    [80, "warn"],
    [80.1, "high"],
    [100, "high"],
  ];
  for (const [pct, want] of cases) {
    it(`${pct} is ${want}`, () => assert.equal(ringLevel(pct), want));
  }
});

describe("formatClock", () => {
  it("pads to HH:MM in 24 hours", () => {
    assert.equal(formatClock(new Date(2026, 9, 20, 9, 5)), "09:05");
    assert.equal(formatClock(new Date(2026, 9, 20, 23, 59)), "23:59");
    assert.equal(formatClock(new Date(2026, 9, 20, 0, 0)), "00:00");
  });
});

describe("formatDate", () => {
  it("writes weekday, day and month in English", () => {
    assert.equal(formatDate(new Date(2026, 9, 20, 9, 5)), "Tuesday 20 October");
    assert.equal(formatDate(new Date(2026, 0, 4)), "Sunday 4 January");
  });
});
