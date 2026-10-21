import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { createFrameScheduler, effectiveFps, shouldRender } from "../src/frame.ts";

/** A fake rAF: callbacks run only when the test calls `tick`. */
function fakeRaf() {
  let nextId = 1;
  const pending = new Map<number, (nowMs: number) => void>();
  return {
    raf: (cb: (nowMs: number) => void): number => {
      pending.set(nextId, cb);
      return nextId++;
    },
    tick(nowMs: number): void {
      const callbacks = [...pending.values()];
      pending.clear();
      callbacks.forEach((cb) => cb(nowMs));
    },
    pendingCount: () => pending.size,
  };
}

describe("effectiveFps", () => {
  const cases: Array<[string, number, number, number]> = [
    ["runs_at_requested_fps_under_ceiling", 120, 120, 120],
    ["ceiling_wins_when_lower", 120, 30, 30],
    ["no_ceiling_means_requested", 120, 0, 120],
    ["missing_ceiling_means_requested", 60, Number.NaN, 60],
    ["infinite_ceiling_means_requested", 60, Infinity, 60],
    ["negative_ceiling_means_requested", 60, -5, 60],
    ["clamped_to_240", 1000, 0, 240],
    ["zero_request_becomes_1", 0, 0, 1],
    ["nan_request_becomes_1", Number.NaN, 0, 1],
    ["negative_request_becomes_1", -10, 0, 1],
    ["tiny_ceiling_clamped_to_1", 60, 0.2, 1],
  ];
  for (const [name, requested, maxFps, expected] of cases) {
    it(name, () => assert.equal(effectiveFps(requested, maxFps), expected));
  }
});

describe("shouldRender", () => {
  it("renders_after_interval", () => {
    assert.equal(shouldRender(0, 1000 / 30, 30), true);
    assert.equal(shouldRender(0, 1000 / 30 + 5, 30), true);
  });
  it("skips_before_interval", () => {
    assert.equal(shouldRender(0, 10, 30), false);
  });
});

describe("createFrameScheduler", () => {
  it("runs_a_request_at_its_fps", () => {
    const fake = fakeRaf();
    const scheduler = createFrameScheduler(0, fake.raf);
    const seen: number[] = [];
    scheduler.request((nowMs) => seen.push(nowMs), 10); // every 100 ms
    for (const nowMs of [0, 50, 100, 150, 200]) fake.tick(nowMs);
    assert.deepEqual(seen, [0, 100, 200]);
  });

  it("ceiling_limits_a_fast_request", () => {
    const fake = fakeRaf();
    const scheduler = createFrameScheduler(10, fake.raf);
    const seen: number[] = [];
    scheduler.request((nowMs) => seen.push(nowMs), 120);
    for (const nowMs of [0, 50, 100, 150, 200]) fake.tick(nowMs);
    assert.deepEqual(seen, [0, 100, 200]);
  });

  it("uses_one_shared_loop_for_all_requests", () => {
    const fake = fakeRaf();
    const scheduler = createFrameScheduler(0, fake.raf);
    scheduler.request(() => {}, 60);
    scheduler.request(() => {}, 30);
    assert.equal(fake.pendingCount(), 1);
  });

  it("cancel_stops_only_that_request", () => {
    const fake = fakeRaf();
    const scheduler = createFrameScheduler(0, fake.raf);
    const a: number[] = [];
    const b: number[] = [];
    const cancelA = scheduler.request((nowMs) => a.push(nowMs), 240);
    scheduler.request((nowMs) => b.push(nowMs), 240);
    fake.tick(0);
    cancelA();
    fake.tick(100);
    assert.deepEqual(a, [0]);
    assert.deepEqual(b, [0, 100]);
  });

  it("loop_stops_when_no_requests_remain_and_restarts_on_new_request", () => {
    const fake = fakeRaf();
    const scheduler = createFrameScheduler(0, fake.raf);
    const cancel = scheduler.request(() => {}, 60);
    cancel();
    fake.tick(0); // the already scheduled frame sees no requests and stops
    assert.equal(fake.pendingCount(), 0);
    scheduler.request(() => {}, 60);
    assert.equal(fake.pendingCount(), 1);
  });

  it("request_made_inside_a_callback_runs_on_a_later_frame", () => {
    const fake = fakeRaf();
    const scheduler = createFrameScheduler(0, fake.raf);
    const seen: number[] = [];
    scheduler.request(() => {
      scheduler.request((nowMs) => seen.push(nowMs), 240);
    }, 240);
    fake.tick(0);
    assert.deepEqual(seen, []);
    fake.tick(10);
    assert.deepEqual(seen, [10]);
  });
});
