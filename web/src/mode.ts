import type { Card, Mode } from "./api.ts";

/** Which layout mode the screen uses: a secondary display never alerts. */
export function screenMode(cards: Card[], display: "main" | "secondary"): Mode {
  if (display === "secondary") return "secondary";
  return cards.some((card) => card.kind === "approval") ? "attention" : "idle";
}

/** Ascending; a missing value sorts last. Avoids `Infinity - Infinity` (NaN). */
function compareSince(a?: number, b?: number): number {
  if (a === b) return 0;
  if (a === undefined) return 1;
  if (b === undefined) return -1;
  return a - b;
}

/**
 * Pending approvals, oldest first. A card without `approval_since_ms` sorts
 * after those that have one; ties keep input order (Array.sort is stable).
 */
export function approvalQueue(cards: Card[]): { first?: Card; more: number } {
  const approvals = cards
    .filter((card) => card.kind === "approval")
    .sort((a, b) => compareSince(a.approval_since_ms, b.approval_since_ms));
  return { first: approvals[0], more: Math.max(0, approvals.length - 1) };
}
