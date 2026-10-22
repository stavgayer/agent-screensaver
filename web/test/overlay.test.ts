import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { LOCK_UI_CLASS, setLockUI } from "../src/overlay.ts";

function fakeRoot() {
  const classes = new Set<string>();
  const classList = {
    toggle(name: string, force?: boolean) {
      const on = force ?? !classes.has(name);
      if (on) classes.add(name);
      else classes.delete(name);
      return on;
    },
  } as unknown as DOMTokenList;
  return { root: { classList }, classes };
}

describe("setLockUI", () => {
  it("hides_components_while_lock_ui_shows", () => {
    const { root, classes } = fakeRoot();
    setLockUI(root, true);
    assert.ok(classes.has(LOCK_UI_CLASS));
  });
  it("shows_components_again_when_lock_ui_goes_away", () => {
    const { root, classes } = fakeRoot();
    setLockUI(root, true);
    setLockUI(root, false);
    assert.ok(!classes.has(LOCK_UI_CLASS));
  });
  it("repeated_calls_keep_the_same_state", () => {
    const { root, classes } = fakeRoot();
    setLockUI(root, true);
    setLockUI(root, true);
    assert.ok(classes.has(LOCK_UI_CLASS));
  });
  it("non_boolean_input_shows_components", () => {
    for (const bad of [undefined, null, "true", 1, {}]) {
      const { root, classes } = fakeRoot();
      setLockUI(root, true);
      setLockUI(root, bad);
      assert.ok(!classes.has(LOCK_UI_CLASS), `input ${String(bad)}`);
    }
  });
});
