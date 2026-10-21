import type { Card } from "../../api.ts";
import { formatDuration } from "../../format.ts";

/** "waiting 3m", counted from when the approval began (the last update if that is unknown). */
export function approvalWaitingLabel(card: Card, nowMs: number): string {
  return `waiting ${formatDuration(nowMs - (card.approval_since_ms ?? card.updated_at_ms))}`;
}

/** "+2 more" when more approvals are queued behind the one on screen, else nothing. */
export function approvalMoreLabel(more: number): string {
  return more > 0 ? `+${more} more` : "";
}
