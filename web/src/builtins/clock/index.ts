import type { ComponentExt, Ctx } from "../../api.ts";
import { formatClock, formatDate } from "../../format.ts";

const STYLE = `
.clock { text-align: center; color: #b8ffd8; text-shadow: 0 0 14px color-mix(in srgb, var(--accent, #00e07a) 55%, transparent); font-variant-numeric: tabular-nums; }
.clock .time { font-weight: 300; line-height: 1; font-size: 16vmin; transition: font-size 400ms ease; }
.clock .date { color: #9fdcbc; opacity: 0.8; margin-top: 0.8vmin; font-size: 2.6vmin; transition: font-size 400ms ease; }
.clock.small { text-align: left; }
.clock.small .time { font-size: 4.7vmin; }
.clock.small .date { display: none; }
@media (prefers-reduced-motion: reduce) { .clock .time, .clock .date { transition: none; } }
`;

function createClock(): ComponentExt {
  let box: HTMLElement | undefined;
  let time: HTMLElement | undefined;
  let date: HTMLElement | undefined;
  let cancelFrame: (() => void) | undefined;

  const tick = (nowMs: number): void => {
    const moment = new Date(nowMs);
    if (time) time.textContent = formatClock(moment);
    if (date) date.textContent = formatDate(moment);
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
