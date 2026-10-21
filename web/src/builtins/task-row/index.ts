import type { Card, ComponentExt, ViewState } from "../../api.ts";
import { displayName } from "../../format.ts";
import { taskCards, taskLabel } from "./logic.ts";

// Every line is one line with an ellipsis, so a long title never grows the card or shifts the row.
const STYLE = `
.tasks { display: flex; gap: 1.6vmin; }
.task { flex: 1; min-width: 0; background: rgba(10, 20, 14, var(--card-opacity, 0.82)); border: 1px solid color-mix(in srgb, var(--accent, #00e07a) 25%, transparent);
  border-radius: 1.6vmin; padding: 2vmin 2.4vmin; }
.task.finished { border-color: color-mix(in srgb, var(--done, #78c8ff) 50%, transparent); }
.task.finished .heading { color: var(--done, #78c8ff); }
.task > div { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.task .meta { font-size: 1.8vmin; line-height: 1.3; color: rgba(255, 255, 255, 0.68); font-variant-numeric: tabular-nums; }
.task .heading { margin-top: 0.6vmin; font-size: 2.6vmin; line-height: 1.25; color: #fff; font-weight: 600; letter-spacing: -0.005em; }
.task .icon { display: inline-block; width: 0.9em; height: 0.9em; margin-inline-end: 0.3em; vertical-align: -0.1em; }
.task .icon svg { display: block; width: 100%; height: 100%; fill: none; stroke: currentColor; stroke-width: 2.6; stroke-linecap: round; stroke-linejoin: round; }
.task .project { margin-top: 0.4vmin; font-size: 1.9vmin; line-height: 1.3; color: rgba(255, 255, 255, 0.72); }
`;

// A drawn check instead of the "✓" character, which each font draws differently.
const CHECK_ICON = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4.5 12.8 9.6 17.8 19.5 6.8"/></svg>`;

/** The bold line of a card. In privacy mode there is no title, so the project takes its place. */
function headingOf(card: Card): HTMLElement {
  const heading = document.createElement("div");
  heading.className = "heading";
  heading.textContent = card.title ?? card.project ?? "";
  heading.dir = "auto";
  if (card.kind === "finished" && heading.textContent !== "") {
    const icon = document.createElement("span");
    icon.className = "icon";
    icon.innerHTML = CHECK_ICON;
    heading.prepend(icon);
  }
  return heading;
}

function createTask(card: Card, nowMs: number): HTMLElement {
  const element = document.createElement("div");
  element.className = `task ${card.kind}`;
  const meta = document.createElement("div");
  meta.className = "meta";
  meta.textContent = `${displayName(card.source)} · ${taskLabel(card, nowMs)}`;
  element.append(meta, headingOf(card));
  if (card.title !== undefined && card.project !== undefined) {
    const project = document.createElement("div");
    project.className = "project";
    project.textContent = card.project;
    project.dir = "auto";
    element.appendChild(project);
  }
  return element;
}

function createTaskRow(): ComponentExt {
  let row: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = STYLE;
    row = document.createElement("div");
    row.className = "tasks";
    root.append(style, row);
  }

  function update(state: ViewState): void {
    const cards = taskCards(state.cards);
    const key = JSON.stringify(cards.map((card) => [card.source, card.session, card.kind, card.project, card.title, taskLabel(card, state.nowMs)]));
    if (key === lastKey) return;
    lastKey = key;
    row?.replaceChildren(...cards.map((card) => createTask(card, state.nowMs)));
  }

  function unmount(): void {
    row?.parentElement?.replaceChildren();
    row = undefined;
  }

  return { mount, update, unmount };
}

window.agentScreensaver.register(createTaskRow);
