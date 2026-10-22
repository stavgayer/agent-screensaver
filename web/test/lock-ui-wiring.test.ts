import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { LOCK_UI_CLASS } from "../src/overlay.ts";

/** Loads the real runtime with just enough of a page for its top-level code. */
async function loadRuntime() {
  const classes = new Set<string>();
  const documentElement = {
    classList: {
      toggle(name: string, force?: boolean) {
        if (force) classes.add(name);
        else classes.delete(name);
        return classes.has(name);
      },
    },
  };
  const globals = globalThis as Record<string, unknown>;
  const realSetInterval = globals.setInterval;
  globals.window = globals;
  globals.document = { documentElement };
  globals.setInterval = () => 0; // the service-line check must not keep the test alive
  try {
    await import("../src/runtime.ts");
  } finally {
    globals.setInterval = realSetInterval;
  }
  const api = (globals.window as { agentScreensaver: { setLockUI(shown: unknown): void } }).agentScreensaver;
  return { api, classes };
}

describe("lock screen wiring", () => {
  it("runtime_exports_setLockUI_and_it_toggles_the_class", async () => {
    const { api, classes } = await loadRuntime();
    assert.equal(typeof api.setLockUI, "function");
    api.setLockUI(true);
    assert.ok(classes.has(LOCK_UI_CLASS));
    api.setLockUI(false);
    assert.ok(!classes.has(LOCK_UI_CLASS));
  });

  it("css_hides_screen_and_service_line_under_the_class", () => {
    const css = readFileSync(new URL("../styles.css", import.meta.url), "utf8");
    const rule = css.split("\n").find((line) => line.includes(`.${LOCK_UI_CLASS} `) && line.includes("opacity: 0"));
    assert.ok(rule, `no rule for .${LOCK_UI_CLASS} with opacity 0`);
    assert.match(rule, new RegExp(`\\.${LOCK_UI_CLASS} #screen\\b`));
    assert.match(rule, new RegExp(`\\.${LOCK_UI_CLASS} #service-line\\b`));
    assert.doesNotMatch(rule, /#background/);
  });

  it("page_has_the_elements_the_rule_targets", () => {
    const page = readFileSync(new URL("../index.html", import.meta.url), "utf8");
    assert.match(page, /id="screen"/);
    assert.match(page, /id="service-line"/);
  });
});
