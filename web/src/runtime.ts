import type { BackgroundExt, ComponentExt, CreateExtension, Ctx, Region } from "./api.ts";
import { loadExtension, pendingLoads, safeCall } from "./loader.ts";
import { createFrameScheduler, requestedFps, type FrameScheduler } from "./frame.ts";
import { createKeeper, type Instance } from "./instance.ts";
import { screenMode } from "./mode.ts";
import { setLockUI } from "./overlay.ts";
import {
  backgroundSignature,
  cssVariables,
  isServiceLineVisible,
  planComponents,
  reconcileComponents,
  resetKey,
  toViewState,
  type Options,
  type PlannedComponent,
  type StateFile,
} from "./plan.ts";

const SERVICE_LINE_CHECK_MS = 10_000;
const MOVE_DURATION_MS = 400;
const BACKGROUND_KEY = "background";

type Display = "main" | "secondary";

interface AgentScreensaver {
  setState(json: string, display: Display): void;
  /** The host calls this when macOS shows or hides its lock screen over the saver. */
  setLockUI(shown: boolean): void;
  register(create: CreateExtension): void;
}

declare global {
  interface Window {
    agentScreensaver: AgentScreensaver;
  }
}

interface ComponentInstance extends Instance {
  id: string;
  entryUrl: string;
  options: Options;
  region: Region;
  index: number;
  element: HTMLElement;
  supportsSetOptions: boolean;
  setOptions(options: Options): void;
}

let latest: { state: StateFile; display: Display } | undefined;
let scheduler: FrameScheduler | undefined;
let appliedResetKey: string | undefined;
let background: { signature: string; item?: Instance } | undefined;
let components: ComponentInstance[] = [];
// Failed extensions are not retried until the theme changes.
const failed = new Set<string>();
const loaded = new Map<string, CreateExtension | undefined>();
let queue: Promise<void> = Promise.resolve();

const logFailure = (label: string, err: unknown): void => console.error(`[agent-screensaver] ${label}`, err);
const keeper = createKeeper(failed, logFailure);

const byId = (id: string): HTMLElement => {
  const element = document.getElementById(id);
  if (element === null) throw new Error(`missing #${id} in index.html`);
  return element;
};

const regionElement = (region: Region): HTMLElement => byId(`region-${region}`);

function createContext(item: Instance, options: Options, display: Display): Ctx {
  return {
    options: Object.freeze({ ...options }),
    requestFrame: (callback, fps) =>
      keeper.trackFrame(item, () => (scheduler as FrameScheduler).request(callback, requestedFps(fps))),
    now: () => Date.now(),
    display,
  };
}

async function createFor(entryUrl: string): Promise<CreateExtension | undefined> {
  if (!loaded.has(entryUrl)) loaded.set(entryUrl, await loadExtension(entryUrl));
  return loaded.get(entryUrl);
}

/**
 * Loads, creates and starts one extension inside `parent`. Gives back
 * `undefined` when it cannot load or when it dies while starting.
 */
async function launch(
  key: string,
  parent: HTMLElement,
  entryUrl: string,
  options: Options,
  kind: "background" | "component",
): Promise<{ item: Instance; element: HTMLElement; ext: BackgroundExt | ComponentExt } | undefined> {
  if (failed.has(key)) return undefined;
  const create = await createFor(entryUrl);
  if (create === undefined) {
    failed.add(key);
    logFailure(key, new Error(`could not load ${entryUrl}`));
    return undefined;
  }
  const root = document.createElement("div");
  const item: Instance = { key, root, cancels: new Set(), stop() {}, dead: false };
  const ext = keeper.guard(item, "create", create);
  if (ext === undefined) return undefined;
  root.classList.add("entering");
  root.addEventListener("animationend", () => root.classList.remove("entering"), { once: true });
  parent.appendChild(root);
  const ctx = createContext(item, options, (latest as NonNullable<typeof latest>).display);
  if (kind === "background") {
    item.stop = () => (ext as BackgroundExt).stop();
    keeper.guard(item, "start", () => (ext as BackgroundExt).start(root, ctx));
  } else {
    item.stop = () => (ext as ComponentExt).unmount();
    item.update = (state) => (ext as ComponentExt).update(state);
    keeper.guard(item, "mount", () => (ext as ComponentExt).mount(root, ctx));
  }
  return item.dead ? undefined : { item, element: root, ext };
}

