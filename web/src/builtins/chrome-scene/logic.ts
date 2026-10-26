import type { CardKind, ViewState } from "../../api.ts";

export type SignState = "running" | "finished" | "approval" | "idle";

/** One chrome sign in the scene. `session` tells two cards of one agent apart. */
export interface Sign {
  source: string;
  session: string;
  letter: string;
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

function letterFor(state: ViewState, source: string): string {
  const name = state.agents[source]?.name ?? source;
  return Array.from(name.trim())[0]?.toUpperCase() ?? "?";
}

/**
 * One sign per card, newest first, capped at MAX_SIGNS; none on a secondary
 * display. A card that just turned finished gets flashStartMs.
 */
export function signsFrom(state: ViewState, previous: readonly Sign[], nowMs: number): Sign[] {
  if (state.display === "secondary") return [];
  const before = new Map(previous.map((sign) => [`${sign.source}\n${sign.session}`, sign]));
  return [...state.cards]
    .sort((a, b) => b.updated_at_ms - a.updated_at_ms)
    .slice(0, MAX_SIGNS)
    .map((card) => {
      const sign: Sign = { source: card.source, session: card.session, letter: letterFor(state, card.source), state: STATES[card.kind] ?? "idle" };
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

/** 0 = calm, 1 = full pink; eases toward 1 while any approval is open. */
export function pinkTarget(state: ViewState): number {
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
