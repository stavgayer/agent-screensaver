import { createBridge } from "./bridge.ts";
import { formFor, type Field, type OptionSpec } from "./form.ts";
import { withProject, withoutProject } from "./projects.ts";
import { PREVIEW_DATA_URL, dataFolderUrl, previewState, serviceRunning } from "./preview-url.ts";
import { connectionRows, focusAfterPlan, hookButtonId, isPlanChanged, planDialog, powerButtonId, type Connection, type ConnectionRow, type HookAction, type Plan } from "./connections.ts";

/** The `snapshot` answer (Go's settings.Snapshot). */
interface Snapshot {
  config: {
    theme: string;
    background: string;
    sound: boolean;
    sound_file: string;
    privacy: boolean;
    max_fps: number;
    /** Folder names whose sessions get no card. */
    ignored_projects: string[] | null;
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

type View = "appearance" | "options" | "extensions" | "connections" | "general";

const VIEW_TITLES: Record<View, string> = {
  appearance: "Theme & background",
  options: "Extension options",
  extensions: "Extensions",
  connections: "Connections",
  general: "General",
};
const TYPE_NAMES = { background: "Background", component: "Component", layout: "Layout", theme: "Theme" };
const FPS_CHOICES = [0, 15, 24, 30, 60, 120, 240];
const PREVIEW_POLL_MS = 1000;
// The Connections tab reads the agents' states again this often while it is open.
const CONNECTIONS_POLL_MS = 5000;
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
/** The agent extensions, read only while the Connections view is open. */
let connections: Connection[] = [];
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

/** A small static picture of a theme: a screen with a clock in the theme's accent. Black, or a dusk sky for the aerials background. */
function themeTile(theme: ExtensionInfo, selected: boolean): HTMLElement {
  const id = theme.manifest.id;
  const accent = String(theme.manifest.theme?.style?.["--accent"] ?? DEFAULT_ACCENT);
  const swatch = h("span", { class: "tile-screen", "aria-hidden": "true" }, h("span", { class: "tile-clock" }, "9:41"));
  swatch.dataset.background = theme.manifest.theme?.background.id ?? "";
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

// ---------- Connections ----------

/** The mark is shown only from the data folder, where Go found it. */
function markImage(row: ConnectionRow): HTMLElement {
  const slot = h("span", { class: "connection-mark", "aria-hidden": "true" });
  if (row.mark.startsWith(dataFolderUrl(location.href))) slot.append(h("img", { src: row.mark, alt: "", width: "24", height: "24" }));
  return slot;
}

function connectionRow(row: ConnectionRow): HTMLElement {
  const hookId = hookButtonId(row.id);
  const powerId = powerButtonId(row.id);
  const hook = row.hook === null
    ? h("span", { class: "connection-spacer" })
    : button(row.hook.label, () => void openPlan(row, (row.hook as { action: HookAction }).action), "plain", hookId);
  const power = row.power === null
    ? h("span", { class: "connection-spacer" })
    : button(row.power.label, () => void change("setAgentOn", { id: row.id, on: (row.power as { on: boolean }).on }, powerId), "plain", powerId);
  if (row.hook !== null) hook.setAttribute("aria-label", `${row.hook.label} for ${row.name}`);
  if (row.power !== null) power.setAttribute("aria-label", `${row.power.label} ${row.name}`);
  const state = h("span", { class: "state-pill" }, row.stateText);
  state.dataset.state = row.state;
  return h(
    "div",
    { class: "row connection" },
    markImage(row),
    h(
      "div",
      { class: "row-text" },
      h("span", { class: "row-label" }, row.name),
      h("p", { class: "row-description" }, `${capitalized(row.badge)} · ${row.lastEvent}`),
      row.changed === null ? null : h("p", { class: "row-warning" }, ...keepWhole(`${row.changed}.`, "agent-screensaver")),
      errorLine(hookId),
      errorLine(powerId),
    ),
    h("div", { class: "row-control connection-controls" }, state, hook, power),
  );
}

/** Text with one word kept on one line, so "agent-screensaver" never breaks at its hyphen. */
function keepWhole(text: string, word: string): Child[] {
  const at = text.indexOf(word);
  if (at < 0) return [text];
  return [text.slice(0, at), h("span", { class: "nowrap" }, word), text.slice(at + word.length)];
}

const capitalized = (text: string): string => text.charAt(0).toUpperCase() + text.slice(1);

function connectionsView(): Node[] {
  const rows = connectionRows(connections, Date.now());
  const list: Node[] = rows.length === 0
    ? [h("p", { class: "panel-note" }, "No agent extensions are installed.")]
    : rows.map(connectionRow);
  list.unshift(
    h("p", { class: "panel-note" }, "Each agent reports to the screen saver through hooks in its own settings file. Turning an agent off keeps its hooks; it goes quiet until you turn it on again."),
  );
  const install = button("Install…", () => void installExtension(), "plain", "install-agent");
  return [
    group("Agents", list),
    group(undefined, [row("Add another agent", install, { id: "install-agent", description: "Install its extension from a folder or a .zip file. It shows here with Add hooks." })]),
  ];
}

// ---------- the hook plan dialog ----------

/** Shows the plan Go computed. Nothing changes until the confirm button is pressed. */
async function openPlan(row: ConnectionRow, action: HookAction, notice?: string): Promise<void> {
  const source = hookButtonId(row.id);
  errors = {};
  let plan: Plan;
  try {
    plan = await bridge.call<Plan>("planAgentHooks", { id: row.id, add: action === "add" });
  } catch (err) {
    errors[source] = err instanceof Error ? err.message : String(err);
    showStatus("Not saved", "error");
    render();
    return;
  }
  showPlanDialog(row, action, plan, notice);
}

function showPlanDialog(row: ConnectionRow, action: HookAction, plan: Plan, notice?: string): void {
  const content = planDialog(row.name, action, plan);
  const dialog = h("dialog", { class: "plan-dialog", "aria-labelledby": "plan-title" });
  const close = () => dialog.close();
  const cancel = button("Cancel", close, "plain", "plan-cancel");
  const confirm = button(content.confirmLabel, () => void applyPlan(row, action, plan, dialog, close), "primary", "plan-confirm");
  const parts: Array<Node | null> = [
    h("h2", { id: "plan-title" }, content.title),
    notice === undefined ? null : h("p", { class: "plan-notice", role: "alert" }, notice),
    content.trustNote === null ? null : h("p", { class: "plan-trust" }, content.trustNote),
    h("p", { class: "plan-intro" }, "This will:"),
    h("ul", { class: "plan-lines" }, ...content.lines.map((line) => h("li", { class: line.warning ? "warning" : "" }, line.text))),
    h("p", { class: "plan-hint" }, `Nothing changes until you press ${content.confirmLabel}.`),
    h("div", { class: "plan-buttons" }, cancel, confirm),
  ];
  dialog.append(...parts.filter((part): part is Node => part !== null));
  // Cancel, Escape and confirm all end here. Focus goes back to the row,
  // now and after the redraw that follows a change.
  dialog.addEventListener("close", () => {
    dialog.remove();
    focusAfterClose = row.id;
    restoreFocus();
  });
  document.body.append(dialog);
  dialog.showModal();
  cancel.focus();
}

/**
 * Applies the plan the dialog shows, by its hash. When a file changed in
 * between, Go refuses, and the dialog shows the new plan instead.
 */
async function applyPlan(row: ConnectionRow, action: HookAction, plan: Plan, dialog: HTMLDialogElement, close: () => void): Promise<void> {
  for (const control of Array.from(dialog.querySelectorAll("button"))) control.disabled = true;
  errors = {};
  try {
    await bridge.call("applyAgentHooks", { id: row.id, add: action === "add", hash: plan.hash });
    close();
    showStatus(action === "add" ? `Added hooks for ${row.name}` : `Removed hooks for ${row.name}`, "saved");
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    close();
    if (isPlanChanged(message)) {
      await openPlan(row, action, "A file changed while this plan was open. This is the new plan.");
      return;
    }
    errors[hookButtonId(row.id)] = message;
    showStatus("Not saved", "error");
  }
  await refresh();
}

/** The agent whose plan sheet just closed: the next draw puts focus back on its row. */
let focusAfterClose: string | undefined;

function restoreFocus(): void {
  if (focusAfterClose === undefined || document.querySelector("dialog[open]") !== null) return;
  const target = focusAfterPlan(focusAfterClose, (id) => document.getElementById(id) !== null);
  if (target === undefined) return; // the view is not drawn yet; the next draw tries again
  document.getElementById(target)?.focus();
  focusAfterClose = undefined;
}

/** Reads the agents again every few seconds while the Connections view is open and visible. */
const connectionsPoll = {
  timer: undefined as ReturnType<typeof setInterval> | undefined,

  sync(): void {
    const wanted = currentView === "connections" && document.visibilityState === "visible";
    if (wanted && this.timer === undefined) this.timer = setInterval(() => void refresh("outside"), CONNECTIONS_POLL_MS);
    if (!wanted && this.timer !== undefined) {
      clearInterval(this.timer);
      this.timer = undefined;
    }
  },
};

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
    ignoredProjectsGroup(),
    group("Performance", [
      row("Maximum frame rate", fps, { id: "max-fps", description: "A ceiling for every background. Display follows the screen's refresh rate." }),
    ]),
  ];
}

/**
 * The ignored-projects editor: one row per folder with its Remove button, and
 * a row to add one. Each change saves the whole list.
 */
/** What is typed in the add field, kept across redraws so an error does not wipe it. */
let ignoreDraft = "";

function ignoredProjectsGroup(): HTMLElement {
  const projects = (snapshot as Snapshot).config.ignored_projects ?? [];
  const save = (next: string[], source: string) => void change("setIgnoredProjects", { projects: next }, source);
  const add = () => {
    const result = withProject(projects, ignoreDraft);
    if ("error" in result) {
      errors = { "ignore-new": result.error };
      render();
      return;
    }
    ignoreDraft = "";
    save(result.projects, "ignore-new");
  };
  const rows: Node[] = projects.map((name) => {
    const id = `ignore-remove-${name}`;
    const remove = button("Remove", () => save(withoutProject(projects, name), id), "plain", id);
    remove.setAttribute("aria-label", `Stop ignoring ${name}`);
    return row(name, remove, { id });
  });
  const input = h("input", {
    type: "text",
    id: "ignore-new",
    placeholder: "Folder name",
    value: ignoreDraft,
    oninput: (event: Event) => {
      ignoreDraft = (event.target as HTMLInputElement).value;
    },
    onkeydown: (event: Event) => {
      if ((event as KeyboardEvent).key === "Enter") add();
    },
  });
  rows.unshift(
    h("p", { class: "panel-note" }, "Sessions in these folders never show a card, ask for approval or play the sound. observer-sessions is where the claude-mem plugin runs its hidden sessions."),
  );
  rows.push(
    row("Add a folder", h("span", { class: "file" }, input, button("Add", add, "plain", "ignore-add")), {
      id: "ignore-new",
      description: "The folder's name, or its path. Matches the session's folder name exactly.",
    }),
  );
  return group("Ignored projects", rows);
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
    const frame = h("iframe", {
      src: `${PREVIEW_DATA_URL}runtime/preview.html`,
      title: "Live preview of the screen saver",
      tabindex: "-1",
      // Sandboxed and served from the preview scheme: an opaque origin, so
      // extension scripts in the frame cannot reach this page or its bridge
      // to Go (parent.webkit), whatever WebKit's file:// rules are.
      sandbox: "allow-scripts",
    });
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
    byId("preview-empty").hidden = serviceRunning(state);
    if (frame === undefined || frame !== this.frame || state === null || state === this.lastState) return;
    this.lastState = state;
    // "*": the frame's origin is opaque, so no named target matches it. Only
    // state goes in; this page listens to no message from the frame.
    const dataUrl = dataFolderUrl(location.href);
    frame.contentWindow?.postMessage({ type: "agent-screensaver-state", state: previewState(state, dataUrl), dataUrl }, "*");
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
  connections: connectionsView,
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
  connectionsPoll.sync();
  content.scrollTop = scroll;
  if (focusedId) document.getElementById(focusedId)?.focus();
  else restoreFocus();
}

/**
 * Reads the settings again and draws them. After a change the view is always
 * drawn (its error lines may differ); otherwise it is drawn only when the
 * settings changed, so a refresh never rebuilds a control under the pointer.
 */
async function refresh(after: "change" | "outside" = "change"): Promise<void> {
  // The connection rows are compared as text, so "4m ago" turning into "5m ago" redraws them.
  const drawn = () => JSON.stringify([snapshot, notices, connectionRows(connections, Date.now())]);
  const before = drawn();
  try {
    const next = await bridge.call<Snapshot>("snapshot");
    const withOptions = next.extensions.filter((ext) => Object.keys(ext.manifest.options ?? {}).length > 0);
    const answers = await Promise.all(withOptions.map((ext) => bridge.call<string>("notice", { extension: ext.manifest.id })));
    if (currentView === "connections") connections = await bridge.call<Connection[]>("connections");
    snapshot = next;
    notices = Object.fromEntries(withOptions.map((ext, index) => [ext.manifest.id, answers[index] ?? ""]));
  } catch (err) {
    showStatus(err instanceof Error ? err.message : String(err), "error");
  }
  // A redraw from outside never rebuilds the page under an open plan dialog.
  if (after === "outside" && (drawn() === before || document.querySelector("dialog[open]") !== null)) return;
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
  if (view === "connections") void refresh();
}

for (const link of Array.from(document.querySelectorAll<HTMLButtonElement>(".sidebar button"))) {
  link.addEventListener("click", () => show(link.dataset.view as View));
}
// The menu bar app and the CLI change settings too: read them again when the
// window is shown again.
document.addEventListener("visibilitychange", () => {
  preview.sync();
  connectionsPoll.sync();
  if (document.visibilityState === "visible") void refresh("outside");
});
// WebKit focuses the first control when the window opens, with a keyboard
// ring; nothing should look selected by the keyboard before a key is pressed.
void refresh().then(() => (document.activeElement as HTMLElement | null)?.blur());
