import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { formFor, type OptionSpec } from "../settings/form.ts";

describe("formFor", () => {
  it("number option becomes number field with range", () => {
    const options: Record<string, OptionSpec> = {
      fps: { type: "number", default: 24, min: 1, max: 240, label: "Frames per second" },
    };
    assert.deepEqual(formFor(options, {}), [
      { key: "fps", label: "Frames per second", control: "number", value: 24, min: 1, max: 240 },
    ]);
  });

  it("enum becomes select", () => {
    const options: Record<string, OptionSpec> = {
      charset: { type: "enum", default: "latin", values: ["katakana", "latin"], label: "Characters" },
    };
    assert.deepEqual(formFor(options, {}), [
      { key: "charset", label: "Characters", control: "select", value: "latin", choices: ["katakana", "latin"] },
    ]);
  });

  const simpleKinds: { kind: OptionSpec["type"]; control: string; value: unknown }[] = [
    { kind: "boolean", control: "toggle", value: true },
    { kind: "color", control: "color", value: "#00ff66" },
    { kind: "file", control: "file", value: "" },
    { kind: "string", control: "text", value: "hello" },
  ];
  for (const { kind, control, value } of simpleKinds) {
    it(`${kind} becomes ${control}`, () => {
      const [field] = formFor({ key: { type: kind, default: value, label: "Label" } }, {});
      assert.deepEqual(field, { key: "key", label: "Label", control, value });
    });
  }

  it("current value wins over default", () => {
    const [field] = formFor({ fps: { type: "number", default: 24 } }, { fps: 60 });
    assert.equal(field?.value, 60);
  });

  it("label falls back to key", () => {
    const [field] = formFor({ density: { type: "number", default: 1 } }, {});
    assert.equal(field?.label, "density");
  });

  it("keeps the order of the options", () => {
    const options: Record<string, OptionSpec> = {
      speed: { type: "number", default: 1 },
      color: { type: "color", default: "#000000" },
    };
    assert.deepEqual(formFor(options, {}).map((field) => field.key), ["speed", "color"]);
  });
});
