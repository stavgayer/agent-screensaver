import type { NowPlaying } from "../../api.ts";

/** What the widget shows: the song, its artist ("" for none) and how far it is, 0 to 1, or null when the app gives no length. */
export interface SongView {
  title: string;
  artist: string;
  progress: number | null;
}

/**
 * The song at `nowMs`, or null when none plays. Between the service's reads
 * the position moves on from when it was read; it stops at the song's end
 * until the next read says what plays then.
 */
export function songView(track: NowPlaying | undefined, nowMs: number): SongView | null {
  if (track === undefined || track.title === "") return null;
  let progress: number | null = null;
  if (track.duration_ms > 0) {
    const positionMs = track.position_ms + Math.max(0, nowMs - track.at_ms);
    progress = Math.min(1, Math.max(0, positionMs / track.duration_ms));
  }
  return { title: track.title, artist: track.artist, progress };
}
