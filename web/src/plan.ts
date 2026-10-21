import type { Card, Limit, Mode, Region, ViewState } from "./api.ts";
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
  };
}

export interface PlannedComponent {
  region: Region;
  index: number;
  id: string;
  entryUrl: string;
  options: Options;
  /** Names the placement in logs and failure tracking. */
  key: string;
}

export interface PlannedRegion {
  region: Region;
  /** Changes whenever anything that affects what the region shows changes. */
  signature: string;
  components: PlannedComponent[];
}

export type MountedSignatures = Partial<Record<Region, string>>;

/** Component defaults first, the placement's own options on top. */
export function mergeOptions(base: Options, placement: Options | undefined): Options {
  return { ...base, ...placement };
}

export function planRegions(state: StateFile, mode: Mode): PlannedRegion[] {
  const known = new Map(state.view.components.map((config) => [config.id, config]));
  const placed = regionsFor(state.view.layout?.modes, mode);
  const planned: PlannedRegion[] = [];
  for (const [region, refs] of Object.entries(placed) as [Region, NonNullable<typeof placed[Region]>][]) {
    const components: PlannedComponent[] = [];
    refs.forEach((ref, index) => {
      const config = known.get(ref.id);
      if (config === undefined) return; // not installed or not resolved: leave it out
      const options = mergeOptions(config.options, ref.options);
      components.push({ region, index, id: ref.id, entryUrl: config.entry_url, options, key: `${region}:${index}:${ref.id}` });
    });
    const signature = JSON.stringify(components.map((c) => [c.id, c.entryUrl, c.options]));
    planned.push({ region, signature, components });
  }
  return planned;
}

/**
 * What to do to reach `planned` from what is mounted. A region is replaced
 * as a whole, so the order of its components always matches the layout.
 */
export function diffRegions(
  mounted: MountedSignatures,
  planned: PlannedRegion[],
): { clear: Region[]; mount: PlannedRegion[] } {
  const wanted = new Map(planned.map((entry) => [entry.region, entry]));
  const clear = (Object.keys(mounted) as Region[]).filter(
    (region) => wanted.get(region)?.signature !== mounted[region],
  );
  const mount = planned.filter((entry) => mounted[entry.region] !== entry.signature);
  return { clear, mount };
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
