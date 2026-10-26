import type { Card, ViewState } from "../../api.ts";
import { loadFont } from "../shared/fonts.ts";
import { agentMark, createTaskRowComponent, stateMark, type TaskRowLook } from "../shared/tasks.ts";
import { taskSource, taskStatusParts } from "../task-row/logic.ts";

const FONT_FAMILY = "Chrome Tasks Mono";
const PULSE_MS = 2400;

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// The cards of builtin.task-row (same lines, order and narrow-card rules, from its
// logic), made of the scene's material: a smoked plate the grain and scan lines show
// through, a machined top edge in the state's color, the state and the times in the
// rings' monospace face, set apart in columns instead of joined by dots. The agent is
// named by its mark and its chrome sign, so the card leaves its name out. The title
// stays in the readable system face. The token block holds every color and face; the
// theme can set the shared ones (--accent, --attention, --done).
const STYLE = `
:root { --chrome-tasks-ink: #eef1f4; --chrome-tasks-quiet: rgba(238, 241, 244, 0.66); --chrome-tasks-faint: rgba(238, 241, 244, 0.16);
  --chrome-tasks-plate: rgba(10, 11, 13, 0.6); --chrome-tasks-sheen: rgba(238, 241, 244, 0.06); --chrome-tasks-lime: #c8ff2e;
  --chrome-tasks-pink: #ff3d9a; --chrome-tasks-done: #e6ebf0; --chrome-tasks-mono: "${FONT_FAMILY}", ui-monospace, Menlo, monospace; }
.tasks { display: flex; gap: 1.6vmin; }
.task { --tone: var(--accent, var(--chrome-tasks-lime)); position: relative; flex: 1; min-width: 0; container: task / inline-size;
  display: flex; flex-direction: column; padding: 2.1vmin 2.4vmin 1.9vmin; border-radius: 0.5vmin;
  background: linear-gradient(to bottom, var(--chrome-tasks-sheen), transparent 45%), var(--chrome-tasks-plate);
  border: 1px solid var(--chrome-tasks-faint); border-top: 0.4vmin solid var(--tone); }
.task.done { --tone: var(--done, var(--chrome-tasks-done)); }
.task.check { --tone: var(--attention, var(--chrome-tasks-pink)); }
.task.quiet { --tone: var(--chrome-tasks-quiet); }
.task > div { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.task .top { display: flex; align-items: center; gap: 0.7em; font-family: var(--chrome-tasks-mono); font-size: 2vmin; line-height: 1.25;
  font-variant-numeric: tabular-nums; }
.task .agent { flex: none; display: block; width: 1.6em; height: 1.6em; }
.task .agent.source-mark { object-fit: contain; }
.task .agent.source-dot { width: 0.6em; height: 0.6em; margin: 0 0.5em; border-radius: 50%; background: var(--chrome-tasks-quiet); }
.task .status { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--tone);
  font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; }
.task .since { flex: none; color: var(--chrome-tasks-ink); font-weight: 500; letter-spacing: 0.02em; }
.task .mark { display: inline-block; width: 0.8em; height: 0.8em; margin-inline-end: 0.55em; vertical-align: -0.06em; }
.task .mark svg { display: block; width: 100%; height: 100%; fill: none; stroke: currentColor; stroke-width: 2.8; stroke-linecap: round; stroke-linejoin: round; }
.task .mark .dot { fill: currentColor; stroke: none; }
.task.working .mark { animation: chrome-task-alive ${PULSE_MS}ms cubic-bezier(0.37, 0, 0.63, 1) infinite alternate; }
@keyframes chrome-task-alive { from { opacity: 1; } to { opacity: 0.3; } }
.task .heading { margin-top: 1.1vmin; font-size: 2.8vmin; line-height: 1.2; color: var(--chrome-tasks-ink); font-weight: 600; letter-spacing: -0.01em; }
.task .context { margin-top: 1.2vmin; display: flex; align-items: center; gap: 0.8em; font-family: var(--chrome-tasks-mono); font-size: 1.6vmin;
  line-height: 1.3; color: var(--chrome-tasks-quiet); font-variant-numeric: tabular-nums; }
.task .context .label { letter-spacing: 0.1em; text-transform: uppercase; }
.task .context .track { flex: 1 1 auto; min-width: 4em; height: 0.5vmin; background: var(--chrome-tasks-faint); overflow: hidden; }
.task .context .fill { display: block; height: 100%; background: var(--chrome-tasks-ink); }
.task .context .value { color: var(--chrome-tasks-ink); }
.task .context.high .fill { background: var(--attention, var(--chrome-tasks-pink)); }
.task .context.high .value { color: var(--attention, var(--chrome-tasks-pink)); font-weight: 700; }
.task .where { display: flex; gap: 1.3em; margin-top: auto; padding-top: 1.1vmin; font-family: var(--chrome-tasks-mono); font-size: 1.8vmin;
  line-height: 1.3; color: var(--chrome-tasks-quiet); font-variant-numeric: tabular-nums; }
.task .where > span { flex: none; white-space: nowrap; }
.task .where > .project { color: var(--chrome-tasks-ink); }
/* When the line is too long the branch gives way first, then the project shortens
   down to a few letters; the model shortens only after both. */
.task .where > .project { flex: 0 1000 auto; min-width: 2.5em; overflow: hidden; text-overflow: ellipsis; }
.task .where > .model { flex: 0 1 auto; min-width: 3em; overflow: hidden; text-overflow: ellipsis; }
.task .where > .branch { flex: 0 100000 auto; min-width: 3em; overflow: hidden; text-overflow: ellipsis; }
.task .where > .background { color: var(--accent, var(--chrome-tasks-lime)); font-weight: 700; }
/* Narrow cards drop the least needed words first, as the built-in task row does. */
@container task (max-width: 40vmin) { .task .where > .branch { display: none; } }
@container task (max-width: 40vmin) { .task .where.counted > .project { display: none; } }
@container task (max-width: 33vmin) { .task .context .label { display: none; } }
@container task (max-width: 26vmin) { .task .status .mark { display: none; } }
@media (prefers-reduced-motion: reduce) { .task.working .mark { animation: none; } }
`;

/** The agent's mark, the state in words, and at the far end how long it has been so. */
function topLine(card: Card, state: ViewState, look: TaskRowLook): HTMLElement {
  const parts = taskStatusParts(card, state.nowMs);
  const element = document.createElement("div");
  element.className = "top";
  element.dataset.source = card.source;
  const status = document.createElement("span");
  status.className = "status";
  status.textContent = parts.word;
  status.prepend(stateMark(parts.tone, state.nowMs, look.pulseMs));
  element.append(agentMark(taskSource(card, state.agents)), status);
  if (parts.since !== "") {
    const since = document.createElement("span");
    since.className = "since";
    since.textContent = parts.since;
    element.append(since);
  }
  return element;
}

function loadMono(): void {
  if (scriptSrc !== "") void loadFont(FONT_FAMILY, new URL("fonts/JetBrainsMono.woff2", scriptSrc).href, { weight: "100 800" });
}

window.agentScreensaver.register(() => createTaskRowComponent({ css: STYLE, pulseMs: PULSE_MS, topLine, onMount: loadMono }));
