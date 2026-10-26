import { loadFont } from "../shared/fonts.ts";
import { createMinuteClock } from "../shared/clock.ts";

const FONT_FAMILY = "Chrome Clock Anton";
const SVG_NS = "http://www.w3.org/2000/svg";

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// The digits are made of the scene's own material. The HTML time keeps the layout
// and the text for the page, but is drawn transparent; an SVG over it draws the
// digits: a soft lime glow and a red and a blue copy shifted sideways (the scene's
// chromatic fringe), both kept outside the digits by a mask, the chrome face (bright top, a see-through dark horizon, a
// lime reflection band, a light bounce below) and fine scan lines. Under the face
// a "light" layer is cut to the same digits and shows the scene itself, brightened,
// through backdrop-filter: the scene's grain and the shape's light pass through the
// digits. A blend mode cannot do this: the screen layer is a separate stacking
// context, so a blend never reaches the background canvas. Nothing here may set a
// filter, opacity or transform on the clock box, or the light would sample the box
// instead of the scene.
// The system face draws until the bundled one is ready (.font-ready), so the time
// never waits for a font. Anton has one weight; the fallback is bold and narrow so
// the clock keeps its shape. The date hangs below the time (out of flow), like the
// built-in clock, so the runtime's move scales the digits evenly. The large size is
// capped by the width too, so on a 4:3 screen the digits stay clear of the shape.
const STYLE = `
.chrome-clock { --size: min(24vmin, 13.5vw); position: relative; line-height: 1; font-variant-numeric: tabular-nums;
  font-family: "Helvetica Neue", Arial, sans-serif; font-weight: 700; font-stretch: condensed; letter-spacing: -0.01em; }
.chrome-clock.font-ready { font-family: "${FONT_FAMILY}", "Helvetica Neue", Arial, sans-serif; font-weight: 400; }
.chrome-clock.small { --size: 7vmin; }
.chrome-clock .time { font-size: var(--size); color: transparent; -webkit-text-fill-color: transparent; }
.chrome-clock .glass { position: absolute; inset: 0; font-size: var(--size); pointer-events: none; }
.chrome-clock .light { position: absolute; inset: 0;
  -webkit-backdrop-filter: blur(0.35vmin) brightness(2.6) saturate(1.7); backdrop-filter: blur(0.35vmin) brightness(2.6) saturate(1.7); }
.chrome-clock .chrome { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible; }
.chrome-clock .face { font: inherit; letter-spacing: inherit; }
.chrome-clock .glow { fill: var(--accent, #c8ff2e); opacity: 0.32; filter: blur(0.9vmin); }
.chrome-clock .red { fill: #ff2d55; opacity: 0.75; transform: translateX(-0.016em); }
.chrome-clock .blue { fill: #2d8cff; opacity: 0.75; transform: translateX(0.016em); }
/* The scan lines are fixed pixels, so the small digits get fewer, fainter ones. */
.chrome-clock.small .lines { opacity: 0.55; }
.chrome-clock .date { position: absolute; top: 100%; left: 0.2em; white-space: nowrap; margin-top: 1.4vmin;
  font-size: 2.6vmin; letter-spacing: 0.04em; color: color-mix(in srgb, var(--accent, #c8ff2e) 80%, #fff);
  text-shadow: -0.07em 0 rgba(255, 45, 85, 0.55), 0.07em 0 rgba(45, 140, 255, 0.55), 0 0 0.6em color-mix(in srgb, var(--accent, #c8ff2e) 45%, transparent);
  transition: opacity 200ms ease; } /* taste-skill-ignore: the fringe and glow are the scene's own chromatic look */
.chrome-clock.small .date { opacity: 0; }
@media (prefers-reduced-motion: reduce) { .chrome-clock .date { transition: none; } }
`;

/** Where the baseline sits in a line box of line-height 1, as a share of the font size (Anton and its fallback). */
const BASELINE_EM = "0.91em";

