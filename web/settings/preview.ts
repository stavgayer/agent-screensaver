// Runs inside the settings window's preview frame (runtime/preview.html),
// next to the real runtime. The frame is sandboxed and served from
// ags-preview://, so the settings page cannot reach into it: it posts the
// state, and this hands it to the runtime.
import { createFrameGate } from "./frame-gate.ts";
import { PREVIEW_MAX_FPS, PREVIEW_ONLY_CSS, toPreviewUrl } from "./preview-url.ts";

let dataUrl: string | undefined;

const previewOnly = document.createElement("style");
previewOnly.textContent = PREVIEW_ONLY_CSS;
document.head.append(previewOnly);

/**
 * Built-in media backgrounds build file:// URLs from the paths in their
 * options. A sandboxed frame cannot load those, so `src` on images and media
 * maps data folder files to the preview scheme. Other URLs are left alone.
 */
function mapSource(prototype: HTMLImageElement | HTMLMediaElement | HTMLSourceElement): void {
  const descriptor = Object.getOwnPropertyDescriptor(prototype, "src");
  if (descriptor?.set === undefined || descriptor.get === undefined) return;
  const { get, set } = descriptor;
  Object.defineProperty(prototype, "src", {
    configurable: true,
    enumerable: descriptor.enumerable,
    get,
    set(this: Element, value: string) {
      set.call(this, dataUrl === undefined ? value : toPreviewUrl(String(value), dataUrl));
    },
  });
}
mapSource(HTMLImageElement.prototype);
mapSource(HTMLMediaElement.prototype);
mapSource(HTMLSourceElement.prototype);

// The preview is a small picture: canvases that follow the pixel ratio draw
// one pixel per point, a quarter of the pixels on a Retina screen.
Object.defineProperty(window, "devicePixelRatio", { configurable: true, get: () => 1 });

// Every animation frame goes through the gate: it keeps the preview at
// PREVIEW_MAX_FPS, and the settings page pauses the runtime and all
// extensions through it with one message. The runtime asks for frames only
// after the first state, so it gets the gate too.
const gate = createFrameGate(
  { now: () => performance.now(), setTimer: (callback, delayMs) => void setTimeout(callback, delayMs) },
  1000 / PREVIEW_MAX_FPS,
);
window.requestAnimationFrame = (callback) => gate.request(callback);
window.cancelAnimationFrame = (id) => gate.cancel(id);

/** Media that played when the preview paused, to play again when it resumes. */
let pausedMedia: HTMLMediaElement[] = [];
let isPaused = false;

function setPaused(paused: boolean): void {
  if (paused === isPaused) return;
  isPaused = paused;
  gate.setPaused(paused);
  if (paused) {
    pausedMedia = Array.from(document.querySelectorAll<HTMLMediaElement>("video, audio")).filter((media) => !media.paused);
    pausedMedia.forEach((media) => media.pause());
  } else {
    pausedMedia.forEach((media) => void media.play().catch(() => {}));
    pausedMedia = [];
  }
}

window.addEventListener("message", (event: MessageEvent) => {
  if (event.source !== window.parent) return;
  const data = event.data as { type?: unknown; state?: unknown; dataUrl?: unknown; paused?: unknown } | null;
  if (data?.type === "agent-screensaver-paused" && typeof data.paused === "boolean") {
    setPaused(data.paused);
    return;
  }
  if (data?.type !== "agent-screensaver-state" || typeof data.state !== "string" || typeof data.dataUrl !== "string") return;
  dataUrl = data.dataUrl;
  window.agentScreensaver.setState(data.state, "main");
});
