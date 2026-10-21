// Runs inside the settings window's preview frame (runtime/preview.html),
// next to the real runtime. The frame is sandboxed and served from
// ags-preview://, so the settings page cannot reach into it: it posts the
// state, and this hands it to the runtime.
import { toPreviewUrl } from "./preview-url.ts";

let dataUrl: string | undefined;

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

window.addEventListener("message", (event: MessageEvent) => {
  if (event.source !== window.parent) return;
  const data = event.data as { type?: unknown; state?: unknown; dataUrl?: unknown } | null;
  if (data?.type !== "agent-screensaver-state" || typeof data.state !== "string" || typeof data.dataUrl !== "string") return;
  dataUrl = data.dataUrl;
  window.agentScreensaver.setState(data.state, "main");
});
