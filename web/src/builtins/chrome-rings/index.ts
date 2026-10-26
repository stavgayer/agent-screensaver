import { loadFont } from "../shared/fonts.ts";
import { createRingsComponent, textLine } from "../shared/rings.ts";

const FONT_FAMILY = "Chrome Rings Mono";

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// The same rings as builtin.limit-rings (same order, numbers and captions, from its
// logic), drawn as thin bright arcs around a dark disc, with tiny monospace labels.
// The arc is a conic fill cut to a thin band by a radial mask. The font stack ends in
// the system monospace face, which draws until the bundled one is ready.
const STYLE = `
.chrome-rings { display: flex; justify-content: flex-end; align-items: flex-start; gap: 2.6vmin;
  font-family: "${FONT_FAMILY}", ui-monospace, Menlo, monospace; font-variant-numeric: tabular-nums; }
.chrome-rings .ring-item { display: flex; flex-direction: column; align-items: center; text-align: center; white-space: nowrap; }
.chrome-rings .ring { --ring: var(--accent, #c8ff2e); position: relative; width: 8.4vmin; height: 8.4vmin; border-radius: 50%;
  display: grid; place-items: center; background: radial-gradient(circle, rgba(8, 9, 11, 0.82) 62%, transparent 63%); }
.chrome-rings .ring::before { content: ""; position: absolute; inset: 0; border-radius: 50%;
  background: conic-gradient(var(--ring) calc(var(--pct) * 1%), rgba(255, 255, 255, 0.16) 0);
  -webkit-mask: radial-gradient(farthest-side, transparent calc(100% - 0.55vmin), #000 calc(100% - 0.5vmin));
  mask: radial-gradient(farthest-side, transparent calc(100% - 0.55vmin), #000 calc(100% - 0.5vmin)); } /* taste-skill-ignore: the conic fill is the ring's arc (the chart itself) */
.chrome-rings .ring.warn { --ring: #ffd23f; }
.chrome-rings .ring.high { --ring: var(--attention, #ff3d9a); }
.chrome-rings .ring.stale { opacity: 0.45; }
.chrome-rings .ring-value { position: relative; color: #f4f6f8; font-size: 2.2vmin; font-weight: 600; letter-spacing: -0.02em; }
.chrome-rings .ring-name { margin-top: 1.1vmin; font-size: 1.45vmin; font-weight: 600; letter-spacing: 0.02em; color: rgba(244, 246, 248, 0.92); }
.chrome-rings .ring-detail { margin-top: 0.35vmin; font-size: 1.35vmin; color: color-mix(in srgb, var(--accent, #c8ff2e) 55%, #c9ced4); }
.chrome-rings .ring-name, .chrome-rings .ring-detail { line-height: 1.3; text-shadow: 0 0 0.8vmin #000, 0 0 0.3vmin #000; } /* taste-skill-ignore: a static dark halo keeps captions legible over the scene */
`;

function loadMono(): void {
  if (scriptSrc !== "") void loadFont(FONT_FAMILY, new URL("fonts/JetBrainsMono.woff2", scriptSrc).href, { weight: "100 800" });
}

window.agentScreensaver.register(() =>
  createRingsComponent({ css: STYLE, className: "chrome-rings", center: (text) => textLine("ring-value", text), onMount: loadMono }),
);
