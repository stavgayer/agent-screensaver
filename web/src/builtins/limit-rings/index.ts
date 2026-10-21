import type { ComponentExt, Limit, ViewState } from "../../api.ts";
import { ringLevel } from "../../format.ts";
import { orderLimits, ringCaption, ringPercent } from "./logic.ts";

const STYLE = `
.rings { display: flex; justify-content: flex-end; gap: 1.8vmin; }
.ring-item { text-align: center; max-width: 14vmin; }
.ring { --ring: var(--accent, #00e07a); width: 8.5vmin; height: 8.5vmin; margin: 0 auto; border-radius: 50%; display: grid; place-items: center; color: #fff; font-size: 2vmin;
  background: conic-gradient(var(--ring) calc(var(--pct) * 1%), rgba(255, 255, 255, 0.15) 0); }
.ring.warn { --ring: #e0c000; }
.ring.high { --ring: #ff5040; }
.ring .inner { width: 78%; height: 78%; border-radius: 50%; background: #000; display: grid; place-items: center; }
.ring-caption { margin-top: 0.6vmin; font-size: 1.6vmin; opacity: 0.65; text-transform: uppercase; letter-spacing: 0.06em; }
`;

function createRing(limit: Limit, nowMs: number): HTMLElement {
  const percent = ringPercent(limit.used_percent);
  const item = document.createElement("div");
  item.className = "ring-item";
  const ring = document.createElement("div");
  ring.className = `ring ${ringLevel(percent)}`;
  ring.style.setProperty("--pct", String(percent));
  const inner = document.createElement("span");
  inner.className = "inner";
  inner.textContent = `${percent}%`;
  ring.appendChild(inner);
  const caption = document.createElement("div");
  caption.className = "ring-caption";
  caption.textContent = ringCaption(limit, nowMs);
  item.append(ring, caption);
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
    const ordered = orderLimits(state.limits);
    // Rebuild only when something visible changed.
    const key = JSON.stringify(ordered.map((limit) => [ringPercent(limit.used_percent), ringCaption(limit, state.nowMs)]));
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
