import type { Card, ComponentExt, ViewState } from "../../api.ts";
import { taskCards, taskHeading, taskStatus, taskWhere, type TaskTone } from "./logic.ts";

// Three lines that answer, from across the room: what state (in words and color),
// what task (the title) and where (project, agent, background jobs). Every line is
// one line with an ellipsis, so a long title never grows the card or shifts the row.
const PULSE_MS = 2400;
const STYLE = `
.tasks { display: flex; gap: 1.6vmin; }
.task { --tone: var(--accent, #00e07a); flex: 1; min-width: 0; container: task / inline-size; background: rgba(10, 20, 14, var(--card-opacity, 0.82));
  border: 1px solid color-mix(in srgb, var(--tone) 30%, transparent); border-radius: 1.6vmin; padding: 2vmin 2.4vmin; }
.task.done { --tone: var(--done, #78c8ff); }
.task.check { --tone: var(--attention, #ffb020); }
.task.quiet { --tone: rgba(255, 255, 255, 0.62); }
.task > div { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.task .status { font-size: 2.2vmin; line-height: 1.25; font-weight: 600; color: var(--tone); font-variant-numeric: tabular-nums; }
.task .mark { display: inline-block; width: 0.9em; height: 0.9em; margin-inline-end: 0.4em; vertical-align: -0.1em; }
.task .mark svg { display: block; width: 100%; height: 100%; fill: none; stroke: currentColor; stroke-width: 2.6; stroke-linecap: round; stroke-linejoin: round; }
.task .mark .dot { fill: currentColor; stroke: none; }
.task.working .mark { animation: task-alive ${PULSE_MS}ms cubic-bezier(0.37, 0, 0.63, 1) infinite alternate; }
@keyframes task-alive { from { opacity: 1; } to { opacity: 0.35; } }
.task .heading { margin-top: 0.7vmin; font-size: 2.6vmin; line-height: 1.25; color: #fff; font-weight: 600; letter-spacing: -0.005em; }
.task .where { margin-top: 0.5vmin; font-size: 1.9vmin; line-height: 1.3; color: rgba(255, 255, 255, 0.72); font-variant-numeric: tabular-nums; }
.task .where { display: flex; }
.task .where > span { flex: none; white-space: nowrap; }
.task .where > span + span::before { content: "·"; margin: 0 0.4em; color: rgba(255, 255, 255, 0.72); font-weight: 400; }
.task .where > .project { flex: 0 1 auto; min-width: 3.5em; overflow: hidden; text-overflow: ellipsis; }
@container task (max-width: 33vmin) { .task .where.counted > .agent { display: none; } }
.task .where > .background { color: var(--accent, #00e07a); font-weight: 600; }
@media (prefers-reduced-motion: reduce) { .task.working .mark { animation: none; } }
`;

// Drawn marks instead of text glyphs, which each font draws differently. Working is a
// dot that breathes (it is alive), done a check, quiet an empty ring, check a solid dot.
const MARKS: Record<TaskTone, string> = {
  working: `<svg viewBox="0 0 24 24" aria-hidden="true"><circle class="dot" cx="12" cy="12" r="6.5"/></svg>`,
  done: `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4.5 12.8 9.6 17.8 19.5 6.8"/></svg>`,
  quiet: `<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="6"/></svg>`,
  check: `<svg viewBox="0 0 24 24" aria-hidden="true"><circle class="dot" cx="12" cy="12" r="6.5"/></svg>`,
};

function line(className: string, text: string): HTMLElement {
  const element = document.createElement("div");
  element.className = className;
  element.textContent = text;
  return element;
}

function statusLine(card: Card, nowMs: number): HTMLElement {
  const status = taskStatus(card, nowMs);
  const element = line("status", status.text);
  const mark = document.createElement("span");
  mark.className = "mark";
  mark.innerHTML = MARKS[status.tone];
  // The row is redrawn every minute; a delay tied to the clock keeps the breathing smooth across redraws.
  mark.style.animationDelay = `-${nowMs % (2 * PULSE_MS)}ms`;
  element.prepend(mark);
  return element;
}

/**
 * Project, agent and background jobs. When the line is too long the project gives way
 * down to a few letters, and on a narrow card with a count the agent is left out; the
 * agent and the count are never cut. Each part is isolated, so a Hebrew
 * project name keeps its own direction.
 */
function whereLine(card: Card): HTMLElement {
  const where = taskWhere(card);
  const element = line("where", "");
  for (const part of where.parts) {
    const span = document.createElement("span");
    span.textContent = part;
    span.dir = "auto";
    span.className = part === card.project ? "project" : "agent";
    element.append(span);
  }
  if (where.background !== "") {
    element.classList.add("counted");
    const span = document.createElement("span");
    span.className = "background";
    span.textContent = where.background;
    element.append(span);
  }
  return element;
}

function createTask(card: Card, nowMs: number): HTMLElement {
  const element = document.createElement("div");
  element.className = `task ${taskStatus(card, nowMs).tone}`;
  const heading = line("heading", taskHeading(card));
  heading.dir = "auto";
  element.append(statusLine(card, nowMs), heading, whereLine(card));
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
    const key = JSON.stringify(cards.map((card) => [card.source, card.session, taskStatus(card, state.nowMs), taskHeading(card), taskWhere(card)]));
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
