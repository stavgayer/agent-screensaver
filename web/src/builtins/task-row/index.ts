import type { Card, ComponentExt, ViewState } from "../../api.ts";
import { displayName } from "../../format.ts";
import { taskCards, taskLabel } from "./logic.ts";

const STYLE = `
.tasks { display: flex; gap: 1.4vmin; }
.task { flex: 1; min-width: 0; background: rgba(10, 20, 14, var(--card-opacity, 0.82)); border: 1px solid rgba(0, 255, 120, 0.25); border-radius: 1.6vmin; padding: 1.6vmin 2vmin; line-height: 1.35; font-size: 2.2vmin; }
.task.finished { border-color: rgba(120, 200, 255, 0.5); }
.task .meta { font-size: 1.7vmin; opacity: 0.65; text-transform: uppercase; letter-spacing: 0.06em; }
.task .heading { color: #fff; font-weight: 600; overflow-wrap: anywhere; }
.task .project { opacity: 0.7; font-size: 1.8vmin; }
`;

/** The bold line of a card. In privacy mode there is no title, so the project takes its place. */
function headingOf(card: Card): string {
  const text = card.title ?? card.project ?? "";
  return card.kind === "finished" && text !== "" ? `✓ ${text}` : text;
}

function createTask(card: Card, nowMs: number): HTMLElement {
  const element = document.createElement("div");
  element.className = `task ${card.kind}`;
  const meta = document.createElement("div");
  meta.className = "meta";
  meta.textContent = `${displayName(card.source)} · ${taskLabel(card, nowMs)}`;
  const heading = document.createElement("div");
  heading.className = "heading";
  heading.textContent = headingOf(card);
  element.append(meta, heading);
  if (card.title !== undefined && card.project !== undefined) {
    const project = document.createElement("div");
    project.className = "project";
    project.textContent = card.project;
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
