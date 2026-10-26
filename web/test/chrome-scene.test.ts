import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Card, ViewState } from "../src/api.ts";
import { GLITCH_MS, MAX_SIGNS, easeToward, flashAmount, glitchAmount, pinkTarget, renderSize, signsFrom, type Sign } from "../src/builtins/chrome-scene/logic.ts";

const NOW_MS = 10_000_000_000;

const card = (overrides: Partial<Card>): Card => ({
  source: "claude",
  session: "s1",
  kind: "running",
  started_at_ms: NOW_MS - 60_000,
  updated_at_ms: NOW_MS - 1000,
  ...overrides,
});

const view = (cards: Card[], overrides: Partial<ViewState> = {}): ViewState => ({
  mode: "idle",
  cards,
  agents: {
    claude: { name: "Claude Code", mark_url: "file:///claude.svg" },
    codex: { name: "Codex", mark_url: "file:///codex.svg" },
  },
  agentOrder: ["claude", "codex"],
  limits: [],
  nowMs: NOW_MS,
  serviceOk: true,
  display: "main",
  ...overrides,
});

describe("signsFrom", () => {
  it("one sign per card with the agent's first letter", () => {
    const signs = signsFrom(
      view([
        card({ source: "claude", session: "a", updated_at_ms: NOW_MS - 1000 }),
        card({ source: "codex", session: "b", updated_at_ms: NOW_MS - 2000 }),
        card({ source: "aider", session: "c", updated_at_ms: NOW_MS - 3000 }),
      ]),
      [],
      NOW_MS,
    );
    assert.deepEqual(
      signs.map((sign) => [sign.source, sign.letter, sign.state]),
      [
        ["claude", "C", "running"],
        ["codex", "C", "running"],
        // No agent extension: the letter comes from the source.
        ["aider", "A", "running"],
      ],
    );
  });

  it("newest card comes first", () => {
    const signs = signsFrom(
      view([card({ session: "old", updated_at_ms: NOW_MS - 9000 }), card({ source: "codex", session: "new", updated_at_ms: NOW_MS - 10 })]),
      [],
      NOW_MS,
    );
    assert.deepEqual(signs.map((sign) => sign.session), ["new", "old"]);
  });

  it("approval card gives an approval sign", () => {
    const [sign] = signsFrom(view([card({ kind: "approval" })]), [], NOW_MS);
    assert.equal(sign.state, "approval");
  });

  it("cards with no work give idle signs", () => {
    const signs = signsFrom(view([card({ session: "a", kind: "unknown" }), card({ session: "b", kind: "no_activity" })]), [], NOW_MS);
    assert.deepEqual(signs.map((sign) => sign.state), ["idle", "idle"]);
  });

  it("card that turns finished gets a flash start, an old finished card does not", () => {
    const previous = signsFrom(view([card({ session: "a", kind: "running" }), card({ session: "b", kind: "finished" })]), [], NOW_MS - 500);
    assert.equal(previous[1].flashStartMs, undefined, "a card already finished when first seen does not flash");
    const signs = signsFrom(view([card({ session: "a", kind: "finished" }), card({ session: "b", kind: "finished" })]), previous, NOW_MS);
    const bySession = new Map(signs.map((sign) => [sign.session, sign]));
    assert.equal(bySession.get("a")?.flashStartMs, NOW_MS);
    assert.equal(bySession.get("b")?.flashStartMs, undefined);
  });

  it("a flash keeps its start while the card stays finished", () => {
    const previous: Sign[] = [{ source: "claude", session: "a", letter: "C", state: "finished", flashStartMs: NOW_MS - 300 }];
    const [sign] = signsFrom(view([card({ session: "a", kind: "finished" })]), previous, NOW_MS);
    assert.equal(sign.flashStartMs, NOW_MS - 300);
  });

  it("eight cards give six signs", () => {
    const cards = Array.from({ length: 8 }, (_, index) => card({ session: `s${index}`, updated_at_ms: NOW_MS - index }));
    const signs = signsFrom(view(cards), [], NOW_MS);
    assert.equal(MAX_SIGNS, 6);
    assert.equal(signs.length, MAX_SIGNS);
    assert.deepEqual(signs.map((sign) => sign.session), ["s0", "s1", "s2", "s3", "s4", "s5"]);
  });

  it("secondary display gives no signs", () => {
    assert.deepEqual(signsFrom(view([card({})], { display: "secondary", mode: "secondary" }), [], NOW_MS), []);
  });
});

