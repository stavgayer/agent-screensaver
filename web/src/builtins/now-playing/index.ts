import type { Ctx, NowPlaying, ViewState } from "../../api.ts";
import { songView } from "./logic.ts";

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

// The bar moves 1/34 of the widget's width in a 3-minute song every ~5 s, so a few frames a second look smooth.
const BAR_FPS = 4;

window.agentScreensaver.register(() => {
  let box: HTMLElement | undefined;
  let song: HTMLElement | undefined;
  let artist: HTMLElement | undefined;
  let bar: HTMLElement | undefined;
  let fill: HTMLElement | undefined;
  let track: NowPlaying | undefined;
  let cancelFrame: (() => void) | undefined;
  let now = (): number => Date.now();

  const setText = (element: HTMLElement | undefined, text: string): void => {
    if (element !== undefined && element.textContent !== text) element.textContent = text;
  };

  const draw = (nowMs: number): void => {
    const view = songView(track, nowMs);
    box?.classList.toggle("hidden", view === null);
    if (view === null) return;
    setText(song, view.title);
    setText(artist, view.artist);
    if (artist !== undefined) artist.hidden = view.artist === "";
    if (bar !== undefined) bar.hidden = view.progress === null;
    if (fill !== undefined && view.progress !== null) fill.style.transform = `scaleX(${view.progress})`;
  };

  function mount(root: HTMLElement, ctx: Ctx): void {
    const style = document.createElement("style");
    style.textContent = STYLE;
    const element = (className: string): HTMLElement => {
      const created = document.createElement("div");
      created.className = className;
      return created;
    };
    box = element("now-playing hidden");
    song = element("song");
    artist = element("artist");
    bar = element("bar");
    fill = element("fill");
    bar.append(fill);
    box.append(song, artist, bar);
    root.append(style, box);
    now = () => ctx.now();
    cancelFrame = ctx.requestFrame(() => draw(now()), BAR_FPS);
  }

  function update(state: ViewState): void {
    track = state.nowPlaying;
    draw(now());
  }

  function unmount(): void {
    cancelFrame?.();
    cancelFrame = undefined;
    box?.parentElement?.replaceChildren();
    box = song = artist = bar = fill = undefined;
  }

  return { mount, update, unmount };
});
