// A small fake page for tests that run a component's entry file: elements with
// classes, text and children, a fake FontFace and a register hook. Entry files
// register themselves on import, so each test file loads one entry once.
import type { ComponentExt, Ctx } from "../src/api.ts";

export class FakeElement {
  readonly tag: string;
  readonly children: FakeElement[] = [];
  private readonly properties = new Map<string, string>();
  readonly style = {
    setProperty: (name: string, value: string) => void this.properties.set(name, value),
    get: (name: string) => this.properties.get(name),
  };
  readonly classes = new Set<string>();
  readonly attributes = new Map<string, string>();
  parentElement: FakeElement | undefined;
  textContent = "";
  dir = "";
  readonly dataset: Record<string, string> = {};
  /** Markup set with innerHTML is kept as text, not parsed. */
  innerHTML = "";
  src = "";
  alt = "";
  constructor(tag: string) {
    this.tag = tag;
  }
  setAttribute(name: string, value: string): void {
    if (name === "class") this.className = value;
    else this.attributes.set(name, value);
  }
  getAttribute(name: string): string | null {
    return name === "class" ? this.className : (this.attributes.get(name) ?? null);
  }
  get className(): string {
    return [...this.classes].join(" ");
  }
  set className(value: string) {
    this.classes.clear();
    for (const name of value.split(/\s+/)) if (name !== "") this.classes.add(name);
  }
  readonly classList = {
    add: (name: string) => void this.classes.add(name),
    contains: (name: string) => this.classes.has(name),
    toggle: (name: string, on?: boolean) => {
      const want = on ?? !this.classes.has(name);
      if (want) this.classes.add(name);
      else this.classes.delete(name);
      return want;
    },
  };
  append(...nodes: FakeElement[]): void {
    for (const node of nodes) {
      node.parentElement = this;
      this.children.push(node);
    }
  }
  prepend(...nodes: FakeElement[]): void {
    for (const node of nodes) node.parentElement = this;
    this.children.unshift(...nodes);
  }
  toggleAttribute(name: string, on?: boolean): boolean {
    const want = on ?? !this.attributes.has(name);
    if (want) this.attributes.set(name, "");
    else this.attributes.delete(name);
    return want;
  }
  /** The first element below this one that matches a single class selector such as ".headline". */
  querySelector(selector: string): FakeElement | null {
    return this.find(selector.replace(/^\./, ""))[0] ?? null;
  }
  appendChild(node: FakeElement): FakeElement {
    this.append(node);
    return node;
  }
  replaceChildren(...nodes: FakeElement[]): void {
    for (const node of this.children) node.parentElement = undefined;
    this.children.length = 0;
    this.append(...nodes);
  }
  /** Every element below this one, depth first. */
  all(): FakeElement[] {
    return this.children.flatMap((child) => [child, ...child.all()]);
  }
  /** Elements below this one with a class. */
  find(name: string): FakeElement[] {
    return this.all().filter((element) => element.classes.has(name));
  }
  /** The text of this element and everything below it. */
  text(): string {
    return this.textContent + this.children.map((child) => child.text()).join("");
  }
}

/** A FontFace whose load fails, so the component keeps the system font. */
class FailingFontFace {
  async load(): Promise<never> {
    throw new Error("no font in tests");
  }
}

const globals = globalThis as unknown as Record<string, unknown>;

/** Imports an entry file on the fake page and returns what it registered. */
export async function loadComponent(scriptSrc: string, importEntry: () => Promise<unknown>): Promise<() => ComponentExt> {
  let registered: (() => ComponentExt) | undefined;
  globals.window = { agentScreensaver: { register: (create: () => ComponentExt) => (registered = create) } };
  globals.document = {
    currentScript: { src: scriptSrc },
    fonts: { add() {} },
    createElement: (tag: string) => new FakeElement(tag),
    createElementNS: (_namespace: string, tag: string) => new FakeElement(tag),
  };
  globals.FontFace = FailingFontFace;
  await importEntry();
  if (registered === undefined) throw new Error("the entry file did not register");
  return registered;
}

/** A ctx with a frame loop that runs only when the test calls `tick`. */
export function fakeCtx(nowMs: number, options: Record<string, unknown> = {}): { ctx: Ctx; tick: (atMs: number) => void; frames: { started: number; cancelled: number } } {
  const callbacks: Array<(nowMs: number) => void> = [];
  const frames = { started: 0, cancelled: 0 };
  let now = nowMs;
  const ctx: Ctx = {
    options,
    display: "main",
    now: () => now,
    requestFrame(cb) {
      frames.started++;
      callbacks.push(cb);
      return () => void frames.cancelled++;
    },
  };
  const tick = (atMs: number): void => {
    now = atMs;
    for (const cb of callbacks) cb(atMs);
  };
  return { ctx, tick, frames };
}

/** Lets pending promises (a font load) settle. */
export const settle = (): Promise<void> => new Promise((resolve) => setTimeout(resolve, 0));
