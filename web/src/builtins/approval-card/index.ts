import type { ComponentExt, ViewState } from "../../api.ts";
import { agentName } from "../../format.ts";
import { approvalQueue } from "../../mode.ts";
import { approvalMoreLabel, approvalWaitingLabel } from "./logic.ts";

// The pulse is a ring outside the border whose opacity breathes. Opacity is composited, so the
// large card is painted once instead of on every frame (an animated box-shadow repaints it).
const STYLE = `
.approval { position: relative; width: 55vw; max-width: 120vmin; text-align: center; padding: 3.2vmin 4vmin 3.4vmin;
  border: 2px solid var(--attention, #ffb020); border-radius: 2vmin;
  background: color-mix(in srgb, color-mix(in srgb, var(--attention, #ffb020) 16%, #080604) calc(var(--card-opacity, 0.82) * 100%), transparent); }
.approval::before { content: ""; position: absolute; inset: -1.2vmin; border-radius: 3.2vmin; pointer-events: none;
  box-shadow: 0 0 0 0.6vmin color-mix(in srgb, var(--attention, #ffb020) 60%, transparent); opacity: 0; /* taste-skill-ignore: the pulse ring, a crisp spread with no blur, faded by opacity only */
  animation: approval-pulse 1.6s cubic-bezier(0.45, 0, 0.55, 1) infinite; will-change: opacity; }
.approval > div { overflow-wrap: anywhere; }
.approval .meta { font-size: 2.1vmin; color: color-mix(in srgb, var(--attention, #ffb020) 45%, #fff); font-variant-numeric: tabular-nums; }
.approval .headline { margin: 1vmin 0 1.6vmin; font-size: 6vmin; line-height: 1.05; font-weight: 650; letter-spacing: -0.02em;
  color: color-mix(in srgb, var(--attention, #ffb020) 70%, #fff); }
.approval .icon { display: inline-block; width: 0.82em; height: 0.82em; margin-inline-end: 0.22em; vertical-align: -0.06em; }
.approval .icon svg { display: block; width: 100%; height: 100%; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
.approval .where { font-size: 3vmin; font-weight: 500; color: #fff; }
/* taste-skill-ignore: the request is a literal command, so it is set as code */
.approval .request { margin-top: 0.8vmin; font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 2.5vmin; line-height: 1.35;
  color: rgba(255, 255, 255, 0.86); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; }
.approval .more { margin-top: 1.6vmin; font-size: 2.1vmin; font-weight: 600; color: var(--attention, #ffb020); font-variant-numeric: tabular-nums; }
@keyframes approval-pulse { 50% { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .approval::before { animation: none; opacity: 1; } }
`;

function line(className: string, text: string): HTMLElement {
  const element = document.createElement("div");
  element.className = className;
  element.textContent = text;
  // Project names and requests can be Hebrew or Arabic; let the text pick its own direction.
  element.dir = "auto";
  return element;
}

// A drawn icon instead of the "⚠" character, which each font draws differently (or as an emoji).
const WARNING_ICON = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3.2 22.2 20.6H1.8Z"/><path d="M12 9.6v4.8"/><path d="M12 17.6v.1"/></svg>`;

function warningIcon(): HTMLElement {
  const icon = document.createElement("span");
  icon.className = "icon";
  icon.innerHTML = WARNING_ICON;
  return icon;
}

function createApprovalCard(): ComponentExt {
  let box: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = STYLE;
    box = document.createElement("div");
    box.className = "approval";
    root.append(style, box);
  }

  function update(state: ViewState): void {
    const { first, more } = approvalQueue(state.cards);
    box?.toggleAttribute("hidden", first === undefined);
    if (first === undefined) return;
    const lines: Array<[string, string]> = [
      ["meta", `${agentName(first.source, state.agents)} · ${approvalWaitingLabel(first, state.nowMs)}`],
      ["headline", "Needs approval"],
      ["where", first.project ?? ""],
      ["request", first.request ?? ""],
      ["more", approvalMoreLabel(more)],
    ];
    // Privacy mode removes the request; skip empty lines.
    const shown = lines.filter(([, text]) => text !== "");
    const key = JSON.stringify(shown);
    if (key === lastKey) return;
    lastKey = key;
    box?.replaceChildren(...shown.map(([className, text]) => line(className, text)));
    box?.querySelector(".headline")?.prepend(warningIcon());
  }

  function unmount(): void {
    box?.parentElement?.replaceChildren();
    box = undefined;
  }

  return { mount, update, unmount };
}

window.agentScreensaver.register(createApprovalCard);
