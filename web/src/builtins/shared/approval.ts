import type { Card, ComponentExt, ViewState } from "../../api.ts";
import { approvalQueue } from "../../mode.ts";

/** What the card shows for the approval on screen: `key` says when it must be redrawn. */
export interface ApprovalContent {
  key: string;
  build(): HTMLElement[];
}

/** What makes one approval card look different from another. */
export interface ApprovalLook {
  /** The component's stylesheet. The card is `.approval`, hidden while nothing waits. */
  css: string;
  /** The card's lines for the oldest approval and the number queued behind it. */
  content(first: Card, more: number, state: ViewState): ApprovalContent;
  /** Optional. Called once the card is on the page, for example to load a font. */
  onMount?(box: HTMLElement): void;
}

/**
 * A component that shows the oldest open approval, from approvalQueue. The built-in
 * approval card and the chrome approval share it and differ only in their look.
 */
export function createApprovalComponent(look: ApprovalLook): ComponentExt {
  let box: HTMLElement | undefined;
  let lastKey = "";

  function mount(root: HTMLElement): void {
    const style = document.createElement("style");
    style.textContent = look.css;
    box = document.createElement("div");
    box.className = "approval";
    root.append(style, box);
    look.onMount?.(box);
  }

  function update(state: ViewState): void {
    const { first, more } = approvalQueue(state.cards);
    box?.toggleAttribute("hidden", first === undefined);
    if (first === undefined) return;
    const content = look.content(first, more, state);
    if (content.key === lastKey) return;
    lastKey = content.key;
    box?.replaceChildren(...content.build());
  }

  function unmount(): void {
    box?.parentElement?.replaceChildren();
    box = undefined;
  }

  return { mount, update, unmount };
}
