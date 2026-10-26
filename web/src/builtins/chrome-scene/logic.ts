import type { CardKind, ViewState } from "../../api.ts";

export type SignState = "running" | "finished" | "approval" | "idle";

/** One chrome sign in the scene. `session` tells two cards of one agent apart. */
export interface Sign {
  source: string;
  session: string;
  label: string;
  state: SignState;
  flashStartMs?: number;
}

export const MAX_SIGNS = 6;

/** How long the lime flash of a card that just finished lasts. */
export const FLASH_MS = 1200;

const MAX_RENDER_SCALE = 0.5;
const MIN_RENDER_WIDTH = 320;

const STATES: Record<CardKind, SignState> = {
  running: "running",
  finished: "finished",
  approval: "approval",
  unknown: "idle",
  no_activity: "idle",
};

const MAX_LABEL = 8;

/**
 * The word a sign carries: the agent's name in capitals, its first word when
 * the whole name is longer than eight letters, cut at eight letters.
 */
export function labelFor(name: string): string {
  const upper = name.trim().toUpperCase();
  if (upper === "") return "?";
  const word = upper.length > MAX_LABEL ? upper.split(/\s+/)[0] : upper;
  return Array.from(word).slice(0, MAX_LABEL).join("");
}

/**
 * One sign per card, capped at MAX_SIGNS; none on a secondary display. The order
 * is stable, so a sign never jumps when its card updates: agents in the
 * service's order (unknown sources last), then the card's start, then the
 * session. A card that just turned finished gets flashStartMs.
 */
export function signsFrom(state: ViewState, previous: readonly Sign[], nowMs: number): Sign[] {
  if (state.display === "secondary") return [];
  const before = new Map(previous.map((sign) => [`${sign.source}\n${sign.session}`, sign]));
  const sourceRank = (source: string): number => {
    const known = state.agentOrder.indexOf(source);
    return known === -1 ? state.agentOrder.length : known;
  };
  return [...state.cards]
    .sort(
      (a, b) =>
        sourceRank(a.source) - sourceRank(b.source) ||
        a.started_at_ms - b.started_at_ms ||
        a.session.localeCompare(b.session) ||
        a.source.localeCompare(b.source),
    )
    .slice(0, MAX_SIGNS)
    .map((card) => {
      const sign: Sign = { source: card.source, session: card.session, label: labelFor(state.agents[card.source]?.name ?? card.source), state: STATES[card.kind] ?? "idle" };
      if (sign.state !== "finished") return sign;
      const old = before.get(`${card.source}\n${card.session}`);
      // A card seen for the first time already finished is old news: no flash.
      if (old === undefined) return sign;
      const flashStartMs = old.state === "finished" ? old.flashStartMs : nowMs;
      return flashStartMs === undefined ? sign : { ...sign, flashStartMs };
    });
}

/** The lime flash of a sign: 1 when it starts, fading to 0 over FLASH_MS. */
export function flashAmount(sign: Sign, nowMs: number): number {
  if (sign.flashStartMs === undefined) return 0;
  const progress = (nowMs - sign.flashStartMs) / FLASH_MS;
  if (progress < 0 || progress >= 1) return 0;
  return (1 - progress) ** 2;
}

/**
 * Height of the sign row in scene units. Calm, it sits just under the shape with
 * room above the task row; while an approval is open it drops below the approval
 * card (which the runtime centres on the screen).
 */
export const ROW_Y_CALM = -0.82;
export const ROW_Y_APPROVAL = -1.07;

/** The row's height for an approval amount from 0 (calm) to 1 (open). */
export function rowY(approval: number): number {
  const amount = Math.min(1, Math.max(0, approval));
  return ROW_Y_CALM + (ROW_Y_APPROVAL - ROW_Y_CALM) * amount;
}

/** 0 = calm, 1 = full pink; eases toward 1 while any approval is open. */
export function pinkTarget(state: ViewState): number {
  // A secondary display shows the shape and grain only.
  if (state.display === "secondary") return 0;
  return state.cards.some((card) => card.kind === "approval") ? 1 : 0;
}

/** Moves `current` toward `target`, covering most of the way in about `tauMs`, at any frame rate. */
export function easeToward(current: number, target: number, frameMs: number, tauMs: number): number {
  return target + (current - target) * Math.exp(-Math.max(0, frameMs) / tauMs);
}

/** Render size for a screen size: at most half in each direction, at least 320 wide. */
export function renderSize(width: number, height: number, scale: number): { width: number; height: number } {
  if (!(width > 0) || !(height > 0)) return { width: 1, height: 1 };
  const factor = Math.min(MAX_RENDER_SCALE, Math.max(0, scale));
  const renderWidth = Math.min(width, Math.max(MIN_RENDER_WIDTH, Math.round(width * factor)));
  return { width: renderWidth, height: Math.max(1, Math.round((renderWidth * height) / width)) };
}

/** How long the glitch burst lasts when a new approval opens. */
export const GLITCH_MS = 450;

/** The glitch burst `sinceMs` after it started: 1 at the start, 0 when over or never started. */
export function glitchAmount(sinceMs: number | undefined): number {
  if (sinceMs === undefined || sinceMs < 0 || sinceMs >= GLITCH_MS) return 0;
  return 1 - sinceMs / GLITCH_MS;
}

/** Counts approval signs, so a new approval can start a glitch burst. */
export function approvalCount(signs: readonly Sign[]): number {
  return signs.filter((sign) => sign.state === "approval").length;
}

/**
 * Lays signs of the given half widths in one centred row with `gap` between
 * them. A row wider than `maxWidth` is shrunk as a whole; `scale` says how much.
 */
export function layoutRow(halfWidths: readonly number[], gap: number, maxWidth: number): { centers: number[]; scale: number } {
  if (halfWidths.length === 0) return { centers: [], scale: 1 };
  const total = halfWidths.reduce((sum, half) => sum + half * 2, 0) + gap * (halfWidths.length - 1);
  const scale = Math.min(1, maxWidth / total);
  let left = -total / 2;
  const centers = halfWidths.map((half) => {
    const center = (left + half) * scale;
    left += half * 2 + gap;
    return center;
  });
  return { centers, scale };
}
