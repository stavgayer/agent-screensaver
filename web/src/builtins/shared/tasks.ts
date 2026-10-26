import type { Card, ComponentExt, ViewState } from "../../api.ts";
import { taskCards, taskContext, taskHeading, taskSource, taskStatus, taskWhere, type TaskContext, type TaskTone } from "../task-row/logic.ts";

// Drawn marks instead of text glyphs, which each font draws differently. Working is a
// dot that breathes (it is alive), done a check, quiet an empty ring, check a solid dot.
export const MARKS: Record<TaskTone, string> = {
  working: `<svg viewBox="0 0 24 24" aria-hidden="true"><circle class="dot" cx="12" cy="12" r="6.5"/></svg>`,
  done: `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4.5 12.8 9.6 17.8 19.5 6.8"/></svg>`,
  quiet: `<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="6"/></svg>`,
  check: `<svg viewBox="0 0 24 24" aria-hidden="true"><circle class="dot" cx="12" cy="12" r="6.5"/></svg>`,
};

export function line(className: string, text: string): HTMLElement {
  const element = document.createElement("div");
  element.className = className;
  element.textContent = text;
  return element;
}

/** What makes one task row look different from another. */
export interface TaskRowLook {
  /** The component's stylesheet. */
  css: string;
  /** How long the working mark takes to breathe in, in ms; its delay follows the clock. */
  pulseMs: number;
  /** Optional. The card's first line; the default is the mark, the state in words and the agent's name. */
  topLine?(card: Card, state: ViewState, look: TaskRowLook): HTMLElement;
  /** Optional. Called once the row is on the page, for example to load a font. */
  onMount?(): void;
}

/** The agent's mark, the state, and on wide cards the agent's name at the far end. */
function defaultTopLine(card: Card, state: ViewState, look: TaskRowLook): HTMLElement {
  const source = taskSource(card, state.agents);
  const element = document.createElement("div");
  element.className = "top";
  element.dataset.source = card.source;
  const name = document.createElement("span");
  name.className = "source-name";
  name.textContent = source.name;
  element.append(agentMark(source), statusLine(card, state.nowMs, look.pulseMs), name);
  return element;
}

/**
 * The mark of the agent's extension, or a neutral dot for a source without one. The
 * mark is an <img>, so an SVG can run no code and load nothing (the service checked
 * it too). Its alt text names the agent, because on narrow cards the name is hidden.
 */
export function agentMark(source: { markUrl: string | undefined; name: string }): HTMLElement {
  if (source.markUrl === undefined) {
    const dot = document.createElement("span");
    dot.className = "agent source-dot";
    dot.setAttribute("aria-hidden", "true");
    return dot;
  }
  const image = document.createElement("img");
  image.className = "agent source-mark";
  image.src = source.markUrl;
  image.alt = source.name;
  return image;
}

/** A labelled bar whose fill is the true share of the window in use. */
export function contextLine(context: TaskContext): HTMLElement {
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

/** The drawn mark of a state. */
export function stateMark(tone: TaskTone, nowMs: number, pulseMs: number): HTMLElement {
  const mark = document.createElement("span");
  mark.className = "mark";
  mark.innerHTML = MARKS[tone];
  // The row is redrawn every minute; a delay tied to the clock keeps the breathing smooth across redraws.
  mark.style.animationDelay = `-${nowMs % (2 * pulseMs)}ms`;
  return mark;
}

function statusLine(card: Card, nowMs: number, pulseMs: number): HTMLElement {
  const status = taskStatus(card, nowMs);
  const element = document.createElement("span");
  element.className = "status";
  element.textContent = status.text;
  element.prepend(stateMark(status.tone, nowMs, pulseMs));
  return element;
}

/**
 * Project, model, branch and background jobs. When the line is too long the branch
 * gives way first, then the project down to a few letters, and the model last; on
 * narrow cards the branch is left out, and with a count the project too (see STYLE). The count is never cut. Each part is isolated, so a
 * Hebrew project name keeps its own direction.
 */
export function whereLine(card: Card): HTMLElement {
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

function createTask(card: Card, state: ViewState, look: TaskRowLook): HTMLElement {
  const element = document.createElement("div");
  element.className = `task ${taskStatus(card, state.nowMs).tone}`;
  const heading = line("heading", taskHeading(card));
  heading.dir = "auto";
  element.append((look.topLine ?? defaultTopLine)(card, state, look), heading);
  const context = taskContext(card);
  if (context !== undefined) element.append(contextLine(context));
  element.append(whereLine(card));
  return element;
}

/**
 * A component with one card per task, in the order of the cards, with the lines of
 * task-row/logic.ts. The built-in task row and the chrome tasks share it and differ
 * only in their look.
 */
export function createTaskRowComponent(look: TaskRowLook): ComponentExt {
  let row: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = look.css;
    row = document.createElement("div");
    row.className = "tasks";
    root.append(style, row);
    look.onMount?.();
  }

  function update(state: ViewState): void {
    const cards = taskCards(state.cards);
    const key = JSON.stringify(cards.map((card) => [card.source, card.session, taskStatus(card, state.nowMs), taskSource(card, state.agents), taskHeading(card), taskContext(card), taskWhere(card)]));
    if (key === lastKey) return;
    lastKey = key;
    row?.replaceChildren(...cards.map((card) => createTask(card, state, look)));
  }

  function unmount(): void {
    row?.parentElement?.replaceChildren();
    row = undefined;
  }

  return { mount, update, unmount };
}
