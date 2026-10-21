import { describe, it } from "node:test";
import assert from "node:assert/strict";
import type { CreateExtension } from "../src/api.ts";
import { createPendingLoads, type ScriptHandle } from "../src/pending.ts";

const create: CreateExtension = () => ({ start() {}, stop() {} });

/** A script and a clock that the test drives by hand. */
function harness() {
  const scripts: ScriptHandle[] = [];
  const timers: { cb: () => void; ms: number; live: boolean }[] = [];
  const loads = createPendingLoads({
    createScript(url) {
      const script: ScriptHandle = {
        src: `file:///x/${url}`,
        onload: null,
        onerror: null,
        attach() {},
      };
      scripts.push(script);
      return script;
    },
    setTimer(cb, ms) {
      const timer = { cb, ms, live: true };
      timers.push(timer);
      return timer;
    },
    clearTimer(handle) {
      (handle as { live: boolean }).live = false;
    },
  });
  return { loads, scripts, timers };
}

describe("createPendingLoads", () => {
  it("registers_by_current_script_src", async () => {
    const { loads, scripts } = harness();
    const first = loads.load("a.js", 100);
    const second = loads.load("b.js", 100);
    loads.register("file:///x/b.js", create);
    scripts[1]?.onload?.();
    assert.equal(await second, create);
    loads.register("file:///x/a.js", create);
    scripts[0]?.onload?.();
    assert.equal(await first, create);
  });

  it("unmatched_register_is_ignored", async () => {
    const { loads, scripts } = harness();
    const pending = loads.load("a.js", 100);
    loads.register("file:///x/other.js", create);
    loads.register(undefined, create);
    scripts[0]?.onload?.();
    assert.equal(await pending, undefined);
  });

  it("script_error_resolves_undefined", async () => {
    const { loads, scripts, timers } = harness();
    const pending = loads.load("a.js", 100);
    scripts[0]?.onerror?.();
    assert.equal(await pending, undefined);
    assert.equal(timers[0]?.live, false);
  });

  it("loaded_without_register_resolves_undefined", async () => {
    const { loads, scripts } = harness();
    const pending = loads.load("a.js", 100);
    scripts[0]?.onload?.();
    assert.equal(await pending, undefined);
  });

  it("times_out", async () => {
    const { loads, timers } = harness();
    const pending = loads.load("a.js", 250);
    assert.equal(timers[0]?.ms, 250);
    timers[0]?.cb();
    assert.equal(await pending, undefined);
  });

  it("late_register_after_timeout_is_ignored", async () => {
    const { loads, timers } = harness();
    const pending = loads.load("a.js", 100);
    timers[0]?.cb();
    assert.equal(await pending, undefined);
    assert.doesNotThrow(() => loads.register("file:///x/a.js", create));
  });
});
