import type { ComponentExt, Ctx } from "../../api.ts";
import { formatClock, formatDate } from "../../format.ts";

/** What makes one clock look different from another. */
export interface ClockLook {
  /** The component's stylesheet. */
  css: string;
  /** Class of the clock's box; the time and date inside are `.time` and `.date`, and `.small` marks the small size. */
  className: string;
  /** Optional. Called once the box is on the page, for example to load a font. */
  onMount?(box: HTMLElement): void;
  /** Optional. Called with the time text after mount and whenever it changes, for a look that draws the digits itself. */
  onTime?(text: string): void;
}

/**
 * A clock component that shows the time and the date and ticks once a second. Its
 * `size` option is "large" (the default) or "small". The built-in clock and the
 * chrome clock share it and differ only in their look.
 */
export function createMinuteClock(look: ClockLook): ComponentExt {
  let box: HTMLElement | undefined;
  let time: HTMLElement | undefined;
  let date: HTMLElement | undefined;
  let cancelFrame: (() => void) | undefined;

  // Write only on change: the tick runs every second, the text changes once a minute.
  const show = (element: HTMLElement | undefined, text: string): boolean => {
    if (element === undefined || element.textContent === text) return false;
    element.textContent = text;
    return true;
  };

  const tick = (nowMs: number): void => {
    const moment = new Date(nowMs);
    const clockText = formatClock(moment);
    if (show(time, clockText)) look.onTime?.(clockText);
    show(date, formatDate(moment));
  };

  const setSize = (options: Readonly<Record<string, unknown>>): void => {
    box?.classList.toggle("small", options.size === "small");
  };

  function mount(root: HTMLElement, ctx: Ctx): void {
    const style = document.createElement("style");
    style.textContent = look.css;
    const clock = document.createElement("div");
    clock.className = look.className;
    time = document.createElement("div");
    time.className = "time";
    date = document.createElement("div");
    date.className = "date";
    clock.append(time, date);
    root.append(style, clock);
    box = clock;
    look.onMount?.(clock);
    setSize(ctx.options);
    tick(ctx.now());
    cancelFrame = ctx.requestFrame(() => tick(ctx.now()), 1);
  }

  function unmount(): void {
    cancelFrame?.();
    cancelFrame = undefined;
    box?.parentElement?.replaceChildren();
    box = time = date = undefined;
  }

  return { mount, update() {}, setOptions: setSize, unmount };
}
