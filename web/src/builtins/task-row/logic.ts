import type { Card } from "../../api.ts";
import { formatDuration } from "../../format.ts";

/** Every card the row shows: approvals have their own component. */
export function taskCards(cards: Card[]): Card[] {
  return cards.filter((card) => card.kind !== "approval");
}

/** The time line of a card, for example "running 24m" or "finished 8m ago". */
export function taskLabel(card: Card, nowMs: number): string {
  switch (card.kind) {
    case "running":
      return `running ${formatDuration(nowMs - card.started_at_ms)}`;
    case "finished":
      return `finished ${formatDuration(nowMs - card.updated_at_ms)} ago`;
    case "unknown":
      return "check the terminal";
    case "no_activity":
      return "no activity";
    case "approval":
      return "";
  }
}
