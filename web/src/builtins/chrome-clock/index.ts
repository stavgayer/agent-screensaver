import type { ComponentExt, Ctx } from "../../api.ts";
import { formatClock, formatDate } from "../../format.ts";
import { loadFont } from "../shared/fonts.ts";

const FONT_FAMILY = "Chrome Clock Anton";

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// Huge condensed digits with a polished chrome fill: a bright top, a dark horizon
// line just under the middle and a lighter bounce below, like a chrome bar in a
// studio. The system face draws until the bundled one is ready (.font-ready), so the
// time never waits for a font. Anton has one weight; the fallback is bold and narrow
// so the clock keeps its shape. The date hangs below the time (out of flow), like
// the built-in clock, so the runtime's move scales the digits evenly. The large size
// is capped by the width too, so on a 4:3 screen the digits stay clear of the scene's
// shape in the middle.
const STYLE = `
.chrome-clock { position: relative; line-height: 1; font-variant-numeric: tabular-nums;
  font-family: "Helvetica Neue", Arial, sans-serif; font-weight: 700; font-stretch: condensed;
  filter: drop-shadow(0 0.5vmin 1.2vmin rgba(0, 0, 0, 0.85)); }
.chrome-clock.font-ready { font-family: "${FONT_FAMILY}", "Helvetica Neue", Arial, sans-serif; font-weight: 400; }
.chrome-clock .time { font-size: min(24vmin, 13.5vw); letter-spacing: -0.01em; color: #e9ecef;
  background: linear-gradient(180deg, #ffffff 0%, #e4e8ec 30%, #9aa1aa 49%, #3c4148 53%, #c9ced4 66%, #f6f8fa 82%, #8b929b 100%);
  -webkit-background-clip: text; background-clip: text; -webkit-text-fill-color: transparent; } /* taste-skill-ignore: the gradient is the chrome material of the theme */
.chrome-clock .date { position: absolute; top: 100%; left: 0.2em; white-space: nowrap; margin-top: 1.4vmin;
  font-size: 2.6vmin; letter-spacing: 0.04em; color: color-mix(in srgb, var(--accent, #c8ff2e) 80%, #fff);
  transition: opacity 200ms ease; }
.chrome-clock.small .time { font-size: 7vmin; }
.chrome-clock.small .date { opacity: 0; }
@media (prefers-reduced-motion: reduce) { .chrome-clock .date { transition: none; } }
`;

function createChromeClock(): ComponentExt {
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
    const clock = document.createElement("div");
    clock.className = "chrome-clock";
    time = document.createElement("div");
    time.className = "time";
    date = document.createElement("div");
    date.className = "date";
    clock.append(time, date);
    root.append(style, clock);
    box = clock;
    setSize(ctx.options);
    tick(ctx.now());
    cancelFrame = ctx.requestFrame(() => tick(ctx.now()), 1);
    if (scriptSrc !== "") {
      void loadFont(FONT_FAMILY, new URL("fonts/Anton-Regular.woff2", scriptSrc).href).then((ok) => {
        if (ok) clock.classList.add("font-ready");
      });
    }
  }

  function unmount(): void {
    cancelFrame?.();
    cancelFrame = undefined;
    box?.parentElement?.replaceChildren();
    box = time = date = undefined;
  }

  return { mount, update() {}, setOptions: setSize, unmount };
}

window.agentScreensaver.register(createChromeClock);
