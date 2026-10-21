import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { Card, CardKind } from "../src/api.ts";
import { approvalQueue, screenMode } from "../src/mode.ts";

function card(session: string, kind: CardKind, approvalSinceMs?: number): Card {
  return {
    source: "claude",
    session,
    kind,
    started_at_ms: 0,
    updated_at_ms: 0,
    ...(approvalSinceMs === undefined ? {} : { approval_since_ms: approvalSinceMs }),
  };
}

describe("screenMode", () => {
  it("secondary_display_mode", () => {
    assert.equal(screenMode([card("a", "approval", 1)], "secondary"), "secondary");
  });
  it("attention_when_any_approval", () => {
    assert.equal(screenMode([card("a", "running"), card("b", "approval", 1)], "main"), "attention");
  });
  it("idle_without_approval", () => {
    assert.equal(screenMode([card("a", "running"), card("b", "finished")], "main"), "idle");
    assert.equal(screenMode([], "main"), "idle");
  });
});

describe("approvalQueue", () => {
  it("approval_queue_oldest_first_and_counts_more", () => {
    const queue = approvalQueue([
      card("late", "approval", 300),
      card("running", "running"),
      card("oldest", "approval", 100),
      card("middle", "approval", 200),
    ]);
    assert.equal(queue.first?.session, "oldest");
    assert.equal(queue.more, 2);
  });
  it("empty_when_no_approvals", () => {
    assert.deepEqual(approvalQueue([card("a", "running")]), { first: undefined, more: 0 });
  });
  it("card_without_since_sorts_last", () => {
    const queue = approvalQueue([card("none", "approval"), card("has", "approval", 500)]);
    assert.equal(queue.first?.session, "has");
    assert.equal(queue.more, 1);
  });
  it("ties_keep_input_order", () => {
    const queue = approvalQueue([card("one", "approval", 5), card("two", "approval", 5)]);
    assert.equal(queue.first?.session, "one");
    const noneQueue = approvalQueue([card("x", "approval"), card("y", "approval")]);
    assert.equal(noneQueue.first?.session, "x");
  });
  it("does_not_mutate_input", () => {
    const cards = [card("b", "approval", 2), card("a", "approval", 1)];
    approvalQueue(cards);
    assert.equal(cards[0]?.session, "b");
  });
});
