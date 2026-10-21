import { numberOption } from "../matrix/logic.ts";

export type Fit = "cover" | "contain";

export const DEFAULT_DIM = 0.25;
export const MAX_DIM = 0.8;

/**
 * The `file://` URL for a `file` option. The settings service stores an
 * absolute path, and "Application Support" has a space, so every path
 * segment is encoded. Anything that is not an absolute path gives undefined.
 */
export function fileUrl(path: unknown): string | undefined {
  if (typeof path !== "string" || !path.startsWith("/")) return undefined;
  return `file://${path.split("/").map(encodeURIComponent).join("/")}`;
}

/** `cover` fills the screen and crops; `contain` shows the whole picture on black. Anything else is `cover`. */
export function fitOption(value: unknown): Fit {
  return value === "contain" ? "contain" : "cover";
}

/** How dark the static overlay is, from 0 (none) to 0.8, so the clock and cards stay readable. */
export function dimOption(value: unknown): number {
  return numberOption(value, DEFAULT_DIM, 0, MAX_DIM);
}

const FADE_IN = "opacity 600ms cubic-bezier(0.23, 1, 0.32, 1)";

/** A black box with a dark overlay. The media goes inside it, under the overlay. */
export function createStage(dim: number): HTMLElement {
  const stage = document.createElement("div");
  stage.style.cssText = "position:relative;width:100%;height:100%;overflow:hidden;background:#000";
  if (dim > 0) {
    const shade = document.createElement("div");
    shade.style.cssText = `position:absolute;inset:0;background:rgba(0,0,0,${dim})`;
    stage.appendChild(shade);
  }
  return stage;
}

/**
 * Puts `media` under the overlay. It starts invisible and fades in when
 * `loadEvent` fires; if it fails to load it is removed and the black stays.
 */
export function addMedia(stage: HTMLElement, media: HTMLElement, fit: Fit, loadEvent: string): void {
  media.style.cssText =
    `position:absolute;inset:0;width:100%;height:100%;object-fit:${fit};opacity:0;transition:${FADE_IN}`;
  media.addEventListener(loadEvent, () => { media.style.opacity = "1"; }, { once: true });
  media.addEventListener("error", () => media.remove(), { once: true });
  stage.prepend(media);
}
