import { createBridge } from "./bridge.ts";
import { formFor, type Field, type OptionSpec } from "./form.ts";

/** The `snapshot` answer (Go's settings.Snapshot). */
interface Snapshot {
  config: {
    theme: string;
    background: string;
    sound: boolean;
    sound_file: string;
    privacy: boolean;
    max_fps: number;
  };
  /** The background in effect: the user's choice or the theme's. */
  background: string;
  extensions: ExtensionInfo[];
}

interface ExtensionInfo {
  manifest: {
    id: string;
    name?: string;
    version?: string;
    type: "background" | "component" | "layout" | "theme";
    options?: Record<string, OptionSpec>;
    theme?: { background: { id: string }; layout: string; style?: Record<string, unknown> };
  };
  values: Record<string, unknown>;
  builtin: boolean;
}

interface PickResult {
  cancelled: boolean;
  manifest?: { id: string; name?: string };
}

type View = "appearance" | "options" | "extensions" | "general";

const VIEW_TITLES: Record<View, string> = {
  appearance: "Theme & background",
  options: "Extension options",
  extensions: "Extensions",
  general: "General",
};
const TYPE_NAMES = { background: "Background", component: "Component", layout: "Layout", theme: "Theme" };
const FPS_CHOICES = [0, 15, 24, 30, 60, 120, 240];
const PREVIEW_POLL_MS = 1000;
const STATUS_CLEAR_MS = 2000;
// The runtime's own default accent (styles.css), for a theme that sets none.
const DEFAULT_ACCENT = "#00e07a";

declare global {
  interface Window {
    __agsReply(id: number, result: unknown, error: string | null): void;
    webkit?: { messageHandlers: { ags: { postMessage(message: unknown): void } } };
  }
}

const bridge = createBridge(
  (request) => {
    if (window.webkit === undefined) throw new Error("not inside the settings window");
    window.webkit.messageHandlers.ags.postMessage(request);
  },
  { set: (callback, ms) => setTimeout(callback, ms), clear: (handle) => clearTimeout(handle as number) },
);
window.__agsReply = (id, result, error) => bridge.reply(id, result, error);

let snapshot: Snapshot | undefined;
let notices: Record<string, string> = {};
let currentView: View = "appearance";
/** The extension whose Remove button waits for a second click. */
let confirmingRemove: string | undefined;

// ---------- DOM helpers ----------

type Child = Node | string | false | null | undefined;

/** Creates an element. Text always goes in as text, never as HTML: names come from outside extensions. */
function h<K extends keyof HTMLElementTagNameMap>(
  tag: K,
  props: Partial<Record<string, string | boolean | ((event: Event) => void)>> = {},
  ...children: Child[]
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  for (const [name, value] of Object.entries(props)) {
    if (value === undefined || value === false) continue;
    if (typeof value === "function") element.addEventListener(name.slice(2), value);
    else if (value === true) element.setAttribute(name, "");
    else element.setAttribute(name, value);
  }
  for (const child of children) {
    if (child === false || child === null || child === undefined) continue;
    element.append(child);
  }
  return element;
}

const byId = (id: string): HTMLElement => document.getElementById(id) as HTMLElement;
const displayName = (ext: ExtensionInfo): string => ext.manifest.name || ext.manifest.id;
const ofType = (type: ExtensionInfo["manifest"]["type"]): ExtensionInfo[] =>
  (snapshot?.extensions ?? []).filter((ext) => ext.manifest.type === type);
const find = (id: string): ExtensionInfo | undefined => snapshot?.extensions.find((ext) => ext.manifest.id === id);
const nameOf = (id: string): string => {
  const ext = find(id);
  return ext === undefined ? id : displayName(ext);
};
const baseName = (path: string): string => path.split("/").pop() || path;

// ---------- status ----------

let statusTimer: ReturnType<typeof setTimeout> | undefined;

function showStatus(text: string, kind: "saved" | "error"): void {
  const status = byId("status");
  status.textContent = text;
  status.dataset.kind = kind;
  status.classList.add("visible");
  clearTimeout(statusTimer);
  if (kind === "saved") statusTimer = setTimeout(() => status.classList.remove("visible"), STATUS_CLEAR_MS);
}

