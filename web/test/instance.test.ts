import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { ViewState } from "../src/api.ts";
import { createKeeper, type Instance } from "../src/instance.ts";

const view: ViewState = { mode: "idle", cards: [], limits: [], nowMs: 0, serviceOk: true, display: "main" };

function fakeItem(key: string, behaviour: { update?: () => void; stop?: () => void } = {}) {
  const calls = { removed: 0, stopped: 0, updated: 0 };
  const item: Instance = {
    key,
    root: { remove: () => void calls.removed++ },
    cancels: new Set(),
    stop() {
      calls.stopped++;
      behaviour.stop?.();
    },
    update() {
      calls.updated++;
      behaviour.update?.();
    },
    dead: false,
  };
  return { item, calls };
}

function keeper() {
  const failed = new Set<string>();
  const logged: string[] = [];
  return { failed, logged, ...createKeeper(failed, (label) => logged.push(label)) };
}

describe("createKeeper", () => {
  it("a throwing call tears the item down once and marks it failed", () => {
    const { guard, failed, logged } = keeper();
    const { item, calls } = fakeItem("bad");
    assert.equal(guard(item, "mount", () => { throw new Error("x"); }), undefined);
    assert.deepEqual([item.dead, calls.stopped, calls.removed], [true, 1, 1]);
    assert.deepEqual([...failed], ["bad"]);
    assert.deepEqual(logged, ["bad.mount"]);
  });

  it("update_skips_dead_items", () => {
    const { updateAll, guard } = keeper();
    const live = fakeItem("live");
    const dead = fakeItem("dead");
    guard(dead.item, "mount", () => { throw new Error("x"); });
    updateAll([live.item, dead.item], view);
    assert.deepEqual([live.calls.updated, dead.calls.updated], [1, 0]);
  });

  it("an item that throws in update is not updated again", () => {
    const { updateAll } = keeper();
    const { item, calls } = fakeItem("flaky", { update: () => { throw new Error("x"); } });
    updateAll([item], view);
    updateAll([item], view);
    assert.equal(calls.updated, 1);
  });

  it("request_frame_on_dead_item_is_a_noop", () => {
    const { teardown, trackFrame } = keeper();
    const { item } = fakeItem("gone");
    teardown(item);
    let requests = 0;
    const cancel = trackFrame(item, () => { requests++; return () => {}; });
    cancel();
    assert.equal(requests, 0);
  });

  it("teardown_cancels_frame_requests", () => {
    const { teardown, trackFrame } = keeper();
    const { item } = fakeItem("live");
    let cancelled = 0;
    trackFrame(item, () => () => void cancelled++);
    teardown(item);
    assert.equal(cancelled, 1);
  });
});
