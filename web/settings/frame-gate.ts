// Runs inside the settings window's preview frame, in place of the browser's
// animation frames. Each animation frame costs WebKit a full drawing pass
// across its processes, even when no extension draws in it, so the preview
// takes few of them: callbacks run together on a timer, at most once every
// intervalMs. The settings page can also pause the preview when nobody is
// looking at it: then the callbacks wait here, the runtime and every
// extension stop drawing at once, and the last picture stays on the screen.

type Callback = (nowMs: number) => void;

export interface FrameGateClock {
  now(): number;
  setTimer(callback: () => void, delayMs: number): void;
}

export interface FrameGate {
  /** Stands in for requestAnimationFrame. */
  request(callback: Callback): number;
  /** Stands in for cancelAnimationFrame. */
  cancel(id: number): void;
  /** While paused, callbacks wait; on resume they run in the order they came. */
  setPaused(paused: boolean): void;
}

export function createFrameGate(clock: FrameGateClock, intervalMs: number): FrameGate {
  let nextId = 1;
  let paused = false;
  let timerSet = false;
  let lastMs: number | undefined;
  const waiting = new Map<number, Callback>();

  function run(): void {
    timerSet = false;
    if (paused) return;
    lastMs = clock.now();
    const callbacks = [...waiting.values()];
    waiting.clear();
    // Like the browser's frames: one callback that throws does not stop the others.
    for (const callback of callbacks) {
      try {
        callback(lastMs);
      } catch (err) {
        console.error(err);
      }
    }
  }

  function schedule(): void {
    if (paused || timerSet || waiting.size === 0) return;
    timerSet = true;
    const dueMs = lastMs === undefined ? 0 : lastMs + intervalMs - clock.now();
    clock.setTimer(run, Math.max(0, dueMs));
  }

  return {
    request(callback) {
      const id = nextId++;
      waiting.set(id, callback);
      schedule();
      return id;
    },
    cancel(id) {
      waiting.delete(id);
    },
    setPaused(next) {
      if (next === paused) return;
      paused = next;
      schedule();
    },
  };
}
