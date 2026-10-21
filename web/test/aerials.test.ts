import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { createPlaylist, fileList, shuffled } from "../src/builtins/aerials/logic.ts";

/** A random source that returns the given numbers in turn, then repeats them. */
function sequence(...values: number[]): () => number {
  let index = 0;
  return () => values[index++ % values.length];
}

describe("fileList", () => {
  const cases: [string, unknown, string[]][] = [
    ["file URLs are kept in order", ["file:///a/1.mov", "file:///a/2.mov"], ["file:///a/1.mov", "file:///a/2.mov"]],
    ["anything that is not a file URL is dropped", ["file:///a/1.mov", "https://x/2.mov", "/a/3.mov", 4, null], ["file:///a/1.mov"]],
    ["an empty list", [], []],
    ["not a list", "file:///a/1.mov", []],
    ["nothing", undefined, []],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => assert.deepEqual(fileList(input), want));
  }
});

describe("shuffled", () => {
  it("keeps every item exactly once", () => {
    const items = ["a", "b", "c", "d", "e"];
    assert.deepEqual([...shuffled(items, Math.random)].sort(), items);
  });
  it("does not change the input", () => {
    const items = ["a", "b", "c"];
    shuffled(items, sequence(0));
    assert.deepEqual(items, ["a", "b", "c"]);
  });
  it("follows the random source (Fisher-Yates)", () => {
    // 0 always swaps with the first slot: [a,b,c] -> [c,b,a]? walk: i=2 j=0 -> [c,b,a]; i=1 j=0 -> [b,c,a]
    assert.deepEqual(shuffled(["a", "b", "c"], sequence(0)), ["b", "c", "a"]);
  });
});

describe("createPlaylist", () => {
  it("an empty list has nothing to play", () => {
    assert.equal(createPlaylist([], Math.random).next(), undefined);
  });
  it("one file plays again and again", () => {
    const playlist = createPlaylist(["file:///a.mov"], Math.random);
    assert.deepEqual([playlist.next(), playlist.next(), playlist.next()], ["file:///a.mov", "file:///a.mov", "file:///a.mov"]);
  });
  it("plays every file once before any repeats", () => {
    const files = ["1", "2", "3", "4"];
    const playlist = createPlaylist(files, Math.random);
    const round = [playlist.next(), playlist.next(), playlist.next(), playlist.next()];
    assert.deepEqual([...round].sort(), files);
  });
  it("never plays the same file twice in a row across rounds", () => {
    // 0.99 keeps the order as it is, so every round would start with the last one's end.
    const playlist = createPlaylist(["1", "2", "3"], sequence(0.99));
    const played = Array.from({ length: 12 }, () => playlist.next());
    for (let index = 1; index < played.length; index++) {
      assert.notEqual(played[index], played[index - 1], `repeat at ${index}: ${played.join(",")}`);
    }
  });
});
