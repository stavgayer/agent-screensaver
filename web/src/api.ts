/** Version of the extension API. A manifest must declare this exact number. */
export const API_VERSION = 1;

export type Region = "top-left" | "top-right" | "center" | "bottom";
export type Mode = "idle" | "attention" | "secondary";
export type CardKind = "approval" | "running" | "finished" | "unknown" | "no_activity";

/** One agent session. Field names match the service's state.json. */
export interface Card {
  source: string;
  session: string;
  kind: CardKind;
  project?: string;
  title?: string;
  request?: string;
  started_at_ms: number;
  updated_at_ms: number;
  approval_since_ms?: number;
}

/** One usage limit. `resets_at` is in seconds, the other times are in milliseconds. */
export interface Limit {
  source: string;
  window: "5h" | "week";
  used_percent: number;
  resets_at?: number;
  updated_at_ms: number;
}

export interface ViewState {
  mode: Mode;
  cards: Card[];
  limits: Limit[];
  nowMs: number;
  serviceOk: boolean;
  display: "main" | "secondary";
}

export interface Ctx {
  options: Readonly<Record<string, unknown>>;
  requestFrame(cb: (nowMs: number) => void): () => void;
  now(): number;
  display: "main" | "secondary";
}

export interface BackgroundExt {
  start(root: HTMLElement, ctx: Ctx): void;
  stop(): void;
}

export interface ComponentExt {
  mount(root: HTMLElement, ctx: Ctx): void;
  update(state: ViewState): void;
  unmount(): void;
}

/** An extension's entry module: `export default function create(): BackgroundExt | ComponentExt`. */
export type CreateExtension = () => BackgroundExt | ComponentExt;
