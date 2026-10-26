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
  /** How many background jobs (shells, subagents) the session still runs. Left out when none run or the agent does not say (Codex). */
  background?: number;
  /** The conversation name the agent shows (Claude /rename or its own title, the Codex thread name). Left out when unknown or in privacy mode. */
  name?: string;
  /** Short model name, for example "Opus 5.5" or "gpt-5.6". */
  model?: string;
  /** Git branch checked out in the session's folder. Left out outside a repository and in privacy mode. */
  branch?: string;
  /** How full the context window is, 0 to 100. Left out when the agent does not say. */
  context_percent?: number;
}

/** One usage limit. `resets_at` is in seconds, the other times are in milliseconds. */
export interface Limit {
  source: string;
  window: "5h" | "week" | "plan";
  used_percent: number;
  resets_at?: number;
  updated_at_ms: number;
  /** True when the percent was worked out locally between exact readings. */
  estimated?: boolean;
}

/** An agent extension as the cards show it: its name and the URL of its mark (an SVG shown with `<img>`). */
export interface AgentInfo {
  name: string;
  mark_url: string;
}

export interface ViewState {
  mode: Mode;
  cards: Card[];
  /** The installed agents by card source. A card whose source is not here has no agent extension. */
  agents: Readonly<Record<string, AgentInfo>>;
  /** The installed agents' sources, built-ins first: the order limits are shown in. */
  agentOrder: readonly string[];
  limits: Limit[];
  nowMs: number;
  serviceOk: boolean;
  display: "main" | "secondary";
}

export interface Ctx {
  options: Readonly<Record<string, unknown>>;
  /**
   * Calls `cb` every frame until you call the returned cancel function. Call
   * it once: calling it again inside `cb` adds a second request each frame.
   * `fps` is the rate you want; leave it out for every display frame. The
   * user's frame ceiling can still lower it.
   */
  requestFrame(cb: (nowMs: number) => void, fps?: number): () => void;
  now(): number;
  display: "main" | "secondary";
}

export interface BackgroundExt {
  start(root: HTMLElement, ctx: Ctx): void;
  /** Optional. Called after start and whenever the view state changes, like a component's update. */
  update?(state: ViewState): void;
  stop(): void;
}

export interface ComponentExt {
  mount(root: HTMLElement, ctx: Ctx): void;
  update(state: ViewState): void;
  /** Optional. Called when the same component moves to a placement with other options. */
  setOptions?(options: Readonly<Record<string, unknown>>): void;
  unmount(): void;
}

/** What an entry file passes to `window.agentScreensaver.register(create)`. */
export type CreateExtension = () => BackgroundExt | ComponentExt;
