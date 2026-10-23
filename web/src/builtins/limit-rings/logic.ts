import type { Limit } from "../../api.ts";
import { displayName, formatResetIn, formatResetLabel } from "../../format.ts";

const MS_PER_MINUTE = 60_000;
const MS_PER_HOUR = 60 * MS_PER_MINUTE;
const MS_PER_DAY = 24 * MS_PER_HOUR;
const SOURCE_ORDER = ["claude", "codex"];
const WINDOW_ORDER: Limit["window"][] = ["5h", "week", "plan"];

/** Known sources first in a fixed order, then the others by name. Windows: 5h, week, plan. */
function sourceRank(source: string): number {
  const known = SOURCE_ORDER.indexOf(source);
  return known === -1 ? SOURCE_ORDER.length : known;
}

export function orderLimits(limits: Limit[]): Limit[] {
  return [...limits].sort(
    (a, b) =>
      sourceRank(a.source) - sourceRank(b.source) ||
      a.source.localeCompare(b.source) ||
      WINDOW_ORDER.indexOf(a.window) - WINDOW_ORDER.indexOf(b.window),
  );
}

/**
 * How old a reading may be before the ring is dimmed and says its age. A 5h window
 * moves fast, so its readings age in half an hour; week and plan windows move
 * slowly. Claude readings come only from the status line, so they stop while no
 * Claude Code terminal session runs.
 */
export const STALE_AFTER_MS: Record<Limit["window"], number> = {
  "5h": 30 * MS_PER_MINUTE,
  week: 6 * MS_PER_HOUR,
  plan: 6 * MS_PER_HOUR,
};

/** Said after an estimated percent, so it is never taken for an exact one. */
const ESTIMATE_MARK = "est.";

/** fresh: as read. stale: older than STALE_AFTER_MS, dimmed with its age. reset: the window reset after the reading. */
export type RingLook = "fresh" | "stale" | "reset";

export interface RingView {
  look: RingLook;
  /** The arc, 0 to 100. Empty after a reset. */
  percent: number;
  /** The text inside the ring. */
  center: string;
  /** First caption line, for example "Claude 5h". */
  name: string;
  /** Second caption line: the reset time, the age when stale, and "est." for an estimate. Empty when there is nothing to add. */
  detail: string;
}

/**
 * What one ring shows. A reading taken before its window reset says nothing about
 * the new window, so its percent is never shown as current: the ring is empty and
 * says "Reset" until a newer reading comes.
 */
export function ringView(limit: Limit, nowMs: number): RingView {
  const name = `${displayName(limit.source)} ${limit.window}`;
  if (limit.resets_at !== undefined && nowMs >= limit.resets_at * 1000) {
    return { look: "reset", percent: 0, center: "Reset", name, detail: "" };
  }
  const percent = ringPercent(limit.used_percent);
  const details: string[] = [];
  if (limit.window === "5h") details.push(formatResetIn(limit.resets_at, nowMs));
  if (limit.window === "plan") details.push(formatResetLabel(limit.resets_at, nowMs));
  const ageMs = nowMs - limit.updated_at_ms;
  const stale = ageMs > STALE_AFTER_MS[limit.window];
  if (stale) details.push(`${shortAge(ageMs)} ago`);
  if (limit.estimated === true) details.push(ESTIMATE_MARK);
  return { look: stale ? "stale" : "fresh", percent, center: `${percent}%`, name, detail: details.filter((part) => part !== "").join(" · ") };
}

/** Age in its largest whole unit, so the caption stays short: "31m", "20h", "2d". */
function shortAge(ms: number): string {
  if (ms < MS_PER_HOUR) return `${Math.floor(ms / MS_PER_MINUTE)}m`;
  if (ms < MS_PER_DAY) return `${Math.floor(ms / MS_PER_HOUR)}h`;
  return `${Math.floor(ms / MS_PER_DAY)}d`;
}

/** The caption on one line: "Claude 5h · 1h40m", "Codex plan · resets 3 Nov". */
export function ringCaption(limit: Limit, nowMs: number): string {
  const { name, detail } = ringView(limit, nowMs);
  return detail === "" ? name : `${name} · ${detail}`;
}

/** Whole percent for the ring, kept inside 0..100 so a bad value cannot break the drawing. */
export function ringPercent(usedPercent: number): number {
  if (!Number.isFinite(usedPercent)) return 0;
  return Math.min(100, Math.max(0, Math.round(usedPercent)));
}
