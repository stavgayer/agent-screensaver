import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { STYLE } from "../src/builtins/task-row/style.ts";

/** The declarations of the first plain rule for a selector, as a map. */
function rule(selector: string): Map<string, string> {
  const start = STYLE.indexOf(`\n${selector} {`);
  assert.notEqual(start, -1, `no rule for ${selector}`);
  const body = STYLE.slice(STYLE.indexOf("{", start) + 1, STYLE.indexOf("}", start));
  return new Map(body.split(";").map((part) => part.split(":").map((text) => text.trim())).filter((pair) => pair.length === 2) as Array<[string, string]>);
}

const shrink = (selector: string): number => Number(rule(selector).get("flex")?.split(/\s+/)[1]);

/** Every selector a container query hides. Each query is one line of the style. */
function hiddenWhenNarrow(): string[] {
  return STYLE.split("\n")
    .filter((line) => line.startsWith("@container task"))
    .flatMap((line) => [...line.slice(line.indexOf("{") + 1).matchAll(/([^{}]+)\{ display: none; \}/g)])
    .flatMap((hide) => hide[1].split(",").map((selector) => selector.trim()));
}

describe("task row style", () => {
  it("the last line gives way branch first, then project, then model", () => {
    const branch = shrink(".task .where > .branch");
    const project = shrink(".task .where > .project");
    const model = shrink(".task .where > .model");
    assert.ok(branch > project && project > model, `shrink: branch ${branch}, project ${project}, model ${model}`);
  });

  it("narrow cards never drop the model, the agent's mark, the state or the count", () => {
    const hidden = hiddenWhenNarrow();
    assert.ok(hidden.includes(".task .where > .branch"), `hidden: ${hidden.join(" | ")}`);
    for (const kept of [".model", ".agent", ".status", ".background", ".top"]) {
      assert.ok(!hidden.some((selector) => selector.endsWith(kept)), `${kept} is hidden on narrow cards: ${hidden.join(" | ")}`);
    }
  });

  it("the agent's mark is drawn at the state line's size, not as a small badge", () => {
    const top = rule(".task .top");
    const mark = rule(".task .agent");
    assert.equal(top.get("font-size"), "2.2vmin");
    assert.equal(mark.get("height"), "1.5em");
  });
});
