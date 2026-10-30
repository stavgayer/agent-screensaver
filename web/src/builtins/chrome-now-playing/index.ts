import { loadFont } from "../shared/fonts.ts";
import { createNowPlaying } from "../shared/now-playing.ts";

const FONT_FAMILY = "Chrome Now Playing Mono";

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// The song as the Chrome rings set their captions: the song in capitals, the artist in the
// lime-tinted tone of a reset time, both in the bundled monospace face with the same dark
// halo. The bar is the thin track of the task cards' context bar. It stands under the
// clock, whose date hangs below the time out of flow, so its top margin clears the date.
// The token block holds every color and face; the theme can set --accent.
const STYLE = `
:root { --chrome-np-lime: #c8ff2e; --chrome-np-ink: #f4f6f8; --chrome-np-steel: #c9ced4; --chrome-np-faint: rgba(255, 255, 255, 0.16);
  --chrome-np-shade: #000; --chrome-np-mono: "${FONT_FAMILY}", ui-monospace, Menlo, monospace; }
.chrome-now-playing { width: 36vmin; margin-top: 4vmin; padding-left: 0.5vmin; font-family: var(--chrome-np-mono);
  opacity: 1; transition: opacity 200ms cubic-bezier(0.23, 1, 0.32, 1); }
.chrome-now-playing.hidden { opacity: 0; }
.chrome-now-playing .song, .chrome-now-playing .artist { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; line-height: 1.25;
  text-shadow: 0 0 0.8vmin var(--chrome-np-shade), 0 0 0.3vmin var(--chrome-np-shade); } /* taste-skill-ignore: a static dark halo keeps captions legible over the scene */
.chrome-now-playing .song { font-size: 2vmin; font-weight: 700; letter-spacing: 0.06em; text-transform: uppercase; color: var(--chrome-np-ink); }
.chrome-now-playing .artist { margin-top: 0.4vmin; font-size: 1.85vmin; font-weight: 500;
  color: color-mix(in srgb, var(--accent, var(--chrome-np-lime)) 55%, var(--chrome-np-steel)); }
.chrome-now-playing .bar { margin-top: 1.2vmin; height: 0.5vmin; overflow: hidden; background: var(--chrome-np-faint); }
.chrome-now-playing .fill { height: 100%; background: var(--chrome-np-ink); transform-origin: left center; transform: scaleX(0); }
.chrome-now-playing .bar[hidden], .chrome-now-playing .artist[hidden] { display: none; }
@media (prefers-reduced-motion: reduce) { .chrome-now-playing { transition: none; } }
`;

function loadMono(): void {
  if (scriptSrc !== "") void loadFont(FONT_FAMILY, new URL("fonts/JetBrainsMono.woff2", scriptSrc).href, { weight: "100 800" });
}

window.agentScreensaver.register(() => createNowPlaying({ css: STYLE, className: "chrome-now-playing", onMount: loadMono }));
