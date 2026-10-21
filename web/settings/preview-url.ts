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

/** state.json text with every data folder file URL (extension entries, aerial videos) as a preview URL. */
export function previewState(state: string, dataUrl: string): string {
  return state.split(dataUrl).join(PREVIEW_DATA_URL);
}
