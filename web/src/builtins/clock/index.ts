import type { ComponentExt, Ctx } from "../../api.ts";
import { formatClock, formatDate } from "../../format.ts";

// The date hangs below the time (out of flow), so the box is the time alone in both sizes.
// The runtime's move then scales width and height by the same amount and the digits do not squash.
// Sizes change with no transition: the runtime animates the move with a transform.
const STYLE = `
.clock { position: relative; text-align: center; color: color-mix(in srgb, var(--accent, #00e07a) 12%, #fff); font-variant-numeric: tabular-nums;
  text-shadow: 0 0 2vmin #000, 0 0 0.6vmin #000; } /* taste-skill-ignore: a static dark halo separates the digits from the rain */
.clock .time { font-weight: 300; line-height: 1; letter-spacing: -0.02em; font-size: 16vmin; }
.clock .date { position: absolute; top: 100%; left: 50%; transform: translateX(-50%); white-space: nowrap; margin-top: 1.2vmin;
  color: color-mix(in srgb, var(--accent, #00e07a) 30%, #fff); opacity: 0.85; font-size: 3vmin; font-weight: 400; transition: opacity 200ms ease; }
.clock.small .time { font-size: 5vmin; }
.clock.small .date { opacity: 0; }
@media (prefers-reduced-motion: reduce) { .clock .date { transition: none; } }
`;

function createClock(): ComponentExt {
  let box: HTMLElement | undefined;
  let time: HTMLElement | undefined;
  let date: HTMLElement | undefined;
  let cancelFrame: (() => void) | undefined;

  // Write only on change: the tick runs every second, the text changes once a minute.
  const show = (element: HTMLElement | undefined, text: string): void => {
    if (element !== undefined && element.textContent !== text) element.textContent = text;
  };

  const tick = (nowMs: number): void => {
    const moment = new Date(nowMs);
    show(time, formatClock(moment));
    show(date, formatDate(moment));
  };

  const setSize = (options: Readonly<Record<string, unknown>>): void => {
    box?.classList.toggle("small", options.size === "small");
  };

  function mount(root: HTMLElement, ctx: Ctx): void {
    const style = document.createElement("style");
    style.textContent = STYLE;
    box = document.createElement("div");
    box.className = "clock";
    time = document.createElement("div");
    time.className = "time";
    date = document.createElement("div");
    date.className = "date";
    box.append(time, date);
    root.append(style, box);
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

window.agentScreensaver.register(createClock);
