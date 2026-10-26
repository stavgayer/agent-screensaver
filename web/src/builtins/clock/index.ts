import { createMinuteClock } from "../shared/clock.ts";

// The date hangs below the time (out of flow), so the box is the time alone in both sizes.
// The runtime's move then scales width and height by the same amount and the digits do not squash.
// Sizes change with no transition: the runtime animates the move with a transform.
const STYLE = `
.clock { position: relative; text-align: center; color: color-mix(in srgb, var(--accent, #00e07a) 12%, #fff); font-variant-numeric: tabular-nums;
  text-shadow: 0 0 2vmin #000, 0 0 0.6vmin #000; } /* taste-skill-ignore: a static dark halo separates the digits from the rain */
.clock .time { font-weight: 300; line-height: 1; letter-spacing: -0.02em; font-size: 16vmin; }
.clock .date { position: absolute; top: 100%; left: 50%; transform: translateX(-50%); white-space: nowrap; margin-top: 1.2vmin;
  color: color-mix(in srgb, var(--accent, #00e07a) 30%, #fff); opacity: 0.85; font-size: 3vmin; font-weight: 400; transition: opacity 200ms ease; }
.clock.small .time { font-size: 5vmin; }
.clock.small .date { opacity: 0; }
@media (prefers-reduced-motion: reduce) { .clock .date { transition: none; } }
`;

window.agentScreensaver.register(() => createMinuteClock({ css: STYLE, className: "clock" }));
