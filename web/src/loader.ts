import type { CreateExtension } from "./api.ts";
import { createPendingLoads, type PendingLoads, type ScriptHandle } from "./pending.ts";

const DEFAULT_TIMEOUT_MS = 2000;

/** Runs `fn`; on a throw, reports it and returns `undefined` so one extension cannot break the screen. */
export function safeCall<T>(
  label: string,
  fn: () => T,
  onFail: (label: string, err: unknown) => void,
): T | undefined {
  try {
    return fn();
  } catch (err) {
    onFail(label, err);
    return undefined;
  }
}

/** Loads must go through one shared bookkeeper so `register` can find them. */
export const pendingLoads: PendingLoads = createPendingLoads({
  createScript(url) {
    const element = document.createElement("script");
    element.src = url;
    const handle: ScriptHandle = {
      get src() {
        return element.src;
      },
      onload: null,
      onerror: null,
      attach() {
        element.onload = () => handle.onload?.();
        element.onerror = () => handle.onerror?.();
        document.head.appendChild(element);
      },
    };
    return handle;
  },
  setTimer: (callback, ms) => setTimeout(callback, ms),
  clearTimer: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
});

/**
 * Injects `entryUrl` as a classic script and resolves with the `create`
 * function it registered. A missing, slow or silent script gives `undefined`.
 * Classic scripts, not `import()`: WebKit refuses ES modules on file://.
 */
export function loadExtension(
  entryUrl: string,
  timeoutMs: number = DEFAULT_TIMEOUT_MS,
): Promise<CreateExtension | undefined> {
  return pendingLoads.load(entryUrl, timeoutMs);
}
