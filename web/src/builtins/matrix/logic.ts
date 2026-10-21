const CHARSETS: Record<string, string> = {
  katakana: Array.from({ length: 0x9d - 0x66 + 1 }, (_, index) => String.fromCharCode(0xff66 + index)).join(""),
  latin: "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
  digits: "0123456789",
  hebrew: Array.from({ length: 0x5ea - 0x5d0 + 1 }, (_, index) => String.fromCharCode(0x5d0 + index)).join(""),
};

/** The glyphs for a `charset` option value. An unknown value gives katakana, the default. */
export function charsetFor(name: unknown): string[] {
  const key = typeof name === "string" && Object.hasOwn(CHARSETS, name) ? name : "katakana";
  return Array.from(CHARSETS[key]);
}

/** A number option kept inside min..max; anything that is not a number gives the fallback. */
export function numberOption(value: unknown, fallback: number, min: number, max: number): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

/** The look was tuned at 24 frames a second with a black fill of this opacity. */
const TUNED_FPS = 24;
const TUNED_FADE = 0.08;

/** Fill opacity for a frame that took `frameMs`, so trails last as long at any frame rate. */
export function fadeAlpha(frameMs: number): number {
  return 1 - (1 - TUNED_FADE) ** (frameMs / (1000 / TUNED_FPS));
}

/** Rows a drop falls per second at speed 1: one row per frame at the tuned rate. */
export const ROWS_PER_SECOND = TUNED_FPS;

const RESTART_CHANCE_PER_ROW = 0.025;

/**
 * Moves a drop down by `rows`. Below the screen it restarts at the top with
 * the same chance per row that the original look used, so columns do not
 * restart in lockstep.
 */
export function advanceDrop(row: number, screenRows: number, rows: number, random: () => number): number {
  const restartChance = 1 - (1 - RESTART_CHANCE_PER_ROW) ** rows;
  if (row > screenRows && random() < restartChance) return 0;
  return row + rows;
}
