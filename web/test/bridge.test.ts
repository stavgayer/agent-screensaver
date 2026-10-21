import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { createBridge, type Request } from "../settings/bridge.ts";

function setup() {
  const posted: Request[] = [];
  const timers = new Map<number, () => void>();
  let nextTimer = 1;
  const bridge = createBridge((request) => posted.push(request), {
    set: (callback) => {
      const id = nextTimer++;
      timers.set(id, callback);
      return id;
    },
    clear: (id) => timers.delete(id as number),
  });
  return { bridge, posted, timers };
}

describe("createBridge", () => {
  it("posts the call and resolves with the matching reply", async () => {
    const { bridge, posted } = setup();
    const first = bridge.call("setTheme", { id: "builtin.default" });
    const second = bridge.call("snapshot");
    assert.deepEqual(posted, [
      { id: 1, method: "setTheme", params: { id: "builtin.default" } },
      { id: 2, method: "snapshot", params: {} },
    ]);
    bridge.reply(2, { theme: "x" }, null);
    bridge.reply(1, null, null);
    assert.deepEqual(await second, { theme: "x" });
    assert.equal(await first, null);
  });

  it("rejects with the error text", async () => {
    const { bridge } = setup();
    const pending = bridge.call("setMaxFps", { fps: 999 });
    bridge.reply(1, null, "max fps 999 is out of range");
    await assert.rejects(pending, { message: "max fps 999 is out of range" });
  });

  it("times out when no reply comes", async () => {
    const { bridge, timers } = setup();
    const pending = bridge.call("snapshot", {}, 50);
    [...timers.values()][0]?.();
    await assert.rejects(pending, { message: "snapshot did not answer" });
  });

  it("a call without a timeout waits as long as it takes", () => {
    const { bridge, timers } = setup();
    void bridge.call("installExtension", {}, 0);
    assert.equal(timers.size, 0);
  });

  it("ignores a reply nobody waits for", () => {
    const { bridge } = setup();
    assert.doesNotThrow(() => bridge.reply(42, null, null));
  });
});
