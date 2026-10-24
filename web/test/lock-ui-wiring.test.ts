import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { LOCK_UI_CLASS } from "../src/overlay.ts";

/** Loads the real runtime with just enough of a page for its top-level code. */
async function loadRuntime() {
  const intervalDelays: number[] = [];
  const timeouts: { callback: () => void; delayMs: number }[] = [];
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
  const realSetTimeout = globals.setTimeout;
  globals.window = globals;
  globals.document = { documentElement };
  globals.setInterval = (_callback: unknown, delayMs: number) => {
    intervalDelays.push(delayMs); // the timers must not keep the test alive
    return 0;
  };
  globals.setTimeout = (callback: () => void, delayMs: number) => {
    timeouts.push({ callback, delayMs }); // kept, not run, so the test controls time
    return 0;
  };
  try {
    await import("../src/runtime.ts");
  } finally {
    globals.setInterval = realSetInterval;
    globals.setTimeout = realSetTimeout;
  }
  const api = (globals.window as { agentScreensaver: { setLockUI(shown: unknown): void } }).agentScreensaver;
  return { api, classes, intervalDelays, timeouts };
}

describe("minute tick", () => {
  it("runtime_updates_components_on_each_minute_for_countdowns", async () => {
    const { timeouts } = await loadRuntime();
    assert.equal(timeouts.length, 1, "one minute timer");
    assert.ok(timeouts[0].delayMs > 0 && timeouts[0].delayMs <= 61_000, `delay ${timeouts[0].delayMs}`);
    // Firing it runs the tick (with no state yet it only waits) and sets the next one.
    const realSetTimeout = globalThis.setTimeout;
    const next: number[] = [];
    (globalThis as Record<string, unknown>).setTimeout = (_callback: unknown, delayMs: number) => next.push(delayMs);
    try {
      timeouts[0].callback();
    } finally {
      globalThis.setTimeout = realSetTimeout;
    }
    assert.equal(next.length, 1, "the next minute timer");
  });
});

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
