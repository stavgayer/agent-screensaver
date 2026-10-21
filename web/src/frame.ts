const MIN_FPS = 1;
const MAX_FPS = 240;
// About a quarter of a 60 Hz frame: a frame landing slightly early still counts.
const SLACK_MS = 4;

/** What `requestFrame(cb)` without an fps asks for: every display frame. */
export function requestedFps(fps: number | undefined): number {
  return fps ?? MAX_FPS;
}

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
  onError: (err: unknown) => void = (err) => console.error(err),
): FrameScheduler {
  const requests = new Set<Request>();
  let looping = false;

  /** Decides if a request renders now and moves its schedule anchor. */
  function isDue(request: Request, nowMs: number): boolean {
    if (request.lastMs === undefined) {
      request.lastMs = nowMs;
      return true;
    }
    if (!shouldRender(request.lastMs, nowMs + SLACK_MS, request.fps)) return false;
    // Advance by whole intervals, not to `nowMs`, so jitter does not drift the rate.
    const interval = 1000 / request.fps;
    request.lastMs += interval * Math.floor((nowMs + SLACK_MS - request.lastMs) / interval);
    return true;
  }

  function frame(nowMs: number): void {
    // Snapshot: a callback may add or cancel requests while we iterate.
    for (const request of [...requests]) {
      if (!requests.has(request)) continue;
      if (!isDue(request, nowMs)) continue;
      try {
        request.callback(nowMs);
      } catch (err) {
        // One broken extension must not stop the loop for the others.
        requests.delete(request);
        onError(err);
      }
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
