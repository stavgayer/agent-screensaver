import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { safeCall } from "../src/loader.ts";

describe("safeCall", () => {
  it("safe_call_returns_value", () => {
    const failures: string[] = [];
    const value = safeCall("ok", () => 42, (label) => failures.push(label));
    assert.equal(value, 42);
    assert.deepEqual(failures, []);
  });

  it("safe_call_reports_and_swallows_errors", () => {
    const failures: { label: string; err: unknown }[] = [];
    const boom = new Error("boom");
    const value = safeCall("bad", () => { throw boom; }, (label, err) => failures.push({ label, err }));
    assert.equal(value, undefined);
    assert.deepEqual(failures, [{ label: "bad", err: boom }]);
  });
});
