import type { Card, ViewState } from "../../api.ts";
import { agentName } from "../../format.ts";
import { approvalMoreLabel, approvalWaitingLabel } from "../approval-card/logic.ts";
import { loadFont } from "../shared/fonts.ts";
import { createApprovalComponent, type ApprovalContent } from "../shared/approval.ts";

const DISPLAY_FAMILY = "Chrome Approval Anton";
const LABEL_FAMILY = "Chrome Approval Mono";

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

// The approval of builtin.approval-card (same queue and words), set like the rest of
// the Chrome screen: a line of monospace labels (the agent at one end, the wait at
// the other), the headline in the clock's condensed face, then the project and the
// request as a small labelled sheet. The card's own border breathes: a second border
// laid exactly on it fades in and out (opacity is composited, so the large card is
// painted once). Names break between words; only the command may break anywhere.
// Under reduced motion the border stays lit. The token block holds every color and
// face; the theme can set --attention. The headline stays on one line at any aspect
// ratio, so the card keeps its height between the lifted shape and the signs; the
// wait keeps its own case ("<1m" is minutes, not months).
const STYLE = `
:root { --chrome-approval-ink: #eef1f4; --chrome-approval-pink: #ff3d9a; --chrome-approval-plate: rgba(16, 7, 12, 0.86);
  --chrome-approval-display: "${DISPLAY_FAMILY}", "Helvetica Neue", sans-serif;
  --chrome-approval-mono: "${LABEL_FAMILY}", ui-monospace, Menlo, monospace; }
.approval { --tone: var(--attention, var(--chrome-approval-pink)); position: relative; width: 58vw; max-width: 124vmin;
  padding: 2.6vmin 3.4vmin 3vmin; border-radius: 0.5vmin;
  border: 2px solid color-mix(in srgb, var(--tone) 38%, transparent);
  background: color-mix(in srgb, var(--tone) 7%, var(--chrome-approval-plate)); }
.approval::after { content: ""; position: absolute; inset: -2px; border: 2px solid var(--tone); border-radius: inherit; pointer-events: none;
  opacity: 0; animation: chrome-approval-pulse 1.6s cubic-bezier(0.45, 0, 0.55, 1) infinite; will-change: opacity; }
@keyframes chrome-approval-pulse { 50% { opacity: 1; } }
.approval > div { overflow-wrap: break-word; }
.approval .meta { display: flex; justify-content: space-between; gap: 2em; font-family: var(--chrome-approval-mono); font-size: 2vmin;
  font-weight: 700; letter-spacing: 0.08em; text-transform: uppercase; color: var(--tone); font-variant-numeric: tabular-nums; }
.approval .meta > .agent { flex: 1 1 auto; }
.approval .meta > span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.approval .meta > .waiting { flex: none; color: var(--chrome-approval-ink); font-weight: 500; text-transform: none; letter-spacing: 0.02em; }
.approval .headline { margin: 1.2vmin 0 1.8vmin; font-family: var(--chrome-approval-display); font-size: min(12.5vmin, 8vw); font-weight: 400; line-height: 0.9; white-space: nowrap;
  letter-spacing: 0.005em; text-transform: uppercase; color: var(--tone); }
.approval .facts { display: grid; grid-template-columns: max-content minmax(0, 1fr); column-gap: 2.6vmin; row-gap: 1vmin; align-items: baseline; }
.approval .label { font-family: var(--chrome-approval-mono); font-size: 1.7vmin; letter-spacing: 0.1em; text-transform: uppercase;
  color: color-mix(in srgb, var(--tone) 55%, var(--chrome-approval-ink)); }
.approval .where { font-size: 3.2vmin; font-weight: 600; line-height: 1.2; color: var(--chrome-approval-ink); overflow-wrap: break-word; }
.approval .request { font-family: var(--chrome-approval-mono); font-size: 2.6vmin; line-height: 1.35; color: var(--chrome-approval-ink);
  overflow-wrap: anywhere; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; }
.approval .meta > .more { flex: none; margin-inline-start: -0.8em; color: var(--tone); }
@media (prefers-reduced-motion: reduce) { .approval::after { animation: none; opacity: 1; } }
`;

function element(tag: string, className: string, text = ""): HTMLElement {
  const made = document.createElement(tag);
  made.className = className;
  made.textContent = text;
  // Project names and requests can be Hebrew or Arabic; let the text pick its own direction.
  if (text !== "") made.dir = "auto";
  return made;
}

function content(first: Card, more: number, state: ViewState): ApprovalContent {
  const agent = agentName(first.source, state.agents);
  const waiting = approvalWaitingLabel(first, state.nowMs);
  // Privacy mode removes the request (and may remove the project); skip empty rows.
  const facts = [
    ["Project", "where", first.project ?? ""],
    ["Request", "request", first.request ?? ""],
  ].filter(([, , text]) => text !== "");
  const moreLabel = approvalMoreLabel(more);
  return {
    key: JSON.stringify([agent, waiting, facts, moreLabel]),
    build() {
      // The queue count shares the label line, so a queue does not make the card taller.
      const meta = element("div", "meta");
      meta.append(element("span", "agent", agent), element("span", "waiting", waiting));
      if (moreLabel !== "") meta.append(element("span", "more", moreLabel));
      const lines = [meta, element("div", "headline", "Needs approval")];
      if (facts.length > 0) {
        const sheet = element("div", "facts");
        for (const [label, className, text] of facts) sheet.append(element("span", "label", label), element("span", className, text));
        lines.push(sheet);
      }
      return lines;
    },
  };
}

function loadFaces(): void {
  if (scriptSrc === "") return;
  void loadFont(DISPLAY_FAMILY, new URL("fonts/Anton-Regular.woff2", scriptSrc).href);
  void loadFont(LABEL_FAMILY, new URL("fonts/JetBrainsMono.woff2", scriptSrc).href, { weight: "100 800" });
}

window.agentScreensaver.register(() => createApprovalComponent({ css: STYLE, content, onMount: loadFaces }));
