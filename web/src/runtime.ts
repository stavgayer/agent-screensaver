import type { BackgroundExt, ComponentExt, CreateExtension, Ctx, Region, ViewState } from "./api.ts";
import { loadExtension, pendingLoads, safeCall } from "./loader.ts";
import { screenMode } from "./mode.ts";
import { createFrameScheduler, type FrameScheduler } from "./frame.ts";
import {
  backgroundSignature,
  cssVariables,
  diffRegions,
  isServiceLineVisible,
  planRegions,
  resetKey,
  toViewState,
  type MountedSignatures,
  type Options,
  type PlannedRegion,
  type StateFile,
} from "./plan.ts";

const SERVICE_LINE_CHECK_MS = 10_000;
// The ceiling in `max_fps` does the limiting; an extension asks for "as fast as allowed".
const REQUESTED_FPS = 60;
const BACKGROUND_KEY = "background";

type Display = "main" | "secondary";

interface AgentScreensaver {
  setState(json: string, display: Display): void;
  register(create: CreateExtension): void;
}

declare global {
  interface Window {
    agentScreensaver: AgentScreensaver;
  }
}

/** One running extension and what is needed to take it down again. */
interface Mounted {
  key: string;
  root: HTMLElement;
  cancels: Set<() => void>;
  stop(): void;
  update?(state: ViewState): void;
  dead: boolean;
}

let latest: { state: StateFile; display: Display } | undefined;
let scheduler: FrameScheduler | undefined;
let appliedResetKey: string | undefined;
let background: { signature: string; mounted?: Mounted } | undefined;
const regions = new Map<Region, { signature: string; items: Mounted[] }>();
// Failed extensions are not retried until the theme changes.
const failed = new Set<string>();
const loaded = new Map<string, CreateExtension | undefined>();
let queue: Promise<void> = Promise.resolve();

const byId = (id: string): HTMLElement => {
  const element = document.getElementById(id);
  if (element === null) throw new Error(`missing #${id} in index.html`);
  return element;
};

const logFailure = (label: string, err: unknown): void => console.error(`[agent-screensaver] ${label}`, err);

/** Takes an extension down once: stop it, cancel its frames, remove its element. */
function teardown(item: Mounted): void {
  if (item.dead) return;
  item.dead = true;
  safeCall(`${item.key}.stop`, () => item.stop(), logFailure);
  item.cancels.forEach((cancel) => safeCall(`${item.key}.cancel`, cancel, logFailure));
  item.root.remove();
}

/** Runs one extension call; a throw takes that extension down and keeps it from coming back. */
function guard<T>(item: Mounted, step: string, fn: () => T): T | undefined {
  return safeCall(`${item.key}.${step}`, fn, (label, err) => {
    logFailure(label, err);
    failed.add(item.key);
    teardown(item);
  });
}

function createContext(item: Mounted, options: Options, display: Display): Ctx {
  return {
    options: Object.freeze({ ...options }),
    requestFrame(callback) {
      const cancel = (scheduler as FrameScheduler).request(callback, REQUESTED_FPS);
      item.cancels.add(cancel);
      return cancel;
    },
    now: () => Date.now(),
    display,
  };
}

async function createFor(entryUrl: string): Promise<CreateExtension | undefined> {
  if (!loaded.has(entryUrl)) loaded.set(entryUrl, await loadExtension(entryUrl));
  return loaded.get(entryUrl);
}

