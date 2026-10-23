import type { AgentInfo, Card, Limit, Mode, Region, ViewState } from "./api.ts";
import { regionsFor } from "./layout.ts";
import { screenMode } from "./mode.ts";

/** Older than this, the daemon is assumed stopped (it rewrites state every 30 s). */
export const STALE_AFTER_MS = 120_000;

export type Options = Record<string, unknown>;

export interface ExtensionConfig {
  id: string;
  entry_url: string;
  options: Options;
}

/** The `state.json` shape the Go service writes (snake_case). */
export interface StateFile {
  version: number;
  generated_at_ms: number;
  service_ok: boolean;
  cards: Card[];
  limits: Limit[];
  view: {
    theme: string;
    background: ExtensionConfig;
    layout: { modes: unknown } | null;
    components: ExtensionConfig[];
    style: Record<string, unknown>;
    privacy: unknown;
    max_fps: number;
    /** Left out by a service from before agent extensions. */
    agents?: Record<string, AgentInfo>;
  };
}

export interface PlannedComponent {
  region: Region;
  /** Position inside its region. */
  index: number;
  id: string;
  entryUrl: string;
  options: Options;
}

/** What a mounted component must tell the planner. */
export interface RunningComponent {
  id: string;
  entryUrl: string;
  options: Options;
  supportsSetOptions: boolean;
  region: Region;
}

export interface Reconciliation<T extends RunningComponent> {
  /** Instances that go away (not in the new mode, or replaced). */
  unmount: T[];
  /** Placements that need a new instance. */
  mount: PlannedComponent[];
  /** Instances that stay. `moved` is a region change; `setOptions` says to tell the component. */
  keep: { running: T; target: PlannedComponent; moved: boolean; setOptions: boolean }[];
}

/** Component defaults first, the placement's own options on top. */
export function mergeOptions(base: Options, placement: Options | undefined): Options {
  return { ...base, ...placement };
}

/** Every placed component for `mode`, region by region. Ids that are not resolved are left out. */
export function planComponents(state: StateFile, mode: Mode): PlannedComponent[] {
  const known = new Map(state.view.components.map((config) => [config.id, config]));
  const placed = regionsFor(state.view.layout?.modes, mode);
  const planned: PlannedComponent[] = [];
  for (const [region, refs] of Object.entries(placed) as [Region, NonNullable<(typeof placed)[Region]>][]) {
    let index = 0;
    for (const ref of refs) {
      const config = known.get(ref.id);
      if (config === undefined) continue;
      const options = mergeOptions(config.options, ref.options);
      planned.push({ region, index: index++, id: ref.id, entryUrl: config.entry_url, options });
    }
  }
  return planned;
}

const sameOptions = (a: Options, b: Options): boolean => JSON.stringify(a) === JSON.stringify(b);

/**
 * Decides what a mode change does to each component. The same id keeps its
 * instance when it moves to another region (the first placement of an id
 * pairs with the first running one). Other options go to `setOptions` when
 * the component has it, otherwise the component is mounted again.
 */
export function reconcileComponents<T extends RunningComponent>(
  running: T[],
  planned: PlannedComponent[],
): Reconciliation<T> {
  const unclaimed = [...running];
  const result: Reconciliation<T> = { unmount: [], mount: [], keep: [] };
  for (const target of planned) {
    const at = unclaimed.findIndex((item) => item.id === target.id && item.entryUrl === target.entryUrl);
    const match = at === -1 ? undefined : unclaimed.splice(at, 1)[0];
    if (match === undefined) {
      result.mount.push(target);
    } else if (sameOptions(match.options, target.options)) {
      result.keep.push({ running: match, target, moved: match.region !== target.region, setOptions: false });
    } else if (match.supportsSetOptions) {
      result.keep.push({ running: match, target, moved: match.region !== target.region, setOptions: true });
    } else {
      result.unmount.push(match);
      result.mount.push(target);
    }
  }
  result.unmount.push(...unclaimed);
  return result;
}

/** Everything is torn down and rebuilt when this changes: the theme, or the frame scheduler. */
export function resetKey(view: StateFile["view"]): string {
  return JSON.stringify([view.theme, view.max_fps]);
}

export function backgroundSignature(view: StateFile["view"]): string {
  return JSON.stringify([view.background.id, view.background.entry_url, view.background.options]);
}

export function isServiceLineVisible(state: StateFile, nowMs: number): boolean {
  return !state.service_ok || nowMs - state.generated_at_ms > STALE_AFTER_MS;
}

export function toViewState(state: StateFile, display: "main" | "secondary", nowMs: number): ViewState {
  return {
    mode: screenMode(state.cards, display),
    cards: state.cards,
    agents: state.view.agents ?? {},
    limits: state.limits,
    nowMs,
    serviceOk: state.service_ok,
    display,
  };
}

/** The `--name` entries of `view.style`, as [name, value] pairs for `:root`. */
export function cssVariables(style: Record<string, unknown>): [string, string][] {
  return Object.entries(style)
    .filter(([name]) => name.startsWith("--"))
    .map(([name, value]) => [name, String(value)]);
}
