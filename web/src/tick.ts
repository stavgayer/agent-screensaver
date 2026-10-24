const MS_PER_MINUTE = 60_000;
/** A timer that fires this close to the next boundary waits for the one after, so one minute never ticks twice. */
const MIN_DELAY_MS = 1_000;

/** Starts a timer; the runtime passes `setTimeout`, tests a fake. */
export type SetTimer = (callback: () => void, delayMs: number) => unknown;

/** How long from `nowMs` to the next whole minute of the clock. */
export function msToNextMinute(nowMs: number): number {
  const delay = MS_PER_MINUTE - (((nowMs % MS_PER_MINUTE) + MS_PER_MINUTE) % MS_PER_MINUTE);
  return delay < MIN_DELAY_MS ? delay + MS_PER_MINUTE : delay;
}

/**
 * Calls `onTick` at every whole minute of the clock. Countdowns and resets
 * are counted in whole minutes, so a tick on the boundary shows a change at
 * once, not up to a minute late. Each timer is set from the clock again, so
 * the ticks do not drift.
 */
export function startMinuteTick(onTick: () => void, now: () => number, setTimer: SetTimer): void {
  const schedule = (): void => {
    setTimer(() => {
      onTick();
      schedule();
    }, msToNextMinute(now()));
  };
  schedule();
}
