import type { ComponentExt, Ctx, NowPlaying, ViewState } from "../../api.ts";
import { songView } from "./song.ts";

/** What makes one now-playing look different from another. */
export interface NowPlayingLook {
  /** The component's stylesheet. */
  css: string;
  /** Class of the box. Inside it are `.song`, `.artist` and `.bar` with its `.fill`; `.hidden` marks the box while no song plays. */
  className: string;
  /** Optional. Called once the box is on the page, for example to load a font. */
  onMount?(box: HTMLElement): void;
}

// The bar moves 1/34 of the widget's width in a 3-minute song every ~5 s, so a few frames a second look smooth.
const BAR_FPS = 4;

/**
 * A component that shows the song a music app plays: the song, its artist and
 * a progress bar that moves on between the service's reads. It hides itself
 * while no song plays; the artist line goes when there is no artist, and the
 * bar when the app gives no length. The built-in widget and the Chrome one
 * share it and differ only in their look.
 */
export function createNowPlaying(look: NowPlayingLook): ComponentExt {
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
    style.textContent = look.css;
    const element = (className: string): HTMLElement => {
      const created = document.createElement("div");
      created.className = className;
      return created;
    };
    box = element(`${look.className} hidden`);
    song = element("song");
    artist = element("artist");
    bar = element("bar");
    fill = element("fill");
    bar.append(fill);
    box.append(song, artist, bar);
    root.append(style, box);
    look.onMount?.(box);
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
}