/** Errors of the last change, by the id of the control that made it. Shown under that control. */
let errors: Record<string, string> = {};

/**
 * Runs one change made with the control `source`, then reads the settings
 * again, so the page always shows what is stored.
 */
async function change(method: string, params: Record<string, unknown>, source: string, timeoutMs?: number): Promise<unknown> {
  errors = {};
  try {
    const result = await bridge.call(method, params, timeoutMs);
    if ((result as PickResult | null)?.cancelled) return result;
    showStatus("Saved", "saved");
    return result;
  } catch (err) {
    errors[source] = err instanceof Error ? err.message : String(err);
    showStatus("Not saved", "error");
    return undefined;
  } finally {
    await refresh();
  }
}

// ---------- shared rows ----------

function group(title: string | undefined, rows: Node[], extra?: Node): HTMLElement {
  return h(
    "section",
    { class: "group" },
    title === undefined ? null : h("header", { class: "group-header" }, h("h2", {}, title), extra),
    h("div", { class: "panel" }, ...rows),
  );
}

function row(label: string, control: Node, options: { id?: string; description?: string } = {}): HTMLElement {
  return h(
    "div",
    { class: "row" },
    h(
      "div",
      { class: "row-text" },
      h("label", { class: "row-label", for: options.id }, label),
      options.description === undefined ? null : h("p", { class: "row-description" }, options.description),
      errorLine(options.id),
    ),
    h("div", { class: "row-control" }, control),
  );
}

/** The error of the last change made with the control `source`, if it failed. */
function errorLine(source: string | undefined): HTMLElement | null {
  const message = source === undefined ? undefined : errors[source];
  return message === undefined ? null : h("p", { class: "row-error", role: "alert" }, message);
}

function toggle(id: string, checked: boolean, onChange: (on: boolean) => void): HTMLInputElement {
  return h("input", {
    type: "checkbox",
    role: "switch",
    class: "switch",
    id,
    checked,
    onchange: (event: Event) => onChange((event.target as HTMLInputElement).checked),
  });
}

function button(label: string, onClick: () => void, kind: "primary" | "plain" | "danger" = "plain", id?: string): HTMLButtonElement {
  return h("button", { type: "button", class: `button ${kind}`, id, onclick: onClick }, label);
}

// ---------- Theme & background ----------

