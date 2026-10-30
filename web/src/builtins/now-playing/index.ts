import { createNowPlaying } from "../shared/now-playing.ts";

// Set like the ring captions: two lines that never wrap, the artist in a quieter tone, the same dark halo over the
// background. The bar fills with a transform only, a few times a second; a song that starts fades in, with no movement.
const STYLE = `
.now-playing { width: 34vmin; opacity: 1; transition: opacity 200ms cubic-bezier(0.23, 1, 0.32, 1); }
.now-playing.hidden { opacity: 0; }
.now-playing .song, .now-playing .artist { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; line-height: 1.25;
  text-shadow: 0 0 0.8vmin #000, 0 0 0.3vmin #000; } /* taste-skill-ignore: a static dark halo keeps the text legible over the rain */
.now-playing .song { font-size: 2.3vmin; font-weight: 500; color: rgba(255, 255, 255, 0.9); }
.now-playing .artist { margin-top: 0.3vmin; font-size: 1.9vmin; color: rgba(255, 255, 255, 0.68); }
.now-playing .bar { margin-top: 1.2vmin; height: 0.4vmin; border-radius: 0.2vmin; overflow: hidden; background: rgba(255, 255, 255, 0.16); }
.now-playing .fill { height: 100%; background: var(--accent, #00e07a); transform-origin: left center; transform: scaleX(0); }
.now-playing .bar[hidden], .now-playing .artist[hidden] { display: none; }
@media (prefers-reduced-motion: reduce) { .now-playing { transition: none; } }
`;

window.agentScreensaver.register(() => createNowPlaying({ css: STYLE, className: "now-playing" }));
