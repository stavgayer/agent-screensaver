import type { Card } from "../../api.ts";
import { displayName, formatDuration } from "../../format.ts";

const MS_PER_MINUTE = 60_000;

/**
 * The color family of a card's state line: working is the accent, done is
 * `--done`, quiet is plain white, check is `--attention`.
 */
export type TaskTone = "working" | "done" | "quiet" | "check";

export interface TaskStatus {
  tone: TaskTone;
  /** Short, plain words that read from across the room, for example "Working · 24m". */
  text: string;
}

/** Every card the row shows: approvals have their own component. */
export function taskCards(cards: Card[]): Card[] {
  return cards.filter((card) => card.kind !== "approval");
}

/** What the card is doing, in words: the first line of a card. */
export function taskStatus(card: Card, nowMs: number): TaskStatus {
  switch (card.kind) {
    case "running":
      return { tone: "working", text: `Working · ${formatDuration(nowMs - card.started_at_ms)}` };
    case "finished":
      return { tone: "done", text: `Done · ${ago(nowMs - card.updated_at_ms)}` };
    case "no_activity":
      return { tone: "quiet", text: `No activity · ${formatDuration(nowMs - card.updated_at_ms)}` };
    case "unknown":
      return { tone: "check", text: "Check the terminal" };
    case "approval":
      return { tone: "check", text: "Needs you" };
  }
}

function ago(ms: number): string {
  return ms < MS_PER_MINUTE ? "just now" : `${formatDuration(ms)} ago`;
}

/** The bold line: the title, or in privacy mode the project, or else the agent. */
export function taskHeading(card: Card): string {
  return card.title ?? card.project ?? displayName(card.source);
}

/**
 * The last line: where the task runs (project, then agent), leaving out
 * whatever is already the heading, plus how many background jobs still run.
 */
export function taskWhere(card: Card): { parts: string[]; background: string } {
  const heading = taskHeading(card);
  const parts = [card.project, displayName(card.source)].filter(
    (part): part is string => part !== undefined && part !== "" && part !== heading,
  );
  const count = card.background ?? 0;
  return { parts, background: count > 0 ? `${count} in background` : "" };
}
