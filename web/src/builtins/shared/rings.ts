import type { ComponentExt, Limit, ViewState } from "../../api.ts";
import { ringLevel } from "../../format.ts";
import { orderLimits, ringView } from "../limit-rings/logic.ts";

/** What makes one set of limit rings look different from another. */
export interface RingsLook {
  /** The component's stylesheet. */
  css: string;
  /** Class of the row that holds the rings. Each ring is `.ring-item` with `.ring`, `.ring-name` and `.ring-detail`. */
  className: string;
  /** The element inside the ring that shows its text, for example "61%". */
  center(text: string): HTMLElement;
  /** Optional. Called once the row is on the page, for example to load a font. */
  onMount?(): void;
}

export function textLine(className: string, text: string): HTMLElement {
  const element = document.createElement("div");
  element.className = className;
  element.textContent = text;
  return element;
}

function createRing(limit: Limit, nowMs: number, look: RingsLook): HTMLElement {
  const { look: freshness, percent, center, name, detail } = ringView(limit, nowMs);
  const item = document.createElement("div");
  item.className = "ring-item";
  const ring = document.createElement("div");
  ring.className = `ring ${ringLevel(percent)} ${freshness}`;
  ring.style.setProperty("--pct", String(percent));
  ring.appendChild(look.center(center));
  item.append(ring, textLine("ring-name", name));
  // An empty detail still takes its line, so rings with and without a reset time line up.
  item.appendChild(textLine("ring-detail", detail === "" ? "\u00a0" : detail));
  return item;
}

/**
 * A component with one ring per usage limit, in the order and with the captions of
 * limit-rings/logic.ts. The built-in limit rings and the chrome rings share it and
 * differ only in their look.
 */
export function createRingsComponent(look: RingsLook): ComponentExt {
  let rings: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = look.css;
    rings = document.createElement("div");
    rings.className = look.className;
    root.append(style, rings);
    lastKey = "";
    look.onMount?.();
  }

  function update(state: ViewState): void {
    const ordered = orderLimits(state.limits, state.agentOrder);
    // Rebuild only when something visible changed.
    const key = JSON.stringify(ordered.map((limit) => ringView(limit, state.nowMs)));
    if (key === lastKey) return;
    lastKey = key;
    rings?.replaceChildren(...ordered.map((limit) => createRing(limit, state.nowMs, look)));
  }

  function unmount(): void {
    rings?.parentElement?.replaceChildren();
    rings = undefined;
  }

  return { mount, update, unmount };
}
