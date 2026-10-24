import { describe, it } from "node:test";
import assert from "node:assert/strict";
import {
  formatClock,
  formatDate,
  formatDuration,
  formatCountdown,
  formatDayTime,
  formatDayMonth,
  formatResetLabel,
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

describe("formatCountdown", () => {
  const HOUR_MS = 60 * MINUTE_MS;
  const cases: Array<[string, number, string]> = [
    ["hours and minutes", 4 * HOUR_MS + 12 * MINUTE_MS, "4h 12m"],
    ["rounds down, 4h 12m 59s is still 4h 12m", 4 * HOUR_MS + 12 * MINUTE_MS + 59_000, "4h 12m"],
    ["a whole hour keeps its minutes", 2 * HOUR_MS, "2h 0m"],
    ["one minute under two hours", 2 * HOUR_MS - MINUTE_MS, "1h 59m"],
    ["exactly one hour", HOUR_MS, "1h 0m"],
    ["just under an hour", HOUR_MS - 1, "59m"],
    ["minutes only", 47 * MINUTE_MS, "47m"],
    ["exactly one minute", MINUTE_MS, "1m"],
    ["just under a minute", MINUTE_MS - 1, "under 1m"],
    ["one second", 1_000, "under 1m"],
    ["due exactly now", 0, "Reset"],
    ["past due", -5 * MINUTE_MS, "Reset"],
  ];
  for (const [name, remainingMs, want] of cases) {
    it(name, () => assert.equal(formatCountdown(remainingMs), want));
  }
});

describe("formatDayTime", () => {
  const MONDAY_9_UTC = Date.UTC(2026, 9, 26, 9, 0);
  const cases: Array<[string, number, number, string]> = [
    ["utc", MONDAY_9_UTC, 0, "Mon 09:00"],
    ["east of utc moves the clock forward", MONDAY_9_UTC, 120, "Mon 11:00"],
    ["west of utc can move to the day before", MONDAY_9_UTC, -600, "Sun 23:00"],
    ["east of utc can move to the next day", Date.UTC(2026, 9, 26, 23, 30), 180, "Tue 02:30"],
    ["pads hours and minutes", Date.UTC(2026, 9, 24, 5, 7), 0, "Sat 05:07"],
    ["a half-hour offset", MONDAY_9_UTC, 330, "Mon 14:30"],
  ];
  for (const [name, atMs, offset, want] of cases) {
    it(name, () => assert.equal(formatDayTime(atMs, offset), want));
  }
});

describe("formatResetLabel", () => {
  const HOUR_MS = 60 * MINUTE_MS;
  const DAY_MS = 24 * HOUR_MS;
  const nowMs = Date.UTC(2026, 9, 23, 7, 0);
  const inS = (ms: number): number => (nowMs + ms) / 1000;
  const utc = (): number => 0;
  const cases: Array<[string, number | undefined, string]> = [
    ["unknown is empty", undefined, ""],
    ["a countdown inside a day", inS(4 * HOUR_MS + 12 * MINUTE_MS), "4h 12m"],
    ["exactly 24 hours is still a countdown", inS(DAY_MS), "24h 0m"],
    ["a second past 24 hours is a day and time", inS(DAY_MS + 1_000), "Sat 07:00"],
    ["days away is a day and time", inS(3 * DAY_MS + 2 * HOUR_MS), "Mon 09:00"],
    ["exactly 6 days away is still a day and time", inS(6 * DAY_MS), "Thu 07:00"],
    // A weekday a week or more away reads as this week: Codex showed "Sun 02:00" for a reset 8 days away.
    ["more than 6 days away is the date and time", inS(6 * DAY_MS + MINUTE_MS), "29 Oct 07:01"],
    ["8 days away is the date and time", Date.UTC(2026, 9, 31, 23, 0) / 1000, "31 Oct 23:00"],
    ["due is Reset", inS(0), "Reset"],
  ];
  for (const [name, resetsAtS, want] of cases) {
    it(name, () => assert.equal(formatResetLabel(resetsAtS, nowMs, utc), want));
  }

  it("a far date leaves the time out when the caption has no room", () => {
    assert.equal(formatResetLabel(Date.UTC(2026, 10, 1, 0, 0) / 1000, nowMs, utc, { short: true }), "1 Nov");
  });

  it("a near reset is the same with or without room", () => {
    assert.equal(formatResetLabel(inS(3 * DAY_MS + 2 * HOUR_MS), nowMs, utc, { short: true }), "Mon 09:00");
    assert.equal(formatResetLabel(inS(4 * HOUR_MS), nowMs, utc, { short: true }), "4h 0m");
  });

  it("the date follows the offset of the reset instant", () => {
    // 31 Oct 23:00 UTC is 1 Nov 02:00 three hours east of UTC.
    assert.equal(formatResetLabel(Date.UTC(2026, 9, 31, 23, 0) / 1000, nowMs, () => 180), "1 Nov 02:00");
  });

  it("uses the UTC offset of the reset instant, not of now (the clocks change in between)", () => {
    // The clocks go back one hour at changeMs: now is on summer time, the reset is after the change.
    const changeMs = Date.UTC(2026, 9, 24, 23, 0);
    const offsetAt = (atMs: number): number => (atMs >= changeMs ? 120 : 180);
    const resetsAtMs = Date.UTC(2026, 9, 26, 7, 0);
    assert.equal(formatResetLabel(resetsAtMs / 1000, nowMs, offsetAt), "Mon 09:00");
  });
});

describe("formatDayMonth", () => {
  const cases: Array<[string, number, number, string]> = [
    ["utc", Date.UTC(2026, 10, 1, 9, 0), 0, "1 Nov"],
    ["two-digit day", Date.UTC(2026, 11, 25, 9, 0), 0, "25 Dec"],
    ["east of utc can move to the next day and month", Date.UTC(2026, 9, 31, 23, 0), 180, "1 Nov"],
    ["west of utc can move to the day before", Date.UTC(2027, 0, 1, 2, 0), -300, "31 Dec"],
  ];
  for (const [name, atMs, offset, want] of cases) {
    it(name, () => assert.equal(formatDayMonth(atMs, offset), want));
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
