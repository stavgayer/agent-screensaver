import type { Limit } from "../../api.ts";
import { displayName, formatDuration, formatResetIn, formatResetLabel } from "../../format.ts";

const STALE_AFTER_MS = 15 * 60_000;
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

/** The parts of a ring's caption: the name, then the reset time and the data age when they apply. */
function captionParts(limit: Limit, nowMs: number): string[] {
  const parts = [`${displayName(limit.source)} ${limit.window}`];
  if (limit.window === "5h") parts.push(formatResetIn(limit.resets_at, nowMs));
  if (limit.window === "plan") parts.push(formatResetLabel(limit.resets_at, nowMs));
  const age = nowMs - limit.updated_at_ms;
  if (age > STALE_AFTER_MS) parts.push(`updated ${formatDuration(age)} ago`);
  return parts.filter((part) => part !== "");
}

/** Text under a ring: "Claude 5h · 1h40m", "Codex plan · resets 3 Nov", or "Claude week · updated 20m ago" when the data is old. */
export function ringCaption(limit: Limit, nowMs: number): string {
  return captionParts(limit, nowMs).join(" · ");
}

/** The caption on two lines, so the name never breaks: { name: "Claude 5h", detail: "1h40m" }. */
export function ringCaptionLines(limit: Limit, nowMs: number): { name: string; detail: string } {
  const [name, ...details] = captionParts(limit, nowMs);
  return { name, detail: details.join(" · ") };
}

/** Whole percent for the ring, kept inside 0..100 so a bad value cannot break the drawing. */
export function ringPercent(usedPercent: number): number {
  if (!Number.isFinite(usedPercent)) return 0;
  return Math.min(100, Math.max(0, Math.round(usedPercent)));
}