describe("flashAmount", () => {
  it("is full at the start, gone after the flash, zero without one", () => {
    const sign: Sign = { source: "claude", session: "a", letter: "C", state: "finished", flashStartMs: NOW_MS };
    assert.equal(flashAmount(sign, NOW_MS), 1);
    assert.equal(flashAmount(sign, NOW_MS + 60_000), 0);
    assert.ok(flashAmount(sign, NOW_MS + 300) < 1);
    assert.equal(flashAmount({ ...sign, flashStartMs: undefined }, NOW_MS), 0);
  });
});

describe("pinkTarget", () => {
  it("pink target is 1 with an open approval and 0 without", () => {
    assert.equal(pinkTarget(view([card({ session: "a" }), card({ session: "b", kind: "approval" })])), 1);
    assert.equal(pinkTarget(view([card({ kind: "running" }), card({ session: "b", kind: "finished" })])), 0);
    assert.equal(pinkTarget(view([])), 0);
  });
});

describe("glitchAmount", () => {
  it("bursts at the start and is gone after GLITCH_MS or with no start", () => {
    assert.equal(glitchAmount(0), 1);
    assert.equal(glitchAmount(GLITCH_MS), 0);
    assert.equal(glitchAmount(undefined), 0);
    assert.equal(glitchAmount(-5), 0);
  });
});

describe("easeToward", () => {
  it("gets closer at any frame rate and the same distance for the same time", () => {
    const oneStep = easeToward(0, 1, 100, 500);
    const twoSteps = easeToward(easeToward(0, 1, 50, 500), 1, 50, 500);
    assert.ok(oneStep > 0 && oneStep < 1);
    assert.ok(Math.abs(oneStep - twoSteps) < 1e-9);
  });
});

describe("renderSize", () => {
  it("render size is half the screen and never below 320 wide", () => {
    assert.deepEqual(renderSize(2560, 1440, 0.5), { width: 1280, height: 720 });
    assert.deepEqual(renderSize(2560, 1440, 0.25), { width: 640, height: 360 });
    // More than half is not allowed.
    assert.deepEqual(renderSize(2560, 1440, 1), { width: 1280, height: 720 });
    // A small screen keeps 320 wide and its shape.
    assert.deepEqual(renderSize(800, 500, 0.25), { width: 320, height: 200 });
  });

  it("never asks for more than the screen or for nothing", () => {
    assert.deepEqual(renderSize(200, 100, 0.5), { width: 200, height: 100 });
    assert.deepEqual(renderSize(0, 0, 0.5), { width: 1, height: 1 });
  });
});