async function mountComponent(target: PlannedComponent): Promise<ComponentInstance | undefined> {
  const launched = await launch(target.id, regionElement(target.region), target.entryUrl, target.options, "component");
  if (launched === undefined) return undefined;
  const ext = launched.ext as ComponentExt;
  return Object.assign(launched.item, {
    id: target.id,
    entryUrl: target.entryUrl,
    options: target.options,
    region: target.region,
    index: target.index,
    element: launched.element,
    supportsSetOptions: typeof ext.setOptions === "function",
    setOptions: (options: Options) => ext.setOptions?.(Object.freeze({ ...options })),
  });
}

function teardownAll(): void {
  components.forEach(keeper.teardown);
  components = [];
  if (background?.item !== undefined) keeper.teardown(background.item);
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

async function reconcileBackground(state: StateFile): Promise<void> {
  const signature = backgroundSignature(state.view);
  if (background?.signature === signature) return;
  if (background?.item !== undefined) keeper.teardown(background.item);
  background = { signature };
  const { entry_url: entryUrl, options } = state.view.background;
  background.item = (await launch(BACKGROUND_KEY, byId("background"), entryUrl, options, "background"))?.item;
}

/** Puts every item in its region, in layout order, touching only what is out of place. */
function arrange(items: ComponentInstance[]): void {
  const regions = new Set(items.map((item) => item.region));
  for (const region of regions) {
    const parent = regionElement(region);
    items
      .filter((item) => item.region === region)
      .sort((a, b) => a.index - b.index)
      .forEach((item, position) => {
        if (parent.children[position] !== item.element) parent.insertBefore(item.element, parent.children[position] ?? null);
      });
  }
}

/** FLIP: the element is already in its new place; play it from where it was. */
function animateMove(root: HTMLElement, first: DOMRect): void {
  const last = root.getBoundingClientRect();
  if (last.width === 0 || last.height === 0) return;
  const inverse = `translate(${first.left - last.left}px, ${first.top - last.top}px) scale(${first.width / last.width}, ${first.height / last.height})`;
  root.style.transformOrigin = "top left";
  root.animate([{ transform: inverse }, { transform: "none" }], { duration: MOVE_DURATION_MS, easing: "ease-in-out" });
}

const prefersReducedMotion = (): boolean => window.matchMedia("(prefers-reduced-motion: reduce)").matches;

async function reconcileMounted(state: StateFile, display: Display): Promise<void> {
  const planned = planComponents(state, screenMode(state.cards, display)).filter((target) => !failed.has(target.id));
  const plan = reconcileComponents(components.filter((item) => !item.dead), planned);

  plan.unmount.forEach(keeper.teardown);
  const firstRects = new Map<ComponentInstance, DOMRect>();
  for (const { running, target, moved, setOptions } of plan.keep) {
    if (moved) firstRects.set(running, running.element.getBoundingClientRect());
    if (setOptions) keeper.guard(running, "setOptions", () => running.setOptions(target.options));
    Object.assign(running, { options: target.options, region: target.region, index: target.index });
  }
  const added: ComponentInstance[] = [];
  for (const target of plan.mount) {
    const item = await mountComponent(target);
    if (item !== undefined) added.push(item);
  }

  components = [...plan.keep.map((entry) => entry.running), ...added].filter((item) => !item.dead);
  arrange(components);
  if (!prefersReducedMotion()) {
    firstRects.forEach((first, item) => {
      if (!item.dead) safeCall(`${item.key}.move`, () => animateMove(item.element, first), logFailure);
    });
  }
}

async function reconcile(): Promise<void> {
  if (latest === undefined) return;
  const { state, display } = latest;
  resetIfNeeded(state);
  await reconcileBackground(state);
  await reconcileMounted(state, display);
  keeper.updateAll(components, toViewState(state, display, Date.now()));
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
  setLockUI: (shown: boolean) => safeCall("setLockUI", () => setLockUI(document.documentElement, shown), logFailure),
  register(create: CreateExtension) {
    if (typeof create !== "function") return;
    pendingLoads.register((document.currentScript as HTMLScriptElement | null)?.src, create);
  },
});

// The daemon rewrites state every 30 s; a stopped one must show the line without a new setState.
setInterval(refreshServiceLine, SERVICE_LINE_CHECK_MS);
