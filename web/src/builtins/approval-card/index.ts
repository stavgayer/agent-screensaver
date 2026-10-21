import type { ComponentExt, ViewState } from "../../api.ts";
import { displayName } from "../../format.ts";
import { approvalQueue } from "../../mode.ts";
import { approvalMoreLabel, approvalWaitingLabel } from "./logic.ts";

const STYLE = `
.approval { width: 55vw; text-align: center; padding: 2.6vmin; border: 2px solid var(--attention, #ffb020); border-radius: 1.6vmin; background: rgba(60, 35, 0, var(--card-opacity, 0.82));
  box-shadow: 0 0 18px rgba(255, 176, 32, 0.7); animation: approval-pulse 1.6s ease-in-out infinite; }
.approval .meta { font-size: 1.8vmin; opacity: 0.65; text-transform: uppercase; letter-spacing: 0.06em; }
.approval .headline { color: #ffd27a; font-size: 4vmin; font-weight: 600; margin: 0.6vmin 0; }
.approval .where { font-size: 2.8vmin; overflow-wrap: anywhere; }
.approval .request { font-size: 2.8vmin; color: #fff; overflow-wrap: anywhere; }
.approval .more { margin-top: 1.2vmin; font-size: 2vmin; color: var(--attention, #ffb020); }
@keyframes approval-pulse { 50% { box-shadow: 0 0 34px rgba(255, 176, 32, 1); } }
@media (prefers-reduced-motion: reduce) { .approval { animation: none; } }
`;

function line(className: string, text: string): HTMLElement {
  const element = document.createElement("div");
  element.className = className;
  element.textContent = text;
  return element;
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
      ["meta", `${displayName(first.source)} · ${approvalWaitingLabel(first, state.nowMs)}`],
      ["headline", "⚠ Needs approval"],
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
  }

  function unmount(): void {
    box?.parentElement?.replaceChildren();
    box = undefined;
  }

  return { mount, update, unmount };
}

window.agentScreensaver.register(createApprovalCard);
