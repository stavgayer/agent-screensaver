import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { BackgroundExt, Ctx, NowPlaying, ViewState } from "../src/api.ts";
import { recoverWebGL, songProgress } from "../src/helpers.ts";

const AT_MS = 1_000_000;
const song: NowPlaying = { app: "Spotify", title: "Sloop John B", artist: "The Beach Boys", position_ms: 60_000, duration_ms: 180_000, at_ms: AT_MS };

describe("songProgress", () => {
  it("is null when the app gives no length", () => {
    assert.equal(songProgress({ ...song, duration_ms: 0 }, AT_MS), null);
    assert.equal(songProgress({ ...song, duration_ms: -1 }, AT_MS), null);
    assert.equal(songProgress({ ...song, duration_ms: Number.NaN }, AT_MS), null);
  });

  it("moves on with time from when the position was read", () => {
    assert.equal(songProgress(song, AT_MS), 60_000 / 180_000);
    assert.equal(songProgress(song, AT_MS + 30_000), 90_000 / 180_000);
    assert.equal(songProgress(song, AT_MS + 120_000), 1);
  });

  it("stops at 1 after the end of the song", () => {
    assert.equal(songProgress(song, AT_MS + 10 * 60_000), 1);
    assert.equal(songProgress({ ...song, position_ms: 200_000 }, AT_MS), 1);
  });

  it("never goes below 0 when now is before the read", () => {
    assert.equal(songProgress(song, AT_MS - 90_000), 60_000 / 180_000);
    assert.equal(songProgress({ ...song, position_ms: 0 }, AT_MS - 90_000), 0);
  });
});

/** A canvas that keeps its listeners and fires them like the browser does. */
function fakeCanvas() {
  const listeners = new Set<(event: Event) => void>();
  const canvas = {
    addEventListener: (_type: string, listener: (event: Event) => void) => void listeners.add(listener),
    removeEventListener: (_type: string, listener: (event: Event) => void) => void listeners.delete(listener),
  };
  let prevented = 0;
  return {
    element: canvas as unknown as HTMLCanvasElement,
    listeners,
    lose() {
      const event = { preventDefault: () => void prevented++ } as unknown as Event;
      for (const listener of [...listeners]) listener(event);
    },
    get prevented() {
      return prevented;
    },
  };
}

const ctx = {} as Ctx;
const root = {} as HTMLElement;
const view = { mode: "idle" } as ViewState;

/** A background that makes a new canvas on every start, as a WebGL background does. */
function fakeBackground() {
  const log: string[] = [];
  const canvases: ReturnType<typeof fakeCanvas>[] = [];
  const updates: ViewState[] = [];
  let onStop: (() => void) | undefined;
  const api: BackgroundExt = {
    start() {
      log.push("start");
      canvases.push(fakeCanvas());
    },
    update(state) {
      log.push("update");
      updates.push(state);
    },
    stop() {
      log.push("stop");
      onStop?.();
    },
  };
  return {
    api,
    log,
    canvases,
    updates,
    current: () => canvases[canvases.length - 1],
    onStop: (callback: () => void) => (onStop = callback),
  };
}

describe("recoverWebGL", () => {
  it("restarts once after one loss and replays the last state", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    wrapped.update?.(view);
    const first = background.current();
    first.lose();
    assert.equal(first.prevented, 1);
    assert.deepEqual(background.log, ["start", "update"], "waits before it restarts");

    t.mock.timers.tick(399);
    assert.deepEqual(background.log, ["start", "update"]);
    t.mock.timers.tick(1);
    assert.deepEqual(background.log, ["start", "update", "stop", "start", "update"]);
    assert.deepEqual(background.updates, [view, view]);
    assert.equal(first.listeners.size, 0, "the old canvas lost its listener");
    assert.equal(background.current().listeners.size, 1, "the new canvas has one");
  });

  it("restarts without an update when there was no state yet", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    background.current().lose();
    t.mock.timers.tick(400);
    assert.deepEqual(background.log, ["start", "stop", "start"]);
  });

  it("restarts once after two quick losses", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    wrapped.update?.(view);
    const first = background.current();
    first.lose();
    t.mock.timers.tick(100);
    first.lose();
    assert.equal(first.prevented, 2, "both losses are prevented");
    t.mock.timers.tick(1000);
    assert.equal(background.log.filter((entry) => entry === "start").length, 2, "one restart");
    assert.equal(background.log.filter((entry) => entry === "stop").length, 1);
  });

  it("can recover again after a restart", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    background.current().lose();
    t.mock.timers.tick(400);
    background.current().lose();
    t.mock.timers.tick(400);
    assert.equal(background.log.filter((entry) => entry === "start").length, 3);
  });

  it("does not restart when the context is lost during stop", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    const canvas = background.current();
    // A background frees the GPU by losing the context in its own stop().
    background.onStop(() => canvas.lose());
    wrapped.stop();
    t.mock.timers.tick(5000);
    assert.deepEqual(background.log, ["start", "stop"]);
    assert.equal(canvas.listeners.size, 0);
    assert.equal(canvas.prevented, 0, "the listener was off before api.stop ran");
  });

  it("stop cancels a restart that is waiting", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    background.current().lose();
    wrapped.stop();
    t.mock.timers.tick(5000);
    assert.deepEqual(background.log, ["start", "stop"]);
  });

  it("works when the background has no update", (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const background = fakeBackground();
    delete background.api.update;
    const wrapped = recoverWebGL(background.api, () => background.current()?.element);
    wrapped.start(root, ctx);
    wrapped.update?.(view);
    background.current().lose();
    t.mock.timers.tick(400);
    assert.deepEqual(background.log, ["start", "stop", "start"]);
  });

  it("copes with a background that has no canvas", () => {
    const background = fakeBackground();
    const wrapped = recoverWebGL(background.api, () => undefined);
    wrapped.start(root, ctx);
    wrapped.stop();
    assert.deepEqual(background.log, ["start", "stop"]);
  });
});
