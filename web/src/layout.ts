import type { Mode, Region } from "./api.ts";

export type ExtRef = { id: string; options?: Record<string, unknown> };
export type Regions = Partial<Record<Region, ExtRef[]>>;

const REGIONS: readonly Region[] = ["top-left", "top-right", "center", "bottom"];

function fallbackRegions(): Regions {
  return { center: [{ id: "builtin.clock", options: { size: "large" } }] };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Keeps entries that have a string `id`. Options are passed through as given. */
function toExtRef(entry: unknown): ExtRef | undefined {
  if (!isRecord(entry) || typeof entry.id !== "string") return undefined;
  if (!isRecord(entry.options)) return { id: entry.id };
  return { id: entry.id, options: entry.options };
}

/**
 * The extensions to place in each region for `mode`. A layout that is
 * broken, or lacks this mode, shows a large clock instead of a blank screen.
 */
export function regionsFor(layout: unknown, mode: Mode): Regions {
  const placements = isRecord(layout) ? layout[mode] : undefined;
  if (!isRecord(placements)) return fallbackRegions();

  const regions: Regions = {};
  for (const region of REGIONS) {
    const refs = placements[region];
    if (!Array.isArray(refs)) continue;
    regions[region] = refs.map(toExtRef).filter((ref): ref is ExtRef => ref !== undefined);
  }
  return regions;
}
