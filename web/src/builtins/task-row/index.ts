import type { Card, ComponentExt, ViewState } from "../../api.ts";
import { taskCards, taskContext, taskHeading, taskSource, taskStatus, taskWhere, type TaskContext, type TaskTone } from "./logic.ts";

// Lines that answer, from across the room: what state (in words and color) and which
// agent, what task (the conversation name or the prompt), how full the context is,
// and where (project, model, branch, background jobs). Every line is one line with an
// ellipsis, so a long name never grows the card; the last line sits at the bottom, so
// a card without a context bar still lines up with its neighbours.
const PULSE_MS = 2400;
const STYLE = `
.tasks { display: flex; gap: 1.6vmin; }
.task { --tone: var(--accent, #00e07a); flex: 1; min-width: 0; container: task / inline-size; background: rgba(10, 20, 14, var(--card-opacity, 0.82));
  border: 1px solid color-mix(in srgb, var(--tone) 30%, transparent); border-radius: 1.6vmin; padding: 2vmin 2.4vmin;
  display: flex; flex-direction: column; }
.task.done { --tone: var(--done, #78c8ff); }
.task.check { --tone: var(--attention, #ffb020); }
.task.quiet { --tone: rgba(255, 255, 255, 0.62); }
.task > div { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.task .top { display: flex; align-items: baseline; gap: 1.2vmin; }
.task .source { flex: none; display: flex; align-items: center; gap: 0.45em; font-size: 1.8vmin; line-height: 1.25; color: rgba(255, 255, 255, 0.72); }
.task .status { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; font-size: 2.2vmin; line-height: 1.25; font-weight: 600; color: var(--tone); font-variant-numeric: tabular-nums; }
.task .mark { display: inline-block; width: 0.9em; height: 0.9em; margin-inline-end: 0.4em; vertical-align: -0.1em; }
.task .mark svg { display: block; width: 100%; height: 100%; fill: none; stroke: currentColor; stroke-width: 2.6; stroke-linecap: round; stroke-linejoin: round; }
.task .mark .dot { fill: currentColor; stroke: none; }
.task.working .mark { animation: task-alive ${PULSE_MS}ms cubic-bezier(0.37, 0, 0.63, 1) infinite alternate; }
@keyframes task-alive { from { opacity: 1; } to { opacity: 0.35; } }
.task .heading { margin-top: 0.7vmin; font-size: 2.6vmin; line-height: 1.25; color: #fff; font-weight: 600; letter-spacing: -0.005em; }
.task .context { margin-top: 1vmin; display: flex; align-items: center; gap: 0.6em; font-size: 1.7vmin; line-height: 1.3;
  color: rgba(255, 255, 255, 0.72); font-variant-numeric: tabular-nums; }
.task .context .track { flex: 1 1 auto; min-width: 4em; height: 0.7vmin; border-radius: 0.35vmin; background: rgba(255, 255, 255, 0.14); overflow: hidden; }
.task .context .fill { display: block; height: 100%; border-radius: inherit; background: rgba(255, 255, 255, 0.62); }
.task .context.high .fill { background: var(--attention, #ffb020); }
.task .context.high .value { color: var(--attention, #ffb020); font-weight: 600; }
.task .where { margin-top: auto; padding-top: 0.7vmin; font-size: 1.9vmin; line-height: 1.3; color: rgba(255, 255, 255, 0.72); font-variant-numeric: tabular-nums; }
.task .where { display: flex; }
.task .where > span { flex: none; white-space: nowrap; }
.task .where > span + span::before { content: "·"; margin: 0 0.4em; color: rgba(255, 255, 255, 0.72); font-weight: 400; }
.task .where > .project { flex: 0 1 auto; min-width: 3.5em; overflow: hidden; text-overflow: ellipsis; }
/* The branch, then the model, give way long before the project does. */
.task .where > .model { flex: 0 100 auto; min-width: 3em; overflow: hidden; text-overflow: ellipsis; }
.task .where > .branch { flex: 0 1000 auto; min-width: 3em; overflow: hidden; text-overflow: ellipsis; }
/* Narrow cards drop the least needed words first: the branch, then the model, the
   word "Context" and the agent's name (its mark stays), then on the narrowest cards
   the mark too. The state and the count are never cut for them. */
@container task (max-width: 40vmin) { .task .where > .branch { display: none; } }
@container task (max-width: 33vmin) { .task .where > .model, .task .context .label, .task .source-name { display: none; } }
@container task (max-width: 26vmin) { .task .source { display: none; } }
.task .where > .background { color: var(--accent, #00e07a); font-weight: 600; }
@media (prefers-reduced-motion: reduce) { .task.working .mark { animation: none; } }
.task .source-mark { flex: none; width: 1.1em; height: 1.1em; object-fit: contain; }
.task .source-dot { flex: none; width: 0.55em; height: 0.55em; margin: 0 0.275em; border-radius: 50%; background: currentColor; opacity: 0.7; }
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

function topLine(card: Card, state: ViewState): HTMLElement {
  const element = document.createElement("div");
  element.className = "top";
  element.append(statusLine(card, state.nowMs), sourceLabel(card, state));
  return element;
}

/**
 * The agent: the mark of its extension and its name, or a neutral dot and the source
 * name for a source without one. The mark is an <img>, so an SVG can run no code and
 * load nothing (the service checked it too).
 */
function sourceLabel(card: Card, state: ViewState): HTMLElement {
  const source = taskSource(card, state.agents);
  const element = document.createElement("span");
  element.className = "source";
  element.dataset.source = card.source;
  let mark: HTMLElement;
  if (source.markUrl === undefined) {
    mark = document.createElement("span");
    mark.className = "source-dot";
  } else {
    const image = document.createElement("img");
    image.className = "source-mark";
    image.src = source.markUrl;
    image.alt = "";
    mark = image;
  }
  mark.setAttribute("aria-hidden", "true");
  const name = document.createElement("span");
  name.className = "source-name";
  name.textContent = source.name;
  element.append(mark, name);
  return element;
}

/** A labelled bar whose fill is the true share of the window in use. */
function contextLine(context: TaskContext): HTMLElement {
  const element = line(context.high ? "context high" : "context", "");
  const label = document.createElement("span");
  label.className = "label";
  label.textContent = "Context";
  const track = document.createElement("span");
  track.className = "track";
  const fill = document.createElement("span");
  fill.className = "fill";
  fill.style.width = `${context.percent}%`;
  track.append(fill);
  const value = document.createElement("span");
  value.className = "value";
  value.textContent = context.text;
  element.append(label, track, value);
  return element;
}

function statusLine(card: Card, nowMs: number): HTMLElement {
  const status = taskStatus(card, nowMs);
  const element = document.createElement("span");
  element.className = "status";
  element.textContent = status.text;
  const mark = document.createElement("span");
  mark.className = "mark";
  mark.innerHTML = MARKS[status.tone];
  // The row is redrawn every minute; a delay tied to the clock keeps the breathing smooth across redraws.
  mark.style.animationDelay = `-${nowMs % (2 * PULSE_MS)}ms`;
  element.prepend(mark);
  return element;
}

/**
 * Project, model, branch and background jobs. When the line is too long the project
 * and the branch give way down to a few letters; on narrow cards the branch, then the
 * model, are left out (see STYLE). The count is never cut. Each part is isolated, so a
 * Hebrew project name keeps its own direction.
 */
function whereLine(card: Card): HTMLElement {
  const where = taskWhere(card);
  const element = line("where", "");
  for (const part of where.parts) {
    const span = document.createElement("span");
    span.textContent = part.text;
    span.dir = "auto";
    span.className = part.role;
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

function createTask(card: Card, state: ViewState): HTMLElement {
  const element = document.createElement("div");
  element.className = `task ${taskStatus(card, state.nowMs).tone}`;
  const heading = line("heading", taskHeading(card));
  heading.dir = "auto";
  element.append(topLine(card, state), heading);
  const context = taskContext(card);
  if (context !== undefined) element.append(contextLine(context));
  element.append(whereLine(card));
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
    const key = JSON.stringify(cards.map((card) => [card.source, card.session, taskStatus(card, state.nowMs), taskSource(card, state.agents), taskHeading(card), taskContext(card), taskWhere(card)]));
    if (key === lastKey) return;
    lastKey = key;
    row?.replaceChildren(...cards.map((card) => createTask(card, state)));
  }

  function unmount(): void {
    row?.parentElement?.replaceChildren();
    row = undefined;
  }

  return { mount, update, unmount };
}

window.agentScreensaver.register(createTaskRow);
