import type { ViewState } from "./api.ts";
import { safeCall } from "./loader.ts";

/** One running extension. `root` is only asked to `remove`, so tests need no DOM. */
export interface Instance {
  /** Names the extension in logs and in the failed set. */
  key: string;
  root: { remove(): void };
  cancels: Set<() => void>;
  stop(): void;
  update?(state: ViewState): void;
  dead: boolean;
}

export interface Keeper {
  /** Takes an extension down once: stop it, cancel its frames, remove its element. */
  teardown(item: Instance): void;
  /** Runs one extension call; a throw takes the extension down and marks it failed. */
  guard<T>(item: Instance, step: string, fn: () => T): T | undefined;
  /** Registers a frame request for `item`. A dead item gets a no-op and no request. */
  trackFrame(item: Instance, request: () => () => void): () => void;
  /** Calls `update` on every live item. */
  updateAll(items: Instance[], state: ViewState): void;
}

export function createKeeper(failed: Set<string>, log: (label: string, err: unknown) => void): Keeper {
  function teardown(item: Instance): void {
    if (item.dead) return;
    item.dead = true;
    safeCall(`${item.key}.stop`, () => item.stop(), log);
    item.cancels.forEach((cancel) => safeCall(`${item.key}.cancel`, cancel, log));
    item.root.remove();
  }

  function guard<T>(item: Instance, step: string, fn: () => T): T | undefined {
    return safeCall(`${item.key}.${step}`, fn, (label, err) => {
      log(label, err);
      failed.add(item.key);
      teardown(item);
    });
  }

  function trackFrame(item: Instance, request: () => () => void): () => void {
    if (item.dead) return () => {};
    const cancel = request();
    item.cancels.add(cancel);
    return cancel;
  }

  function updateAll(items: Instance[], state: ViewState): void {
    for (const item of items) {
      if (!item.dead) guard(item, "update", () => item.update?.(state));
    }
  }

  return { teardown, guard, trackFrame, updateAll };
}
