/** One call to Go, as the window's script-message handler expects it. */
export interface Request {
  id: number;
  method: string;
  params: Record<string, unknown>;
}

export interface Timers {
  set(callback: () => void, ms: number): unknown;
  clear(handle: unknown): void;
}

export interface Bridge {
  /** Calls a Go method. `timeoutMs` 0 waits forever (a file dialog can stay open for minutes). */
  call<T = unknown>(method: string, params?: Record<string, unknown>, timeoutMs?: number): Promise<T>;
  /** Go answers through this, by evaluating `window.__agsReply(id, result, error)`. */
  reply(id: number, result: unknown, error: string | null): void;
}

const DEFAULT_TIMEOUT_MS = 10_000;

/**
 * Wraps the one-way message channel to Go in promises: each call gets an id,
 * and the reply with that id settles it.
 */
export function createBridge(post: (request: Request) => void, timers: Timers): Bridge {
  const pending = new Map<number, { resolve(value: unknown): void; reject(error: Error): void; timer?: unknown }>();
  let nextId = 1;

  return {
    call<T>(method: string, params: Record<string, unknown> = {}, timeoutMs = DEFAULT_TIMEOUT_MS): Promise<T> {
      const id = nextId++;
      return new Promise<T>((resolve, reject) => {
        const entry: { resolve(value: unknown): void; reject(error: Error): void; timer?: unknown } = {
          resolve: (value) => resolve(value as T),
          reject,
        };
        if (timeoutMs > 0) {
          entry.timer = timers.set(() => {
            pending.delete(id);
            reject(new Error(`${method} did not answer`));
          }, timeoutMs);
        }
        pending.set(id, entry);
        post({ id, method, params });
      });
    },
    reply(id, result, error) {
      const entry = pending.get(id);
      if (entry === undefined) return;
      pending.delete(id);
      if (entry.timer !== undefined) timers.clear(entry.timer);
      if (error !== null) entry.reject(new Error(error));
      else entry.resolve(result);
    },
  };
}
