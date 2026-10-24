// Lines that answer, from across the room: what state (in words and color) and which
// agent, what task (the conversation name or the prompt), how full the context is,
// and where (project, model, branch, background jobs). Every line is one line with an
// ellipsis, so a long name never grows the card; the last line sits at the bottom, so
// a card without a context bar still lines up with its neighbours.
export const PULSE_MS = 2400;
export const STYLE = `
.tasks { display: flex; gap: 1.6vmin; }
.task { --tone: var(--accent, #00e07a); flex: 1; min-width: 0; container: task / inline-size; background: rgba(10, 20, 14, var(--card-opacity, 0.82));
  border: 1px solid color-mix(in srgb, var(--tone) 30%, transparent); border-radius: 1.6vmin; padding: 2vmin 2.4vmin;
  display: flex; flex-direction: column; }
.task.done { --tone: var(--done, #78c8ff); }
.task.check { --tone: var(--attention, #ffb020); }
.task.quiet { --tone: rgba(255, 255, 255, 0.62); }
.task > div { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
/* The top line is the state, led by the agent's mark at a size read from across the
   room: its letter stands about as tall as the state's capitals. */
.task .top { display: flex; align-items: center; gap: 0.5em; font-size: 2.2vmin; line-height: 1.25; }
.task .agent { flex: none; display: block; width: 1.5em; height: 1.5em; }
.task .agent.source-mark { object-fit: contain; }
.task .agent.source-dot { width: 0.6em; height: 0.6em; margin: 0 0.45em; border-radius: 50%; background: rgba(255, 255, 255, 0.72); }
.task .status { flex: 1 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; font-weight: 600; color: var(--tone); font-variant-numeric: tabular-nums; }
.task .source-name { flex: 0 1 auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; font-size: 0.82em; color: rgba(255, 255, 255, 0.72); }
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
/* When the line is too long the branch gives way first, then the project shortens
   down to a few letters; the model shortens only after both. */
.task .where > .project { flex: 0 1000 auto; min-width: 2.5em; overflow: hidden; text-overflow: ellipsis; }
.task .where > .model { flex: 0 1 auto; min-width: 3em; overflow: hidden; text-overflow: ellipsis; }
.task .where > .branch { flex: 0 100000 auto; min-width: 3em; overflow: hidden; text-overflow: ellipsis; }
/* Narrow cards drop the least needed words first: the branch and the agent's name
   (its mark stays), then the word "Context", then on the narrowest cards the state's
   own mark (its color still tells the state). The agent's mark, the state, the model
   and the count are never dropped. */
@container task (max-width: 40vmin) { .task .where > .branch, .task .source-name { display: none; } }
/* A narrow card with background jobs has no room for a project worth reading next
   to the model and the count, so the project goes (a few letters would say nothing). */
@container task (max-width: 40vmin) { .task .where.counted > .project { display: none; } .task .where.counted > .project + span::before { content: none; } }
@container task (max-width: 33vmin) { .task .context .label { display: none; } }
@container task (max-width: 26vmin) { .task .status .mark { display: none; } }
.task .where > .background { color: var(--accent, #00e07a); font-weight: 600; }
@media (prefers-reduced-motion: reduce) { .task.working .mark { animation: none; } }
`;