// The scene's entry file registers itself on import, so the failure paths run
// it against a fake page: a canvas, its listeners and a frame loop to count.
describe("chrome scene failure paths", () => {
  type Listener = (event: { preventDefault(): void }) => void;

  class FakeElement {
    readonly tag: string;
    readonly children: FakeElement[] = [];
    readonly listeners = new Map<string, Listener>();
    readonly style: Record<string, string> & { cssText: string } = { cssText: "" };
    parent: FakeElement | undefined;
    width = 0;
    height = 0;
    clientWidth = 1440;
    clientHeight = 900;
    gl: unknown = null;
    constructor(tag: string) {
      this.tag = tag;
    }
    getContext(kind: string): unknown {
      return kind === "webgl2" ? this.gl : null;
    }
    appendChild(child: FakeElement): FakeElement {
      child.parent = this;
      this.children.push(child);
      return child;
    }
    remove(): void {
      if (this.parent === undefined) return;
      this.parent.children.splice(this.parent.children.indexOf(this), 1);
      this.parent = undefined;
    }
    addEventListener(type: string, listener: Listener): void {
      this.listeners.set(type, listener);
    }
    removeEventListener(type: string): void {
      this.listeners.delete(type);
    }
    fire(type: string): void {
      this.listeners.get(type)?.({ preventDefault() {} });
    }
  }

  /** A WebGL2 context where every call works and returns something truthy. */
  const fakeGl = (): unknown =>
    new Proxy({}, { get: (_target, prop) => (prop === "then" ? undefined : () => ({})) });

  const globals = globalThis as unknown as Record<string, unknown>;
  let nextGl: () => unknown = () => null;
  const canvases: FakeElement[] = [];
  let register: (() => import("../src/api.ts").BackgroundExt) | undefined;

  async function loadScene(): Promise<() => import("../src/api.ts").BackgroundExt> {
    if (register !== undefined) return register;
    globals.window = {
      agentScreensaver: { register: (create: () => import("../src/api.ts").BackgroundExt) => (register = create) },
      matchMedia: () => ({ matches: false }),
      addEventListener() {},
      removeEventListener() {},
    };
    globals.document = {
      currentScript: { src: "file:///ext/builtin.chrome-scene/index.js" },
      createElement(tag: string) {
        const element = new FakeElement(tag);
        if (tag === "canvas") {
          element.gl = nextGl();
          canvases.push(element);
        }
        return element;
      },
    };
    await import("../src/builtins/chrome-scene/index.ts");
    assert.ok(register, "the entry file registers a create function");
    return register;
  }

  function fakeCtx() {
    const loop = { started: 0, cancelled: 0 };
    const ctx = {
      options: {},
      display: "main" as const,
      now: () => NOW_MS,
      requestFrame: () => {
        loop.started++;
        return () => void loop.cancelled++;
      },
    };
    return { ctx, loop };
  }

  it("no WebGL2 falls back to black with CSS grain and does not throw", async () => {
    nextGl = () => null;
    const create = await loadScene();
    const root = new FakeElement("div");
    const { ctx, loop } = fakeCtx();
    const scene = create();
    assert.doesNotThrow(() => scene.start(root as unknown as HTMLElement, ctx));
    assert.doesNotThrow(() => scene.update?.(view([card({ kind: "approval" })])));
    assert.equal(loop.started, 0, "no frame loop without WebGL2");
    assert.equal(root.children.length, 1);
    assert.equal(root.children[0].tag, "div");
    assert.match(root.children[0].style.cssText, /background/);
    scene.stop();
    assert.equal(root.children.length, 0);
  });

  it("context lost stops the loop and falls back", async () => {
    nextGl = fakeGl;
    const create = await loadScene();
    const root = new FakeElement("div");
    const { ctx, loop } = fakeCtx();
    const scene = create();
    scene.start(root as unknown as HTMLElement, ctx);
    scene.update?.(view([card({})]));
    assert.equal(loop.started, 1);
    const canvas = root.children.find((child) => child.tag === "canvas");
    assert.ok(canvas, "the scene draws on a canvas");
    canvas.fire("webglcontextlost");
    assert.equal(loop.cancelled, 1);
    assert.deepEqual(root.children.map((child) => child.tag), ["div"]);
    assert.doesNotThrow(() => scene.update?.(view([card({ kind: "finished" })])));
    scene.stop();
    assert.equal(root.children.length, 0);
    assert.equal(loop.cancelled, 1, "stop does not cancel twice");
  });

  it("update before start does not throw", async () => {
    const create = await loadScene();
    const scene = create();
    assert.doesNotThrow(() => scene.update?.(view([card({})])));
    assert.doesNotThrow(() => scene.stop());
  });
});