/** Loads, creates and starts one extension inside `parent`. */
async function launch(
  key: string,
  parent: HTMLElement,
  entryUrl: string,
  options: Options,
  kind: "background" | "component",
): Promise<Mounted | undefined> {
  if (failed.has(key)) return undefined;
  const create = await createFor(entryUrl);
  if (create === undefined) {
    failed.add(key);
    logFailure(key, new Error(`could not load ${entryUrl}`));
    return undefined;
  }
  const item: Mounted = { key, root: document.createElement("div"), cancels: new Set(), stop() {}, dead: false };
  const ext = guard(item, "create", create);
  if (ext === undefined) return undefined;
  parent.appendChild(item.root);
  const ctx = createContext(item, options, (latest as NonNullable<typeof latest>).display);
  if (kind === "background") {
    item.stop = () => (ext as BackgroundExt).stop();
    guard(item, "start", () => (ext as BackgroundExt).start(item.root, ctx));
  } else {
    item.stop = () => (ext as ComponentExt).unmount();
    item.update = (state) => (ext as ComponentExt).update(state);
    guard(item, "mount", () => (ext as ComponentExt).mount(item.root, ctx));
  }
  return item;
}

function clearRegion(region: Region): void {
  regions.get(region)?.items.forEach(teardown);
  regions.delete(region);
}

async function mountRegion(planned: PlannedRegion): Promise<void> {
  const items: Mounted[] = [];
  regions.set(planned.region, { signature: planned.signature, items });
  for (const component of planned.components) {
    const item = await launch(component.key, byId(`region-${planned.region}`), component.entryUrl, component.options, "component");
    if (item !== undefined) items.push(item);
  }
}

function teardownAll(): void {
  [...regions.keys()].forEach(clearRegion);
  if (background?.mounted !== undefined) teardown(background.mounted);
  background = undefined;
}

/** A new theme, or a new frame ceiling, starts from nothing. */
function resetIfNeeded(state: StateFile): void {
  const key = resetKey(state.view);
  if (key === appliedResetKey) return;
  teardownAll();
  failed.clear();
  loaded.clear();
  scheduler = createFrameScheduler(state.view.max_fps, requestAnimationFrame, (err) => logFailure("frame", err));
  appliedResetKey = key;
}

async function reconcile(): Promise<void> {
  if (latest === undefined) return;
  const { state, display } = latest;
  resetIfNeeded(state);

  const signature = backgroundSignature(state.view);
  if (background?.signature !== signature) {
    if (background?.mounted !== undefined) teardown(background.mounted);
    background = { signature };
    const { entry_url: entryUrl, options } = state.view.background;
    background.mounted = await launch(BACKGROUND_KEY, byId("background"), entryUrl, options, "background");
  }

  const mode = screenMode(state.cards, display);
  const mounted: MountedSignatures = Object.fromEntries([...regions].map(([region, entry]) => [region, entry.signature]));
  const diff = diffRegions(mounted, planRegions(state, mode));
  diff.clear.forEach(clearRegion);
  for (const planned of diff.mount) await mountRegion(planned);

  const view = toViewState(state, display, Date.now());
  regions.forEach((entry) => entry.items.forEach((item) => guard(item, "update", () => item.update?.(view))));
}

/** Parts of the page that follow the state directly, with no loading involved. */
function applyChrome(state: StateFile, display: Display): void {
  document.body.className = `mode-${screenMode(state.cards, display)}`;
  for (const [name, value] of cssVariables(state.view.style)) {
    document.documentElement.style.setProperty(name, value);
  }
  refreshServiceLine();
}

function refreshServiceLine(): void {
  if (latest === undefined) return;
  byId("service-line").hidden = !isServiceLineVisible(latest.state, Date.now());
}

function setState(json: string, display: Display): void {
  const state = safeCall("setState.parse", () => JSON.parse(json) as StateFile, logFailure);
  if (state === undefined) return;
  latest = { state, display };
  safeCall("setState.chrome", () => applyChrome(state, display), logFailure);
  queue = queue.then(reconcile).catch((err) => logFailure("reconcile", err));
}

window.agentScreensaver = Object.freeze({
  setState,
  register(create: CreateExtension) {
    if (typeof create !== "function") return;
    pendingLoads.register((document.currentScript as HTMLScriptElement | null)?.src, create);
  },
});

// The daemon rewrites state every 30 s; a stopped one must show the line without a new setState.
setInterval(refreshServiceLine, SERVICE_LINE_CHECK_MS);
