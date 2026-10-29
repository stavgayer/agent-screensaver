import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { createFrameGate } from "../settings/frame-gate.ts";

const INTERVAL = 100;

/** A fake clock: timers run only when the test moves the time on. */
function fakeClock() {
  let nowMs = 0;
  let timers: { atMs: number; callback: () => void }[] = [];
  return {
    now: () => nowMs,
    setTimer(callback: () => void, delayMs: number): void {
      timers.push({ atMs: nowMs + delayMs, callback });
    },
    advance(toMs: number): void {
      for (;;) {
        const due = timers.filter((timer) => timer.atMs <= toMs).sort((a, b) => a.atMs - b.atMs)[0];
        if (due === undefined) break;
        timers = timers.filter((timer) => timer !== due);
        nowMs = due.atMs;
        due.callback();
      }
      nowMs = toMs;
    },
    timerCount: () => timers.length,
  };
}

/** A callback that asks for the next frame each time, like a render loop. */
function loop(gate: ReturnType<typeof createFrameGate>, frames: number[]): void {
  const frame = (nowMs: number) => {
    frames.push(nowMs);
    gate.request(frame);
  };
  gate.request(frame);
}

describe("createFrameGate", () => {
  it("runs the first request at once and then at most once per interval", () => {
    // Given
    const clock = fakeClock();
    const gate = createFrameGate(clock, INTERVAL);
    const frames: number[] = [];

    // When
    loop(gate, frames);
    clock.advance(350);

    // Then
    assert.deepEqual(frames, [0, 100, 200, 300]);
  });

  it("runs all waiting callbacks in one frame, in the order they came", () => {
    const clock = fakeClock();
    const gate = createFrameGate(clock, INTERVAL);
    const ran: string[] = [];

    gate.request(() => ran.push("first"));
    gate.request(() => ran.push("second"));
    clock.advance(0);

    assert.deepEqual(ran, ["first", "second"]);
  });

  it("holds callbacks while paused and runs them on resume", () => {
    const clock = fakeClock();
    const gate = createFrameGate(clock, INTERVAL);
    const frames: number[] = [];
    loop(gate, frames);
    clock.advance(100);

    gate.setPaused(true);
    clock.advance(1000);
    assert.deepEqual(frames, [0, 100], "nothing runs while paused");
    assert.equal(clock.timerCount(), 0, "no timer is left waiting");

    gate.setPaused(false);
    clock.advance(1000);
    assert.deepEqual(frames, [0, 100, 1000]);
  });

  it("does not run a cancelled callback", () => {
    const clock = fakeClock();
    const gate = createFrameGate(clock, INTERVAL);
    const ran: string[] = [];

    const id = gate.request(() => ran.push("cancelled"));
    gate.request(() => ran.push("kept"));
    gate.cancel(id);
    clock.advance(0);

    assert.deepEqual(ran, ["kept"]);
  });

  it("goes on with the other callbacks when one throws", () => {
    const clock = fakeClock();
    const gate = createFrameGate(clock, INTERVAL);
    const ran: string[] = [];
    const quiet = console.error;
    console.error = () => {};

    gate.request(() => {
      throw new Error("broken extension");
    });
    gate.request(() => ran.push("after"));
    try {
      clock.advance(0);
    } finally {
      console.error = quiet;
    }

    assert.deepEqual(ran, ["after"]);
  });

  it("sets one timer however many callbacks wait", () => {
    const clock = fakeClock();
    const gate = createFrameGate(clock, INTERVAL);

    gate.request(() => {});
    gate.request(() => {});
    gate.setPaused(false);

    assert.equal(clock.timerCount(), 1);
  });
});
