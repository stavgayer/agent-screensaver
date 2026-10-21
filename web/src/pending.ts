import type { CreateExtension } from "./api.ts";

/** The part of a `<script>` element that loading needs; faked in tests. */
export interface ScriptHandle {
  /** Absolute URL, the same string `document.currentScript.src` reports. */
  readonly src: string;
  onload: (() => void) | null;
  onerror: (() => void) | null;
  attach(): void;
}

export interface PendingLoadDeps {
  createScript(url: string): ScriptHandle;
  setTimer(callback: () => void, ms: number): unknown;
  clearTimer(handle: unknown): void;
}

export interface PendingLoads {
  /** Loads a classic script; resolves with what it registered, or `undefined`. */
  load(url: string, timeoutMs: number): Promise<CreateExtension | undefined>;
  /** Called from `register`, with the running script's `src`. */
  register(currentSrc: string | undefined, create: CreateExtension): void;
}

interface Pending {
  registered?: CreateExtension;
  finish(create: CreateExtension | undefined): void;
}

/**
 * Tracks scripts that are loading. An entry file calls `register` while it
 * runs, and the running script's own `src` says which load it belongs to, so
 * several loads can overlap. Kept free of the DOM so it can be unit tested.
 */
export function createPendingLoads(deps: PendingLoadDeps): PendingLoads {
  const pending = new Map<string, Pending>();

  function load(url: string, timeoutMs: number): Promise<CreateExtension | undefined> {
    return new Promise((resolve) => {
      const script = deps.createScript(url);
      const timer = deps.setTimer(() => finish(undefined), timeoutMs);
      const entry: Pending = { finish };

      function finish(create: CreateExtension | undefined): void {
        deps.clearTimer(timer);
        if (pending.get(script.src) === entry) pending.delete(script.src);
        script.onload = null;
        script.onerror = null;
        resolve(create);
      }

      // `register` runs during script execution, before `load` fires.
      script.onload = () => finish(entry.registered);
      script.onerror = () => finish(undefined);
      pending.set(script.src, entry);
      script.attach();
    });
  }

  function register(currentSrc: string | undefined, create: CreateExtension): void {
    if (currentSrc === undefined) return;
    const entry = pending.get(currentSrc);
    if (entry !== undefined) entry.registered = create;
  }

  return { load, register };
}
