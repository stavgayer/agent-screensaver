import type { BackgroundExt, Ctx } from "../../api.ts";
import { createStage, dimOption, fitOption, type Fit } from "../shared/media.ts";
import { createPlaylist, fileList, type Playlist } from "./logic.ts";

/**
 * Apple aerials that macOS already downloaded. The service passes clones of
 * them as `files`; this plays them in a shuffled order, one at a time, and
 * crossfades from one clip into the next. With no files the service shows
 * Matrix instead, so an empty list here only happens when Matrix is missing:
 * then the screen stays black.
 *
 * Videos are muted and pause while the page is hidden. A clip that has
 * faded out is unloaded at once, because a 4K HEVC decoder holds hundreds of
 * megabytes (docs/decisions/0002-video-cost.md).
 */

/**
 * Long enough to read as one shot dissolving into the next, which is how
 * macOS moves between aerials; seen once every few minutes, so it never
 * feels slow. Opacity only, with a gentle in-out curve: both clips are on
 * screen and moving, so neither end of the dissolve should snap.
 */
const CROSSFADE_MS = 1200;
const CROSSFADE = `opacity ${CROSSFADE_MS}ms cubic-bezier(0.45, 0, 0.55, 1)`;
/** The next clip starts loading this long before the current one ends, so the dissolve overlaps motion. */
const SWITCH_BEFORE_END_S = 3;

function createVideo(fit: Fit, loop: boolean): HTMLVideoElement {
  const video = document.createElement("video");
  video.muted = true;
  video.defaultMuted = true; // the `muted` attribute, which autoplay policies read
  video.loop = loop;
  video.playsInline = true;
  video.preload = "auto";
  video.disablePictureInPicture = true;
  video.style.cssText =
    `position:absolute;inset:0;width:100%;height:100%;object-fit:${fit};opacity:0;transition:${CROSSFADE}`;
  return video;
}

/** Removing the source and calling load() is what frees the decoder. */
function release(video: HTMLVideoElement): void {
  video.pause();
  video.removeAttribute("src");
  video.load();
  video.remove();
}

/** A play() that is refused or interrupted is not an error here: the next show tries again. */
function play(video: HTMLVideoElement | undefined): void {
  if (video !== undefined && !document.hidden) void video.play().catch(() => undefined);
}

function createAerials(): BackgroundExt {
  let stage: HTMLElement | undefined;
  let playlist: Playlist = createPlaylist([], Math.random);
  let fileCount = 0;
  let fit: Fit = "cover";
  let current: HTMLVideoElement | undefined; // on screen
  let incoming: HTMLVideoElement | undefined; // loading, still invisible
  let failures = 0;
  let releaseTimer: ReturnType<typeof setTimeout> | undefined;
  let onVisibility: (() => void) | undefined;

  function playNext(): void {
    if (stage === undefined || incoming !== undefined) return;
    const url = playlist.next();
    if (url === undefined) return;
    const video = createVideo(fit, fileCount === 1);
    incoming = video;
    video.addEventListener("playing", () => reveal(video), { once: true });
    video.addEventListener("error", () => skip(video), { once: true });
    video.addEventListener("timeupdate", () => {
      if (video === current && fileCount > 1 && video.duration - video.currentTime <= SWITCH_BEFORE_END_S) playNext();
    });
    video.addEventListener("ended", () => { if (video === current) playNext(); });
    // Above the clip on screen, below the dim overlay.
    if (current !== undefined) current.after(video);
    else stage.prepend(video);
    video.src = url;
    play(video);
  }

  /** The new clip is moving: fade it in over the old one, then unload the old one. */
  function reveal(video: HTMLVideoElement): void {
    if (video !== incoming) return;
    failures = 0;
    video.style.opacity = "1";
    const previous = current;
    current = video;
    incoming = undefined;
    if (previous === undefined) return;
    clearTimeout(releaseTimer);
    releaseTimer = setTimeout(() => release(previous), CROSSFADE_MS);
  }

  /** A clip that cannot play is dropped; the next one is tried, but each file at most once in a row. */
  function skip(video: HTMLVideoElement): void {
    if (video !== incoming) return;
    release(video);
    incoming = undefined;
    failures += 1;
    if (failures < fileCount) playNext();
  }

  function start(root: HTMLElement, ctx: Ctx): void {
    stage = createStage(dimOption(ctx.options.dim));
    root.appendChild(stage);
    const files = fileList(ctx.options.files);
    playlist = createPlaylist(files, Math.random);
    fileCount = files.length;
    fit = fitOption(ctx.options.fit);
    onVisibility = () => {
      if (document.hidden) {
        current?.pause();
        incoming?.pause();
      } else {
        play(current);
        play(incoming);
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    playNext();
  }

  function stop(): void {
    if (onVisibility !== undefined) document.removeEventListener("visibilitychange", onVisibility);
    onVisibility = undefined;
    clearTimeout(releaseTimer);
    releaseTimer = undefined;
    // A clip whose release was still pending is the only other child that is a video.
    stage?.querySelectorAll("video").forEach(release);
    current = undefined;
    incoming = undefined;
    stage?.remove();
    stage = undefined;
  }

  return { start, stop };
}

window.agentScreensaver.register(createAerials);
