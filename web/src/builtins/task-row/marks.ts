// The agent marks: simple drawn shapes of our own, not the products' logos (their
// brand terms do not allow them in a third-party screen). Each shape is a mask
// painted in one color, so a theme can recolor it and replace it without code:
//   "--source-color": the color of every mark (default: the card's muted white)
//   "--mark-claude", "--mark-codex", "--mark-cursor", "--mark-other": a CSS image,
//     for example url("data:image/svg+xml,..."), used as the mask of that mark
// A new source with a mark of its own needs one entry here and its id in logic.ts.

const SHAPES: Record<string, string> = {
  // A speech bubble: a conversation.
  claude: `<path d="M5.5 3.5h13a3 3 0 0 1 3 3v8.5a3 3 0 0 1-3 3H12l-5 3.8v-3.8H5.5a3 3 0 0 1-3-3V6.5a3 3 0 0 1 3-3z"/>`,
  // A terminal prompt.
  codex: `<path d="M4 5.5 11 12l-7 6.5" fill="none" stroke="#000" stroke-width="3.4" stroke-linecap="round" stroke-linejoin="round"/><path d="M13.5 18.5h7" stroke="#000" stroke-width="3.4" stroke-linecap="round"/>`,
  // A pointer.
  cursor: `<path d="M5 2.5 20 11l-6.6 1.9L10.2 19.5z" stroke="#000" stroke-width="1.6" stroke-linejoin="round"/>`,
  // Any other agent: a ring.
  other: `<circle cx="12" cy="12" r="7" fill="none" stroke="#000" stroke-width="3.4"/>`,
};

function maskUrl(shape: string): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">${shape}</svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
}

const perSource = Object.entries(SHAPES)
  .map(([id, shape]) => `.task .source[data-source="${id}"] .source-mark { --mark: var(--mark-${id}, ${maskUrl(shape)}); }`)
  .join("\n");

export const SOURCE_MARK_STYLE = `
.task .source-mark { flex: none; width: 1.1em; height: 1.1em; background: var(--source-color, currentColor);
  -webkit-mask: var(--mark) center / contain no-repeat; mask: var(--mark) center / contain no-repeat; }
${perSource}
`;
