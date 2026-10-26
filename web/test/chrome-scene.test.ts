import assert from "node:assert/strict";
import { describe, it } from "node:test";
import type { Card, ViewState } from "../src/api.ts";
import { GLITCH_MS, MAX_SIGNS, ROW_Y_APPROVAL, ROW_Y_CALM, easeToward, flashAmount, glitchAmount, labelFor, layoutRow, pinkTarget, renderSize, rowY, signsFrom, type Sign } from "../src/builtins/chrome-scene/logic.ts";

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
  it("one sign per card with the agent's name", () => {
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
      signs.map((sign) => [sign.source, sign.label, sign.state]),
      [
        ["claude", "CLAUDE", "running"],
        ["codex", "CODEX", "running"],
        // No agent extension: the name comes from the source.
        ["aider", "AIDER", "running"],
      ],
    );
  });

  it("signs keep a stable order: agent order, then start time, then session", () => {
    const cards = [
      card({ source: "aider", session: "x", started_at_ms: NOW_MS - 90_000 }),
      card({ source: "codex", session: "b", started_at_ms: NOW_MS - 50_000 }),
      card({ source: "claude", session: "late", started_at_ms: NOW_MS - 10_000 }),
      card({ source: "claude", session: "early", started_at_ms: NOW_MS - 80_000 }),
      card({ source: "claude", session: "a", started_at_ms: NOW_MS - 80_000 }),
    ];
    const order = (state: ViewState): string[] => signsFrom(state, [], NOW_MS).map((sign) => sign.session);
    assert.deepEqual(order(view(cards)), ["a", "early", "late", "b", "x"]);
    // A card that updates does not move.
    const updated = cards.map((one) => (one.session === "late" ? { ...one, updated_at_ms: NOW_MS } : { ...one, updated_at_ms: NOW_MS - 50_000 }));
    assert.deepEqual(order(view(updated)), ["a", "early", "late", "b", "x"]);
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
    const previous: Sign[] = [{ source: "claude", session: "a", label: "CLAUDE", state: "finished", flashStartMs: NOW_MS - 300 }];
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

describe("rowY", () => {
  it("the row sits higher while calm and drops below the approval card while one is open", () => {
    assert.equal(rowY(0), ROW_Y_CALM);
    assert.equal(rowY(1), ROW_Y_APPROVAL);
    assert.ok(ROW_Y_APPROVAL < ROW_Y_CALM, "an open approval moves the row down");
    assert.equal(rowY(0.5), (ROW_Y_CALM + ROW_Y_APPROVAL) / 2);
  });

  it("an amount outside 0..1 is kept inside", () => {
    assert.equal(rowY(-1), ROW_Y_CALM);
    assert.equal(rowY(2), ROW_Y_APPROVAL);
  });
});

describe("labelFor", () => {
  const cases: Array<[string, string, string]> = [
    ["a short name is uppercased", "Codex", "CODEX"],
    ["a long name keeps its first word", "Claude Code", "CLAUDE"],
    ["a long single word is cut at eight letters", "Supercalifragilistic", "SUPERCAL"],
    ["a long first word is cut too", "Antigravity Agent", "ANTIGRAV"],
    ["spaces around are dropped", "  Cursor ", "CURSOR"],
    ["an empty name gives a question mark", "  ", "?"],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => assert.equal(labelFor(input), want));
  }
});

describe("layoutRow", () => {
  it("centres the signs with even gaps", () => {
    const { centers, scale } = layoutRow([1, 0.5, 1], 0.2, 100);
    assert.equal(scale, 1);
    // Widths 2 + 1 + 2 and two gaps of 0.2: 5.4 wide, from -2.7 to 2.7.
    assert.deepEqual(centers.map((x) => Math.round(x * 1000) / 1000), [-1.7, 0, 1.7]);
  });

  it("shrinks a row that is too wide to fit", () => {
    const { centers, scale } = layoutRow([1, 1], 0, 2);
    assert.equal(scale, 0.5);
    assert.deepEqual(centers, [-0.5, 0.5]);
  });

  it("no signs, no row", () => {
    assert.deepEqual(layoutRow([], 0.2, 5), { centers: [], scale: 1 });
  });
});

describe("flashAmount", () => {
  it("is full at the start, gone after the flash, zero without one", () => {
    const sign: Sign = { source: "claude", session: "a", label: "CLAUDE", state: "finished", flashStartMs: NOW_MS };
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

  it("a secondary display stays calm", () => {
    assert.equal(pinkTarget(view([card({ kind: "approval" })], { display: "secondary", mode: "secondary" })), 0);
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
// it against a fake page: canvases, their listeners, a counting WebGL2 context
// and a frame loop to count.
describe("chrome scene on a fake page", () => {
  type Listener = (event: { preventDefault(): void }) => void;
  type Ext = import("../src/api.ts").BackgroundExt;

  /** A 2D context with just what the scene uses. */
  const fake2d = () => ({
    createImageData: (width: number, height: number) => ({ data: new Uint8ClampedArray(width * height * 4) }),
    putImageData() {},
    clearRect() {},
    fillText() {},
    measureText: (text: string) => ({ width: text.length * 50 }),
  });

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
      if (kind === "webgl2") return this.gl;
      return kind === "2d" ? fake2d() : null;
    }
    toDataURL(): string {
      return "data:image/png;base64,GRAIN";
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

  /** A WebGL2 context where every call works and is counted. */
  function countingGl(options: { compiles?: boolean; links?: boolean } = {}) {
    const calls = new Map<string, number>();
    const count = (name: string): void => void calls.set(name, (calls.get(name) ?? 0) + 1);
    const special: Record<string, (...args: unknown[]) => unknown> = {
      getShaderParameter: () => options.compiles ?? true,
      getProgramParameter: () => options.links ?? true,
      getShaderInfoLog: () => "fake compile error",
      getProgramInfoLog: () => "fake link error",
      getExtension: (name) => (name === "WEBGL_lose_context" ? { loseContext: () => count("loseContext") } : null),
    };
    const gl = new Proxy(
      {},
      {
        get: (_target, prop) => {
          if (typeof prop !== "string" || prop === "then") return undefined;
          return (...args: unknown[]) => {
            count(prop);
            return special[prop] === undefined ? {} : special[prop](...args);
          };
        },
      },
    );
    return { gl, calls };
  }

  const globals = globalThis as unknown as Record<string, unknown>;
  let nextGl: () => unknown = () => null;
  const windowListeners = new Map<string, unknown>();
  let register: (() => Ext) | undefined;

  async function loadScene(): Promise<() => Ext> {
    if (register !== undefined) return register;
    globals.window = {
      agentScreensaver: { register: (create: () => Ext) => (register = create) },
      matchMedia: () => ({ matches: false }),
      addEventListener: (type: string, listener: unknown) => windowListeners.set(type, listener),
      removeEventListener: (type: string) => windowListeners.delete(type),
    };
    globals.document = {
      currentScript: { src: "file:///ext/builtin.chrome-scene/index.js" },
      createElement(tag: string) {
        const element = new FakeElement(tag);
        if (tag === "canvas") element.gl = nextGl();
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

  /** Runs `fn` with console.error silenced, for paths that log on purpose. */
  function quietly(fn: () => void): void {
    const original = console.error;
    console.error = () => {};
    try {
      fn();
    } finally {
      console.error = original;
    }
  }

  async function started(gl: () => unknown) {
    nextGl = gl;
    const create = await loadScene();
    const root = new FakeElement("div");
    const { ctx, loop } = fakeCtx();
    const scene = create();
    quietly(() => scene.start(root as unknown as HTMLElement, ctx));
    return { root, loop, scene };
  }

  function assertFallback(root: FakeElement): void {
    assert.deepEqual(root.children.map((child) => child.tag), ["div"]);
    const css = root.children[0].style.cssText;
    assert.match(css, /#000/);
    assert.match(css, /repeating-linear-gradient/);
    assert.match(css, /url\(data:image\/png;base64,GRAIN\)/);
  }

  it("no WebGL2 falls back to black with CSS grain and does not throw", async () => {
    const { root, loop, scene } = await started(() => null);
    assert.doesNotThrow(() => scene.update?.(view([card({ kind: "approval" })])));
    assert.equal(loop.started, 0, "no frame loop without WebGL2");
    assertFallback(root);
    scene.stop();
    assert.equal(root.children.length, 0);
  });

  it("a normal start draws on a canvas, and stop releases everything", async () => {
    const { gl, calls } = countingGl();
    const { root, loop, scene } = await started(() => gl);
    scene.update?.(view([card({})]));
    assert.equal(loop.started, 1);
    assert.deepEqual(root.children.map((child) => child.tag), ["canvas"]);
    assert.ok(windowListeners.has("resize"));
    scene.stop();
    assert.equal(loop.cancelled, 1);
    assert.equal(calls.get("deleteProgram"), 1);
    assert.equal(calls.get("deleteTexture"), 1);
    assert.equal(calls.get("loseContext"), 1);
    assert.equal(root.children.length, 0);
    assert.ok(!windowListeners.has("resize"));
  });

  it("a shader that does not compile falls back", async () => {
    const { gl } = countingGl({ compiles: false });
    const { root, loop, scene } = await started(() => gl);
    assert.equal(loop.started, 0);
    assertFallback(root);
    scene.stop();
  });

  it("a program that does not link falls back", async () => {
    const { gl } = countingGl({ links: false });
    const { root, loop, scene } = await started(() => gl);
    assert.equal(loop.started, 0);
    assertFallback(root);
    scene.stop();
  });

  it("context lost stops the loop and falls back", async () => {
    const { gl } = countingGl();
    const { root, loop, scene } = await started(() => gl);
    scene.update?.(view([card({})]));
    const canvas = root.children.find((child) => child.tag === "canvas");
    assert.ok(canvas, "the scene draws on a canvas");
    canvas.fire("webglcontextlost");
    assert.equal(loop.cancelled, 1);
    assert.ok(!windowListeners.has("resize"), "the resize listener goes with the canvas");
    assertFallback(root);
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
