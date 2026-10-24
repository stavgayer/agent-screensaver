import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { msToNextMinute, startMinuteTick } from "../src/tick.ts";

describe("msToNextMinute", () => {
  const cases: [string, number, number][] = [
    ["on a boundary waits a whole minute", 120_000, 60_000],
    ["mid minute waits for the boundary", 125_500, 54_500],
    ["just before a boundary waits for the one after", 179_400, 60_600],
  ];
  for (const [name, nowMs, want] of cases) {
    it(name, () => assert.equal(msToNextMinute(nowMs), want));
  }
});

describe("startMinuteTick", () => {
  it("ticks on each minute boundary and sets the next timer from the clock", () => {
    let nowMs = 1_800_030_000; // 30 s into a minute
    const timers: { callback: () => void; delayMs: number }[] = [];
    let ticks = 0;
    startMinuteTick(() => ticks++, () => nowMs, (callback, delayMs) => timers.push({ callback, delayMs }));

    assert.equal(ticks, 0);
    assert.equal(timers.length, 1);
    assert.equal(timers[0].delayMs, 30_000);

    nowMs += 30_050; // the timer fired a little late
    timers[0].callback();
    assert.equal(ticks, 1);
    assert.equal(timers.length, 2);
    assert.equal(timers[1].delayMs, 59_950, "the next tick is back on the boundary");

    nowMs += 59_950;
    timers[1].callback();
    assert.equal(ticks, 2);
  });
});
