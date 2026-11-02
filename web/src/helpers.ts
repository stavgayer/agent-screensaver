import type { BackgroundExt, Ctx, NowPlaying, ViewState } from "./api.ts";

const CONTEXT_RESTORE_DELAY_MS = 400;

/**
 * How far the song is, 0 to 1, at `nowMs` (wall time, `ctx.now()`), or null
 * when the app gives no length. Between the service's reads the position
 * moves on from `at_ms`; it stops at the song's end.
 */
export function songProgress(track: NowPlaying, nowMs: number): number | null {
  if (!(track.duration_ms > 0)) return null;
  const positionMs = track.position_ms + Math.max(0, nowMs - track.at_ms);
  return Math.min(1, Math.max(0, positionMs / track.duration_ms));
}

/**
 * Wraps a background so it starts again on a fresh canvas, with the last
 * state, after WebGL loses its context. `stop` takes the listener off before
 * it calls `api.stop`: a background may lose the context on purpose to free
 * the GPU, and that must not start it again.
 */
export function recoverWebGL(api: BackgroundExt, surfaceOf: () => HTMLCanvasElement | undefined): BackgroundExt {
  let root: HTMLElement | undefined;
  let ctx: Ctx | undefined;
  let state: ViewState | undefined;
  let canvas: HTMLCanvasElement | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function attach(): void {
    canvas = surfaceOf();
    canvas?.addEventListener("webglcontextlost", lost);
  }
  function detach(): void {
    canvas?.removeEventListener("webglcontextlost", lost);
    canvas = undefined;
  }
  function restart(): void {
    timer = undefined;
    if (root === undefined || ctx === undefined) return;
    const last = state;
    detach();
    api.stop();
    api.start(root, ctx);
    attach();
    if (last !== undefined) api.update?.(last);
  }
  function lost(event: Event): void {
    event.preventDefault();
    if (timer !== undefined) return;
    timer = setTimeout(restart, CONTEXT_RESTORE_DELAY_MS);
  }

  return {
    start(newRoot, newCtx) {
      root = newRoot;
      ctx = newCtx;
      api.start(newRoot, newCtx);
      attach();
    },
    update(newState) {
      state = newState;
      api.update?.(newState);
    },
    stop() {
      clearTimeout(timer);
      timer = undefined;
      detach();
      api.stop();
      root = ctx = state = undefined;
    },
  };
}
