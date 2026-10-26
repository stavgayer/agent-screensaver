import type { AgentInfo, Card } from "../../api.ts";
import { agentName, displayName, formatDuration } from "../../format.ts";

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

/** The state line in two parts: the state in words, and how long it has been so, when that is known. */
export interface TaskStatusParts {
  tone: TaskTone;
  word: string;
  since: string;
}

/** What the card is doing, in parts, for a look that sets the word and the time apart. */
export function taskStatusParts(card: Card, nowMs: number): TaskStatusParts {
  switch (card.kind) {
    case "running":
      return { tone: "working", word: "Working", since: formatDuration(nowMs - card.started_at_ms) };
    case "finished":
      return { tone: "done", word: "Done", since: ago(nowMs - card.updated_at_ms) };
    case "no_activity":
      return { tone: "quiet", word: "No activity", since: formatDuration(nowMs - card.updated_at_ms) };
    case "unknown":
      return { tone: "check", word: "Check the terminal", since: "" };
    case "approval":
      return { tone: "check", word: "Needs you", since: "" };
  }
}

/** What the card is doing, in words: the first line of a card. */
export function taskStatus(card: Card, nowMs: number): TaskStatus {
  const { tone, word, since } = taskStatusParts(card, nowMs);
  return { tone, text: since === "" ? word : `${word} · ${since}` };
}

function ago(ms: number): string {
  return ms < MS_PER_MINUTE ? "just now" : `${formatDuration(ms)} ago`;
}

/**
 * The bold line: the conversation name, else the prompt title, else (in
 * privacy mode) the project, else the agent.
 */
export function taskHeading(card: Card): string {
  return card.name || card.title || card.project || displayName(card.source);
}

/** One part of the last line. The view gives each role its own narrow-card rule. */
export interface WherePart {
  role: "project" | "model" | "branch";
  text: string;
}

/**
 * The last line: project, model and branch, leaving out what is unknown or
 * already the heading, plus how many background jobs still run.
 */
export function taskWhere(card: Card): { parts: WherePart[]; background: string } {
  const heading = taskHeading(card);
  const candidates: WherePart[] = [
    { role: "project", text: card.project ?? "" },
    { role: "model", text: card.model ?? "" },
    { role: "branch", text: card.branch ?? "" },
  ];
  const parts = candidates.filter((part) => part.text !== "" && !(part.role === "project" && part.text === heading));
  const count = card.background ?? 0;
  return { parts, background: count > 0 ? `${count} in background` : "" };
}

/**
 * From this fullness the context bar takes the attention color: Claude Code
 * compacts the conversation near the end of the window, so 80% is the point
 * where the user may want to wrap up or compact on their own terms.
 */
export const CONTEXT_HIGH_PERCENT = 80;

export interface TaskContext {
  /** Whole percent, 0 to 100: the width of the fill. */
  percent: number;
  text: string;
  high: boolean;
}

/** The context bar, or undefined when the agent does not say how full it is. */
export function taskContext(card: Card): TaskContext | undefined {
  const used = card.context_percent;
  if (used === undefined || !Number.isFinite(used)) return undefined;
  const percent = Math.round(Math.min(Math.max(used, 0), 100));
  return { percent, text: `${percent}%`, high: percent >= CONTEXT_HIGH_PERCENT };
}

/**
 * The card's agent: the mark of its agent extension (or the user's own mark, which the
 * service puts in its place) and its name. A source without an agent extension has no
 * mark (the card shows a neutral dot) and is named by its source.
 */
export function taskSource(card: Card, agents: Readonly<Record<string, AgentInfo>>): { markUrl: string | undefined; name: string } {
  const agent = Object.hasOwn(agents, card.source) ? agents[card.source] : undefined;
  return { markUrl: agent?.mark_url || undefined, name: agentName(card.source, agents) };
}
