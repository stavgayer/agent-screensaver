const MIN_FPS = 1;
const MAX_FPS = 240;

/** True when enough time has passed since the last render to hit `fps`. */
export function shouldRender(lastMs: number, nowMs: number, fps: number): boolean {
  return nowMs - lastMs >= 1000 / fps;
}

/**
 * The rate a request really runs at: the lower of what it asked for and the
 * ceiling. A ceiling of 0, a missing one or a non-finite one means "no
 * ceiling", so only the display's own refresh limits the request.
 */
export function effectiveFps(requested: number, maxFps: number): number {
  const wanted = Number.isFinite(requested) && requested > 0 ? requested : MIN_FPS;
  const hasCeiling = Number.isFinite(maxFps) && maxFps > 0;
  const capped = hasCeiling ? Math.min(wanted, maxFps) : wanted;
  return Math.min(MAX_FPS, Math.max(MIN_FPS, capped));
}

export type FrameCallback = (nowMs: number) => void;
type RequestAnimationFrame = (cb: FrameCallback) => number;

interface Request {
  callback: FrameCallback;
  fps: number;
  lastMs?: number;
}

export interface FrameScheduler {
  /** Runs `cb` at `min(fps, maxFps)`. Returns a function that cancels it. */
  request(cb: FrameCallback, fps: number): () => void;
}

/**
 * One shared animation-frame loop serves every request, so ten extensions
 * cost one rAF, not ten. The loop runs only while requests exist.
 */
export function createFrameScheduler(
  maxFps: number,
  raf: RequestAnimationFrame = requestAnimationFrame,
): FrameScheduler {
  const requests = new Set<Request>();
  let looping = false;

  function frame(nowMs: number): void {
    // Snapshot: a callback may add or cancel requests while we iterate.
    for (const request of [...requests]) {
      if (!requests.has(request)) continue;
      const due =
        request.lastMs === undefined || shouldRender(request.lastMs, nowMs, request.fps);
      if (!due) continue;
      request.lastMs = nowMs;
      request.callback(nowMs);
    }
    looping = requests.size > 0;
    if (looping) raf(frame);
  }

  return {
    request(callback, fps) {
      const request: Request = { callback, fps: effectiveFps(fps, maxFps) };
      requests.add(request);
      if (!looping) {
        looping = true;
        raf(frame);
      }
      return () => {
        requests.delete(request);
      };
    },
  };
}
