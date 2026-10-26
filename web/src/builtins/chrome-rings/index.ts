import { loadFont } from "../shared/fonts.ts";
import { createRingsComponent, textLine } from "../shared/rings.ts";

const FONT_FAMILY = "Chrome Rings Mono";

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// The same rings as builtin.limit-rings (same order, numbers and captions, from its
// logic), drawn as thin bright arcs around a dark disc, with monospace captions set
// large enough to read from across the room: the name in capitals, the reset below.
// The arc is a conic fill cut to a thin band by a radial mask. The font stack ends in
// the system monospace face, which draws until the bundled one is ready. The token
// block holds every color and face; the theme can set --accent, --warn and --attention.
const STYLE = `
:root { --chrome-rings-lime: #c8ff2e; --chrome-rings-warn: #fff1c7; --chrome-rings-pink: #ff3d9a; --chrome-rings-ink: #f4f6f8;
  --chrome-rings-steel: #c9ced4; --chrome-rings-disc: rgba(8, 9, 11, 0.82); --chrome-rings-track: rgba(255, 255, 255, 0.16);
  --chrome-rings-shade: #000; --chrome-rings-mono: "${FONT_FAMILY}", ui-monospace, Menlo, monospace; }
.chrome-rings { display: flex; justify-content: flex-end; align-items: flex-start; gap: 2.4vmin;
  font-family: var(--chrome-rings-mono); font-variant-numeric: tabular-nums; }
.chrome-rings .ring-item { display: flex; flex-direction: column; align-items: center; text-align: center; white-space: nowrap; }
.chrome-rings .ring { --ring: var(--accent, var(--chrome-rings-lime)); position: relative; width: 9.6vmin; height: 9.6vmin; border-radius: 50%;
  display: grid; place-items: center; background: radial-gradient(circle, var(--chrome-rings-disc) 62%, transparent 63%); }
.chrome-rings .ring::before { content: ""; position: absolute; inset: 0; border-radius: 50%;
  background: conic-gradient(var(--ring) calc(var(--pct) * 1%), var(--chrome-rings-track) 0);
  -webkit-mask: radial-gradient(farthest-side, transparent calc(100% - 0.6vmin), var(--chrome-rings-shade) calc(100% - 0.55vmin));
  mask: radial-gradient(farthest-side, transparent calc(100% - 0.6vmin), var(--chrome-rings-shade) calc(100% - 0.55vmin)); } /* taste-skill-ignore: the conic fill is the ring's arc (the chart itself) */
.chrome-rings .ring.warn { --ring: var(--warn, var(--chrome-rings-warn)); }
.chrome-rings .ring.high { --ring: var(--attention, var(--chrome-rings-pink)); }
.chrome-rings .ring.stale { opacity: 0.45; }
.chrome-rings .ring-value { position: relative; color: var(--chrome-rings-ink); font-size: 2.6vmin; font-weight: 600; letter-spacing: -0.02em; }
.chrome-rings .ring-name { margin-top: 1.2vmin; font-size: 2vmin; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase;
  color: var(--chrome-rings-ink); }
.chrome-rings .ring-detail { margin-top: 0.4vmin; font-size: 1.85vmin; font-weight: 500;
  color: color-mix(in srgb, var(--accent, var(--chrome-rings-lime)) 55%, var(--chrome-rings-steel)); }
.chrome-rings .ring-name, .chrome-rings .ring-detail { line-height: 1.25;
  text-shadow: 0 0 0.8vmin var(--chrome-rings-shade), 0 0 0.3vmin var(--chrome-rings-shade); } /* taste-skill-ignore: a static dark halo keeps captions legible over the scene */
`;

function loadMono(): void {
  if (scriptSrc !== "") void loadFont(FONT_FAMILY, new URL("fonts/JetBrainsMono.woff2", scriptSrc).href, { weight: "100 800" });
}

window.agentScreensaver.register(() =>
  createRingsComponent({ css: STYLE, className: "chrome-rings", center: (text) => textLine("ring-value", text), onMount: loadMono }),
);
