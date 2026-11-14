import type { ComponentExt, ViewState } from "../../api.ts";
import { ringLevel } from "../../format.ts";
import { batteryRingLevel, batteryView, batteryWarning, orderLimits, ringView, type RingView } from "../limit-rings/logic.ts";

/** What makes one set of limit rings look different from another. */
export interface RingsLook {
  /** The component's stylesheet. */
  css: string;
  /** Class of the row that holds the rings. Each ring is `.ring-item` with `.ring`, `.ring-name` and `.ring-detail`. The low-battery line, `.ring-warning`, sits under the row. */
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

function createRing({ look: freshness, percent, center, name, detail }: RingView, level: string, look: RingsLook): HTMLElement {
  const item = document.createElement("div");
  item.className = "ring-item";
  const ring = document.createElement("div");
  ring.className = `ring ${level} ${freshness}`;
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
  let warning: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = look.css;
    rings = document.createElement("div");
    rings.className = look.className;
    warning = textLine("ring-warning", "");
    root.append(style, rings, warning);
    lastKey = "";
    look.onMount?.();
  }

  function update(state: ViewState): void {
    const ordered = orderLimits(state.limits, state.agentOrder);
    // The battery ring goes last, after the usage rings. Its low numbers are the bad ones, so it has its own level.
    const items: { view: RingView; level: string }[] = ordered.map((limit) => {
      const view = ringView(limit, state.nowMs);
      return { view, level: ringLevel(view.percent) };
    });
    if (state.power !== undefined) items.push({ view: batteryView(state.power), level: batteryRingLevel(state.power) });
    const warningText = state.power === undefined ? "" : batteryWarning(state.power);
    // Rebuild only when something visible changed.
    const key = JSON.stringify([items, warningText]);
    if (key === lastKey) return;
    lastKey = key;
    rings?.replaceChildren(...items.map(({ view, level }) => createRing(view, level, look)));
    if (warning !== undefined) warning.textContent = warningText;
  }

  function unmount(): void {
    rings?.parentElement?.replaceChildren();
    rings = undefined;
    warning = undefined;
  }

  return { mount, update, unmount };
}
