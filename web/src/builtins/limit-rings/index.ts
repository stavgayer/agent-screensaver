import type { ComponentExt, Limit, ViewState } from "../../api.ts";
import { ringLevel } from "../../format.ts";
import { orderLimits, ringView } from "./logic.ts";

// Captions are two short lines that never wrap: the name, then the reset time in a quieter tone.
// The second line says when the ring resets, to the minute. An old reading dims its ring and says its age;
// after a reset the ring is empty and the line says "Reset", so an old number is never read as the current one.
const STYLE = `
.rings { display: flex; justify-content: flex-end; align-items: flex-start; gap: 3vmin; }
.ring-item { display: flex; flex-direction: column; align-items: center; text-align: center; white-space: nowrap; }
.ring { --ring: var(--accent, #00e07a); width: 9vmin; height: 9vmin; border-radius: 50%; display: grid; place-items: center; color: #fff;
  font-size: 2.3vmin; font-weight: 600; font-variant-numeric: tabular-nums; letter-spacing: -0.01em;
  background: conic-gradient(var(--ring) calc(var(--pct) * 1%), rgba(255, 255, 255, 0.16) 0); } /* taste-skill-ignore: the conic fill is the ring's arc (the chart itself) */
.ring.warn { --ring: #e0c000; }
.ring.high { --ring: #ff5040; }
.ring.stale { opacity: 0.45; }
.ring .inner { width: 80%; height: 80%; border-radius: 50%; background: #000; display: grid; place-items: center; }
.ring-name { margin-top: 1.2vmin; font-size: 1.9vmin; font-weight: 500; color: rgba(255, 255, 255, 0.9); }
.ring-detail { margin-top: 0.3vmin; font-size: 1.7vmin; color: rgba(255, 255, 255, 0.68); font-variant-numeric: tabular-nums; }
.ring-name, .ring-detail { line-height: 1.25; text-shadow: 0 0 0.8vmin #000, 0 0 0.3vmin #000; } /* taste-skill-ignore: a static dark halo keeps captions legible over the rain */
`;

function textLine(className: string, text: string): HTMLElement {
  const element = document.createElement("div");
  element.className = className;
  element.textContent = text;
  return element;
}

function createRing(limit: Limit, nowMs: number): HTMLElement {
  const { look, percent, center, name, detail } = ringView(limit, nowMs);
  const item = document.createElement("div");
  item.className = "ring-item";
  const ring = document.createElement("div");
  ring.className = `ring ${ringLevel(percent)} ${look}`;
  ring.style.setProperty("--pct", String(percent));
  const inner = document.createElement("span");
  inner.className = "inner";
  inner.textContent = center;
  ring.appendChild(inner);
  item.append(ring, textLine("ring-name", name));
  // An empty detail still takes its line, so rings with and without a reset time line up.
  item.appendChild(textLine("ring-detail", detail === "" ? "\u00a0" : detail));
  return item;
}

function createLimitRings(): ComponentExt {
  let rings: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = STYLE;
    rings = document.createElement("div");
    rings.className = "rings";
    root.append(style, rings);
  }

  function update(state: ViewState): void {
    const ordered = orderLimits(state.limits, state.agentOrder);
    // Rebuild only when something visible changed.
    const key = JSON.stringify(ordered.map((limit) => ringView(limit, state.nowMs)));
    if (key === lastKey) return;
    lastKey = key;
    rings?.replaceChildren(...ordered.map((limit) => createRing(limit, state.nowMs)));
  }

  function unmount(): void {
    rings?.parentElement?.replaceChildren();
    rings = undefined;
  }

  return { mount, update, unmount };
}

window.agentScreensaver.register(createLimitRings);