// Stops of the chrome face, top to bottom. The horizon is half see-through so the
// scene's light, from the layer below, shows in the middle of every digit.
const METAL_STOPS: Array<[string, string, number]> = [
  ["0", "#ffffff", 1],
  ["0.3", "#e4e8ec", 1],
  ["0.47", "#9aa1aa", 0.92],
  ["0.52", "#2b3036", 0.3],
  ["0.56", "#c8ff2e", 0.9],
  ["0.6", "#eef8d8", 0.95],
  ["0.7", "#cfd4da", 1],
  ["0.85", "#f6f8fa", 1],
  ["1", "#8b929b", 1],
];

let nextId = 0;

function svgElement<K extends keyof SVGElementTagNameMap>(tag: K, attributes: Record<string, string> = {}): SVGElementTagNameMap[K] {
  const element = document.createElementNS(SVG_NS, tag);
  for (const [name, value] of Object.entries(attributes)) element.setAttribute(name, value);
  return element;
}

/** One copy of the digits; every copy gets the same text on each minute. */
function face(className: string, fill?: string): SVGTextElement {
  const text = svgElement("text", { class: `face ${className}`, x: "0", y: BASELINE_EM });
  if (fill !== undefined) text.setAttribute("fill", fill);
  return text;
}

function createChromeClock() {
  const id = `chrome-clock-${nextId++}`;
  let faces: SVGTextElement[] = [];

  /** The light layer and the SVG that draws the digits, over the transparent HTML time. */
  function buildGlass(box: HTMLElement): void {
    const glass = document.createElement("div");
    glass.className = "glass";
    glass.setAttribute("aria-hidden", "true");

    const light = document.createElement("div");
    light.className = "light";
    light.style.setProperty("clip-path", `url(#${id}-clip)`);

    const svg = svgElement("svg", { class: "chrome" });
    const defs = svgElement("defs");
    const metal = svgElement("linearGradient", { id: `${id}-metal`, x1: "0", y1: "0", x2: "0", y2: "1" });
    for (const [offset, color, alpha] of METAL_STOPS) {
      metal.appendChild(svgElement("stop", { offset, "stop-color": color, "stop-opacity": String(alpha) }));
    }
    // Scan lines: one dark line every three pixels, like the scene's.
    const lines = svgElement("pattern", { id: `${id}-lines`, width: "6", height: "3", patternUnits: "userSpaceOnUse" });
    lines.appendChild(svgElement("rect", { width: "6", height: "1", fill: "#000", "fill-opacity": "0.16" }));
    const clip = svgElement("clipPath", { id: `${id}-clip` });
    const clipFace = face("cut");
    clip.appendChild(clipFace);
    // The glow and the fringes live only outside the digits, so the see-through
    // horizon shows the scene's light, not a red and blue copy.
    const outside = svgElement("mask", { id: `${id}-outside`, maskUnits: "userSpaceOnUse", x: "-50%", y: "-50%", width: "200%", height: "200%" });
    const maskFace = face("hole", "#000");
    outside.append(svgElement("rect", { x: "-50%", y: "-50%", width: "200%", height: "200%", fill: "#fff" }), maskFace);
    defs.append(metal, lines, clip, outside);

    const halo = svgElement("g", { mask: `url(#${id}-outside)` });
    const haloFaces = [face("glow"), face("red"), face("blue")];
    halo.append(...haloFaces);
    const surface = [face("metal", `url(#${id}-metal)`), face("lines", `url(#${id}-lines)`)];
    svg.append(defs, halo, ...surface);
    glass.append(light, svg);
    box.appendChild(glass);
    faces = [clipFace, maskFace, ...haloFaces, ...surface];
  }

  function loadAnton(box: HTMLElement): void {
    if (scriptSrc === "") return;
    void loadFont(FONT_FAMILY, new URL("fonts/Anton-Regular.woff2", scriptSrc).href).then((ok) => {
      if (ok) box.classList.add("font-ready");
    });
  }

  return createMinuteClock({
    css: STYLE,
    className: "chrome-clock",
    onMount(box) {
      buildGlass(box);
      loadAnton(box);
    },
    onTime(text) {
      for (const copy of faces) copy.textContent = text;
    },
  });
}

window.agentScreensaver.register(createChromeClock);
