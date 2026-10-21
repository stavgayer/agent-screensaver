import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { dimOption, fileUrl, fitOption } from "../src/builtins/shared/media.ts";

describe("fileUrl", () => {
  const cases: [string, unknown, string | undefined][] = [
    ["a plain path", "/Users/me/pic.jpg", "file:///Users/me/pic.jpg"],
    [
      "spaces in Application Support",
      "/Users/me/Library/Application Support/agent-screensaver/media/builtin.image/file/my photo.png",
      "file:///Users/me/Library/Application%20Support/agent-screensaver/media/builtin.image/file/my%20photo.png",
    ],
    ["characters that would end the path", "/tmp/a#b?c%d.mov", "file:///tmp/a%23b%3Fc%25d.mov"],
    ["non-ASCII names", "/tmp/תמונה.jpg", "file:///tmp/%D7%AA%D7%9E%D7%95%D7%A0%D7%94.jpg"],
    ["an empty path", "", undefined],
    ["a relative path", "media/pic.jpg", undefined],
    ["a value that is not text", 42, undefined],
    ["nothing", undefined, undefined],
  ];
  for (const [name, input, want] of cases) {
    it(name, () => assert.equal(fileUrl(input), want));
  }
});

describe("fitOption", () => {
  const cases: [unknown, string][] = [
    ["cover", "cover"],
    ["contain", "contain"],
    ["fill", "cover"],
    [undefined, "cover"],
    [3, "cover"],
  ];
  for (const [input, want] of cases) {
    it(`${String(input)} gives ${want}`, () => assert.equal(fitOption(input), want));
  }
});

describe("dimOption", () => {
  const cases: [unknown, number][] = [
    [undefined, 0.25],
    ["dark", 0.25],
    [0, 0],
    [0.5, 0.5],
    [0.8, 0.8],
    [1, 0.8],
    [-1, 0],
  ];
  for (const [input, want] of cases) {
    it(`${String(input)} gives ${want}`, () => assert.equal(dimOption(input), want));
  }
});