/** A small static picture of a theme: a dark screen with a clock in the theme's accent. */
function themeTile(theme: ExtensionInfo, selected: boolean): HTMLElement {
  const id = theme.manifest.id;
  const accent = String(theme.manifest.theme?.style?.["--accent"] ?? DEFAULT_ACCENT);
  const swatch = h("span", { class: "tile-screen", "aria-hidden": "true" }, h("span", { class: "tile-clock" }, "9:41"));
  swatch.style.setProperty("--tile-accent", /^#[0-9a-f]{3,8}$/i.test(accent) ? accent : DEFAULT_ACCENT);
  return h(
    "label",
    { class: "tile" },
    h("input", {
      type: "radio",
      name: "theme",
      id: `theme-${id}`,
      value: id,
      "aria-label": displayName(theme),
      checked: selected,
      onchange: () => void change("setTheme", { id }, `theme-${id}`),
    }),
    swatch,
    h("span", { class: "tile-name" }, displayName(theme)),
    h("span", { class: "tile-meta" }, nameOf(theme.manifest.theme?.background.id ?? "")),
  );
}

function appearanceView(): Node[] {
  const config = (snapshot as Snapshot).config;
  const themes = ofType("theme");
  const chosen = themes.some((theme) => theme.manifest.id === config.theme) ? config.theme : "builtin.default";
  const gallery = h(
    "div",
    { class: "tiles", role: "radiogroup", "aria-label": "Theme" },
    ...themes.map((theme) => themeTile(theme, theme.manifest.id === chosen)),
  );

  const themeBackground = find(chosen)?.manifest.theme?.background.id ?? "";
  const select = h(
    "select",
    { id: "background", onchange: (event: Event) => void change("setBackground", { id: (event.target as HTMLSelectElement).value }, "background") },
    h("option", { value: "", selected: config.background === "" }, `Theme's own (${nameOf(themeBackground)})`),
    ...ofType("background").map((ext) =>
      h("option", { value: ext.manifest.id, selected: config.background === ext.manifest.id }, displayName(ext)),
    ),
  );
  return [
    group("Theme", [gallery, ...themes.map((theme) => errorLine(`theme-${theme.manifest.id}`)).filter((line) => line !== null)]),
    group("Background", [
      row("Background", select, { id: "background", description: "Replaces the theme's background. Its settings are under Extension options." }),
    ]),
  ];
}

// ---------- Extension options ----------

function fieldControl(ext: ExtensionInfo, field: Field): Node {
  const extension = ext.manifest.id;
  const id = `option-${extension}-${field.key}`;
  const save = (value: string) => void change("setOption", { extension, key: field.key, value }, id);
  const read = (event: Event) => (event.target as HTMLInputElement).value;
  switch (field.control) {
    case "toggle":
      return toggle(id, field.value === true, (on) => save(String(on)));
    case "select":
      return h(
        "select",
        { id, onchange: (event: Event) => save(read(event)) },
        ...(field.choices ?? []).map((choice) => h("option", { value: choice, selected: choice === field.value }, choice)),
      );
    case "number":
      return h("input", {
        type: "number",
        id,
        class: "number",
        step: "any",
        min: field.min === undefined ? undefined : String(field.min),
        max: field.max === undefined ? undefined : String(field.max),
        value: String(field.value ?? ""),
        onchange: (event: Event) => save(read(event)),
      });
    case "color":
      return h("input", { type: "color", id, value: String(field.value ?? "#000000"), onchange: (event: Event) => save(read(event)) });
    case "file":
      return h(
        "span",
        { class: "file" },
        h("span", { class: "file-name" }, typeof field.value === "string" && field.value !== "" ? baseName(field.value) : "None"),
        button("Choose…", () => void change("setOption", { extension, key: field.key, pick: true }, id, 0), "plain", id),
      );
    default:
      return h("input", { type: "text", id, value: String(field.value ?? ""), onchange: (event: Event) => save(read(event)) });
  }
}

function optionsView(): Node[] {
  const withOptions = (snapshot as Snapshot).extensions
    .filter((ext) => Object.keys(ext.manifest.options ?? {}).length > 0)
    .sort((a, b) => displayName(a).localeCompare(displayName(b)));
  if (withOptions.length === 0) return [h("p", { class: "empty" }, "No installed extension has options.")];
  return withOptions.map((ext) => {
    const fields = formFor(ext.manifest.options ?? {}, ext.values);
    const rows: Node[] = fields.map((field) =>
      row(field.label, fieldControl(ext, field), { id: `option-${ext.manifest.id}-${field.key}`, description: rangeText(field) }),
    );
    const notice = notices[ext.manifest.id];
    if (notice) rows.push(h("p", { class: "notice", role: "note" }, notice));
    const resetId = `reset-${ext.manifest.id}`;
    const resetError = errorLine(resetId);
    if (resetError !== null) rows.unshift(resetError);
    const reset = button("Reset to defaults", () => void change("resetOption", { extension: ext.manifest.id, key: "" }, resetId), "plain", resetId);
    return group(`${displayName(ext)} · ${TYPE_NAMES[ext.manifest.type]}`, rows, reset);
  });
}

function rangeText(field: Field): string | undefined {
  if (field.min === undefined || field.max === undefined) return undefined;
  return `${field.min} to ${field.max}`;
}

// ---------- Extensions ----------

function extensionRow(ext: ExtensionInfo): HTMLElement {
  const id = ext.manifest.id;
  const meta = [id, ext.manifest.version, TYPE_NAMES[ext.manifest.type]].filter(Boolean).join(" · ");
  let action: Node;
  if (ext.builtin) {
    action = h("span", { class: "muted" }, "Built in");
  } else if (confirmingRemove === id) {
    action = h(
      "span",
      { class: "confirm" },
      button("Cancel", () => {
        confirmingRemove = undefined;
        render();
      }, "plain", `cancel-${id}`),
      button(`Remove ${displayName(ext)}`, () => {
        confirmingRemove = undefined;
        void change("removeExtension", { id }, `remove-${id}`);
      }, "danger", `confirm-${id}`),
    );
  } else {
    action = button("Remove…", () => {
      confirmingRemove = id;
      render();
      byId(`confirm-${id}`).focus();
    }, "plain", `remove-${id}`);
    (action as HTMLElement).setAttribute("aria-label", `Remove ${displayName(ext)}…`);
  }
  return h(
    "div",
    { class: "row" },
    h(
      "div",
      { class: "row-text" },
      h("span", { class: "row-label" }, displayName(ext)),
      h("p", { class: "row-description" }, meta),
      errorLine(`remove-${id}`),
    ),
    h("div", { class: "row-control" }, action),
  );
}

function extensionsView(): Node[] {
  const install = button("Install…", () => void installExtension(), "primary", "install");
  const sorted = [...(snapshot as Snapshot).extensions].sort(
    (a, b) => Number(a.builtin) - Number(b.builtin) || displayName(a).localeCompare(displayName(b)),
  );
  return [
    group(undefined, [row("Install an extension", install, { id: "install", description: "From a folder or a .zip file. Built-in extensions cannot be removed." })]),
    group("Installed", sorted.map(extensionRow)),
  ];
}

async function installExtension(): Promise<void> {
  const result = (await change("installExtension", {}, "install", 0)) as PickResult | undefined;
  if (result?.manifest !== undefined) showStatus(`Installed ${result.manifest.name || result.manifest.id}`, "saved");
}

// ---------- General ----------

function generalView(): Node[] {
  const config = (snapshot as Snapshot).config;
  const choices = FPS_CHOICES.includes(config.max_fps) ? FPS_CHOICES : [...FPS_CHOICES, config.max_fps].sort((a, b) => a - b);
  const fps = h(
    "select",
    { id: "max-fps", onchange: (event: Event) => void change("setMaxFps", { fps: Number((event.target as HTMLSelectElement).value) }, "max-fps") },
    ...choices.map((value) =>
      h("option", { value: String(value), selected: value === config.max_fps }, value === 0 ? "Display" : `${value} fps`),
    ),
  );
  const soundFile = h(
    "span",
    { class: "file" },
    h("span", { class: "file-name" }, config.sound_file ? baseName(config.sound_file) : "Default"),
    button("Choose…", () => void change("setSound", { on: config.sound, pick: true }, "sound-file", 0), "plain", "sound-file"),
  );
  return [
    group("Sound", [
      row("Play a sound when an agent needs approval", toggle("sound", config.sound, (on) => void change("setSound", { on }, "sound")), { id: "sound" }),
      row("Sound file", soundFile, { id: "sound-file" }),
    ]),
    group("Privacy", [
      row("Privacy mode", toggle("privacy", config.privacy, (on) => void change("setPrivacy", { on }, "privacy")), {
        id: "privacy",
        description: "Hides task titles and approval requests on the screen saver, and project names in the menu.",
      }),
    ]),
    group("Performance", [
      row("Maximum frame rate", fps, { id: "max-fps", description: "A ceiling for every background. Display follows the screen's refresh rate." }),
    ]),
  ];
}

// ---------- the live preview ----------

/**
 * One live preview: the real runtime in a frame, fed with state.json. It
 * exists only while the Theme view is open and the window is visible, so a
 * settings window in the background costs no animation frames.
 */
const preview = {
  frame: undefined as HTMLIFrameElement | undefined,
  timer: undefined as ReturnType<typeof setInterval> | undefined,
  lastState: undefined as string | undefined,

  sync(): void {
    const wanted = currentView === "appearance" && document.visibilityState === "visible";
    byId("preview").hidden = currentView !== "appearance";
    if (wanted && this.frame === undefined) this.start();
    if (!wanted && this.frame !== undefined) this.stop();
  },

  start(): void {
    const frame = h("iframe", { src: "../preview.html", title: "Live preview of the screen saver", tabindex: "-1" });
    frame.addEventListener("load", () => {
      this.lastState = undefined;
      void this.poll();
    });
    byId("preview").querySelector(".preview-screen")?.prepend(frame);
    this.frame = frame;
    this.timer = setInterval(() => void this.poll(), PREVIEW_POLL_MS);
  },

  stop(): void {
    clearInterval(this.timer);
    this.frame?.remove();
    this.frame = undefined;
    this.lastState = undefined;
  },

  async poll(): Promise<void> {
    const frame = this.frame;
    let state: string | null;
    try {
      state = await bridge.call<string | null>("state");
    } catch {
      return; // the next poll tries again
    }
    byId("preview-empty").hidden = state !== null;
    if (frame === undefined || frame !== this.frame || state === null || state === this.lastState) return;
    this.lastState = state;
    frame.contentWindow?.postMessage({ type: "agent-screensaver-state", state }, "*");
  },
};

function renderPreviewSummary(): void {
  const config = (snapshot as Snapshot).config;
  const theme = find(config.theme) ?? find("builtin.default");
  byId("preview-theme").textContent = theme === undefined ? "No theme" : displayName(theme);
  const parts = [`Background: ${nameOf((snapshot as Snapshot).background)}`];
  const layout = theme?.manifest.theme?.layout;
  if (layout) parts.push(`Layout: ${nameOf(layout)}`);
  byId("preview-detail").textContent = parts.join(" · ");
}

// ---------- rendering ----------

const VIEWS: Record<View, () => Node[]> = {
  appearance: appearanceView,
  options: optionsView,
  extensions: extensionsView,
  general: generalView,
};

/**
 * Draws the current view from the snapshot. The view is rebuilt whole, so
 * focus and scroll are carried over by element id: a keyboard user stays on
 * the control they just changed.
 */
function render(): void {
  if (snapshot === undefined) return;
  const focusedId = document.activeElement?.id;
  const content = document.querySelector(".content") as HTMLElement;
  const scroll = content.scrollTop;
  byId("view-title").textContent = VIEW_TITLES[currentView];
  for (const link of Array.from(document.querySelectorAll<HTMLButtonElement>(".sidebar button"))) {
    if (link.dataset.view === currentView) link.setAttribute("aria-current", "page");
    else link.removeAttribute("aria-current");
  }
  byId("view").replaceChildren(...VIEWS[currentView]());
  if (currentView === "appearance") renderPreviewSummary();
  preview.sync();
  content.scrollTop = scroll;
  if (focusedId) document.getElementById(focusedId)?.focus();
}

async function refresh(): Promise<void> {
  try {
    snapshot = await bridge.call<Snapshot>("snapshot");
    const withOptions = snapshot.extensions.filter((ext) => Object.keys(ext.manifest.options ?? {}).length > 0);
    const answers = await Promise.all(withOptions.map((ext) => bridge.call<string>("notice", { extension: ext.manifest.id })));
    notices = Object.fromEntries(withOptions.map((ext, index) => [ext.manifest.id, answers[index] ?? ""]));
  } catch (err) {
    showStatus(err instanceof Error ? err.message : String(err), "error");
  }
  render();
}

function show(view: View): void {
  if (view === currentView) return;
  currentView = view;
  confirmingRemove = undefined;
  errors = {};
  byId("status").classList.remove("visible");
  (document.querySelector(".content") as HTMLElement).scrollTop = 0;
  render();
}

for (const link of Array.from(document.querySelectorAll<HTMLButtonElement>(".sidebar button"))) {
  link.addEventListener("click", () => show(link.dataset.view as View));
}
document.addEventListener("visibilitychange", () => preview.sync());
// The menu bar app and the CLI change settings too; read them again when the window comes back.
window.addEventListener("focus", () => void refresh());
// WebKit focuses the first control when the window opens, with a keyboard
// ring; nothing should look selected by the keyboard before a key is pressed.
void refresh().then(() => (document.activeElement as HTMLElement | null)?.blur());
