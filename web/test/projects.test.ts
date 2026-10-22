import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { projectName, withProject, withoutProject } from "../settings/projects.ts";

describe("projectName", () => {
  const cases: Array<[string, string]> = [
    ["observer-sessions", "observer-sessions"],
    ["  scratch  ", "scratch"],
    ["/Users/me/tmp/scratch/", "scratch"],
    ["~/work/api", "api"],
    ["", ""],
    ["   ", ""],
    ["/", ""],
    [".", ""],
  ];
  for (const [entry, want] of cases) {
    it(`${JSON.stringify(entry)} is ${JSON.stringify(want)}`, () => assert.equal(projectName(entry), want));
  }
});

describe("withProject", () => {
  it("adds the folder name at the end", () => {
    assert.deepEqual(withProject(["observer-sessions"], "/Users/me/scratch"), { projects: ["observer-sessions", "scratch"] });
  });
  it("refuses a name already in the list", () => {
    assert.deepEqual(withProject(["scratch"], " scratch/ "), { error: "scratch is already in the list." });
  });
  it("refuses a blank entry", () => {
    assert.deepEqual(withProject([], "  "), { error: "Type a folder name, for example observer-sessions." });
  });
});

describe("withoutProject", () => {
  it("removes only that name and keeps the order", () => {
    assert.deepEqual(withoutProject(["a", "b", "c"], "b"), ["a", "c"]);
  });
  it("leaves the list alone when the name is not there", () => {
    assert.deepEqual(withoutProject(["a"], "z"), ["a"]);
  });
});
