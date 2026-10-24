import type { Limit } from "../../api.ts";
import { displayName, formatCountdown, formatResetLabel, localUtcOffsetMinutes } from "../../format.ts";

const MS_PER_MINUTE = 60_000;
const MS_PER_HOUR = 60 * MS_PER_MINUTE;
const MS_PER_DAY = 24 * MS_PER_HOUR;
const WINDOW_ORDER: Limit["window"][] = ["5h", "week", "plan"];

/**
 * The installed agents' sources first, in the service's order (built-ins first), then
 * the others by name. Windows: 5h, week, plan.
 */
export function orderLimits(limits: Limit[], agentOrder: readonly string[]): Limit[] {
  const sourceRank = (source: string): number => {
    const known = agentOrder.indexOf(source);
    return known === -1 ? agentOrder.length : known;
  };
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
  /** Second caption line: the reset time ("4h 12m", "Mon 09:00", "1 Nov 02:00", "Reset"), the age when stale, and "est." for an estimate. Empty when the reset time is unknown and there is nothing else to add. */
  detail: string;
}

/**
 * What one ring shows. A reading taken before its window reset says nothing about
 * the new window, so its percent is never shown as current: the ring is empty and
 * its caption says "Reset" until a newer reading comes. Every other ring says when
 * it resets: a countdown to the minute within 24 hours, the day and time within 6
 * days, else the date (see formatResetLabel). `offsetAt` gives the local UTC offset at an instant.
 */
export function ringView(limit: Limit, nowMs: number, offsetAt: (atMs: number) => number = localUtcOffsetMinutes): RingView {
  const name = `${displayName(limit.source)} ${limit.window}`;
  if (limit.resets_at !== undefined && nowMs >= limit.resets_at * 1000) {
    return { look: "reset", percent: 0, center: "0%", name, detail: formatCountdown(0) };
  }
  const percent = ringPercent(limit.used_percent);
  const ageMs = nowMs - limit.updated_at_ms;
  const stale = ageMs > STALE_AFTER_MS[limit.window];
  const more: string[] = [];
  if (stale) more.push(`${shortAge(ageMs)} ago`);
  if (limit.estimated === true) more.push(ESTIMATE_MARK);
  // A far date drops its time when more follows, so the caption stays short.
  const details = [formatResetLabel(limit.resets_at, nowMs, offsetAt, { short: more.length > 0 }), ...more];
  return { look: stale ? "stale" : "fresh", percent, center: `${percent}%`, name, detail: details.filter((part) => part !== "").join(" · ") };
}

/** Age in its largest whole unit, so the caption stays short: "31m", "20h", "2d". */
function shortAge(ms: number): string {
  if (ms < MS_PER_HOUR) return `${Math.floor(ms / MS_PER_MINUTE)}m`;
  if (ms < MS_PER_DAY) return `${Math.floor(ms / MS_PER_HOUR)}h`;
  return `${Math.floor(ms / MS_PER_DAY)}d`;
}

/** The caption on one line: "Claude 5h · 1h 40m", "Codex plan · Mon 09:00". */
export function ringCaption(limit: Limit, nowMs: number, offsetAt?: (atMs: number) => number): string {
  const { name, detail } = ringView(limit, nowMs, offsetAt);
  return detail === "" ? name : `${name} · ${detail}`;
}

/** Whole percent for the ring, kept inside 0..100 so a bad value cannot break the drawing. */
export function ringPercent(usedPercent: number): number {
  if (!Number.isFinite(usedPercent)) return 0;
  return Math.min(100, Math.max(0, Math.round(usedPercent)));
}
