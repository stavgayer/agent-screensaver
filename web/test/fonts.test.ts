import assert from "node:assert/strict";
import { afterEach, beforeEach, describe, it } from "node:test";
import { loadFont } from "../src/builtins/shared/fonts.ts";

const globals = globalThis as unknown as Record<string, unknown>;

class FakeFontFace {
  static made: FakeFontFace[] = [];
  static fail = false;
  readonly family: string;
  readonly source: string;
  readonly descriptors?: object;
  constructor(family: string, source: string, descriptors?: object) {
    this.family = family;
    this.source = source;
    this.descriptors = descriptors;
    FakeFontFace.made.push(this);
  }
  async load(): Promise<this> {
    if (FakeFontFace.fail) throw new Error("network error");
    return this;
  }
}

describe("loadFont", () => {
  let added: FakeFontFace[];

  beforeEach(() => {
    FakeFontFace.made = [];
    FakeFontFace.fail = false;
    added = [];
    globals.FontFace = FakeFontFace;
    globals.document = { fonts: { add: (face: FakeFontFace) => added.push(face) } };
  });

  afterEach(() => {
    delete globals.FontFace;
    delete globals.document;
  });

  it("loads a family once even when asked twice", async () => {
    const first = await loadFont("Once", "file:///a/once.woff2");
    const second = await loadFont("Once", "file:///a/once.woff2");
    assert.equal(first, true);
    assert.equal(second, true);
    assert.equal(FakeFontFace.made.length, 1);
    assert.equal(added.length, 1);
    assert.equal(FakeFontFace.made[0].source, "url(file:///a/once.woff2)");
  });

  it("passes the descriptors to the font face", async () => {
    await loadFont("Weighted", "file:///a/w.woff2", { weight: "100 800" });
    assert.deepEqual(FakeFontFace.made[0].descriptors, { weight: "100 800" });
  });

  it("resolves false when the font fails to load", async () => {
    FakeFontFace.fail = true;
    assert.equal(await loadFont("Broken", "file:///a/broken.woff2"), false);
    assert.equal(added.length, 0);
  });

  it("resolves false when FontFace is missing", async () => {
    delete globals.FontFace;
    assert.equal(await loadFont("NoApi", "file:///a/noapi.woff2"), false);
  });
});
