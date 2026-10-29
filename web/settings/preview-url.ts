// The settings window serves its preview frame from ags-preview://data/...
// (the data folder) instead of file://, so the frame can be sandboxed: in
// WebKit a sandboxed file:// frame can load no file:// script. These helpers
// turn the data folder's file URLs into preview URLs.

export const PREVIEW_DATA_URL = "ags-preview://data/";

/** The data folder's file URL, from the settings page's own URL (runtime/settings/index.html). */
export function dataFolderUrl(pageHref: string): string {
  return new URL("../../", pageHref).href;
}

/** A file URL inside the data folder becomes a preview URL; any other URL is left as it is. */
export function toPreviewUrl(url: string, dataUrl: string): string {
  return url.startsWith(dataUrl) ? PREVIEW_DATA_URL + url.slice(dataUrl.length) : url;
}

interface Entry {
  entry_url?: unknown;
}

/**
 * The preview's frame rate. The preview is a small picture next to the
 * settings, so it does not need the screen saver's rate. WebKit gives a
 * sandboxed frame about 22 uneven frames a second; the preview takes 15 even
 * ones (frame-gate.ts).
 */
export const PREVIEW_MAX_FPS = 15;

/**
 * state.json text with the extension entry scripts loaded from the preview
 * scheme. Option values stay as they are: extensions check them (the aerials
 * background keeps only file:// URLs), and preview.ts maps media `src` in the
 * frame instead. Text that is not JSON is passed on unchanged.
 */
export function previewState(state: string, dataUrl: string): string {
  let parsed: { view?: { background?: Entry; components?: unknown } } | null;
  try {
    parsed = JSON.parse(state);
  } catch {
    return state;
  }
  // Text that is JSON but not a state object is passed on unchanged too.
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return state;
  const components = parsed.view?.components;
  const entries = [parsed.view?.background, ...(Array.isArray(components) ? (components as Entry[]) : [])];
  for (const entry of entries) {
    if (typeof entry?.entry_url === "string") entry.entry_url = toPreviewUrl(entry.entry_url, dataUrl);
  }
  return JSON.stringify(parsed);
}

/** Whether the state comes from a running service. Without one the preview shows a note over the idle screen. */
export function serviceRunning(state: string | null): boolean {
  if (state === null) return false;
  try {
    return (JSON.parse(state) as { service_ok?: unknown }).service_ok === true;
  } catch {
    return false;
  }
}

/**
 * The runtime's own "service not running" line. In the settings preview the
 * page already says so in a sentence, so the frame hides the line to show one
 * note, not two. The real screen saver does not load this rule.
 */
export const PREVIEW_ONLY_CSS = "#service-line { display: none !important; }";

/**
 * Whether the preview should be paused: while it is scrolled out of sight,
 * and once the window has been in the background (since blurredAtMs) for
 * pauseAfterMs.
 */
export function previewPaused(onScreen: boolean, blurredAtMs: number | undefined, nowMs: number, pauseAfterMs: number): boolean {
  if (!onScreen) return true;
  return blurredAtMs !== undefined && nowMs - blurredAtMs >= pauseAfterMs;
}
