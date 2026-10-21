# Screen design notes

The screen is an ambient display. People read it from about 3 m away, often for one second. It has two states (spec §3, mockup `docs/superpowers/specs/mockups/layout-b-clock.html`):

- **Idle:** a big clock and date in the center, limit rings top right, task cards at the bottom.
- **Attention:** the clock moves to the top left, an orange approval card pulses in the center.

The layout, the regions and the Matrix rain stay as the owner chose them. This file records a design review of the built-ins and the small changes that came out of it.

Sources for the criteria: the `emil-design-eng` skill (motion and polish), the `impeccable` skill (polish pass, craft floor) and the `design-taste-frontend` skill (its app-screen rules, because this screen is worked with, not scrolled).

## The question the screen answers

"Does an agent need me, and if not, how are my agents and limits doing?" In idle the biggest thing is the clock, because nothing needs you. In attention the biggest thing is the approval card. That hierarchy is right and stays.

## Critique of the current screen

### Hierarchy

- Good: one dominant element per state. The approval card wins the center when it matters.
- Weak: the approval card is small at large sizes. "Needs approval" is 4vmin, close to the clock's corner size, so the most important words on the screen are not much bigger than the second-tier text.
- Weak: on every card, the small meta line ("CODEX · RUNNING 24M") is in tracked capitals. Capitals read as shouting labels and are slower to read from a distance than sentence case. The app-screen rules allow at most one tracked uppercase label per screen; here there are seven.

### Typography

- The font stack is `-apple-system`, which gives SF Pro on the Mac. That is the right face: it is the system face, it has true tabular figures, and WebKit switches to the Display cut at large sizes.
- The clock uses tabular figures, so the digits do not jump. The task cards and rings do not: "running 24m" and the ring percents change width as they count.
- The clock time has no negative tracking. At 16vmin, SF Pro Display looks loose without about -0.02em.
- Ring captions are 1.6vmin capitals with a 14vmin max width, so "CLAUDE 5H · 1H40M" and "CODEX PLAN · RESETS 3 NOV" wrap in the middle of the phrase. The name and the time read as one broken line.
- Long task titles and requests wrap with `overflow-wrap: anywhere`, so a long title grows the card and pushes the row up (layout shift). Hebrew titles are laid out left to right.

### Color and contrast on the rain

- The clock has a green glow (`text-shadow` in the accent). A glow on light text over green rain lowers the edge contrast; it reads as soft, not as bright. A dark halo separates the digits from the rain better.
- Secondary text uses `opacity: 0.65` on white. On the card surface that is about 7:1, which is fine, but the value is set four different ways across the files.
- Task cards let the rain show through at 0.82. That is the theme's `--card-opacity` and stays; the text on it keeps more than 7:1.
- The approval card has a permanent orange glow plus a pulse that animates `box-shadow` on a 55vw card. The glow looks soft and costs a repaint of a large area on every frame.

### Spacing rhythm

- The outer gap (4vmin) is used everywhere. Good.
- Inside the cards the padding (1.6vmin by 2vmin) is tight for text read from a distance, and the three lines sit with no rhythm (meta, then title, then project, all at 1.35 line height).

### Cards

- Task cards: 1px border in a tint of the accent, soft radius, dark tinted surface. This follows the app-screen rules (a border or a shadow, never both) and stays.
- The finished card uses a "✓" text glyph before the title, and the approval headline a "⚠". Text glyphs change with the font and can render as emoji. The marks stay (they are in the approved mockup) but should be drawn.
- The approval card centers all its text. That is right for one card in the middle of the screen.

### Rings

- Thick conic rings with the percent inside. Readable from a distance. Ring color follows the spec (green, yellow, red).
- The percent inside is 2vmin regular. It is the number people look for, so it should be heavier and tabular.

### Motion

- Clock move: the runtime plays a FLIP move (transform only, 400 ms). But the clock also has `transition: font-size 400ms`. When the clock changes size, the runtime measures it before the font has shrunk, so the FLIP plays a move with almost no scale and then the font size animates on its own. That is two animations fighting, and the font-size one changes layout on every frame.
- The clock box also changes its shape between states (the date is hidden in the small clock), so the FLIP has to scale width and height by different amounts, which squashes the digits during the move.
- The clock writes its text every second even when the minute has not changed, which repaints it for nothing.
- Approval pulse: 1.6 s, ease-in-out, animating `box-shadow`. The period is good for attention without alarm. The property is the wrong one.
- Region entrance: 0.6 s with the default `ease`, from 1vmin below. The default `ease` is weak; a strong ease-out feels more deliberate.
- `prefers-reduced-motion` is honored by the clock, the pulse and the entrance.

### Accessibility

- Contrast passes on the cards. The ring captions at 0.65 opacity on the rain are the weakest text; the ring inside is black, so the percent always passes.
- No text sets a direction, so Hebrew and other right-to-left titles are shown left to right with the ellipsis on the wrong side.

## What changes

Small, focused changes. No new regions, no new components, no change to the public API or theme variable names.

1. Clock: remove the font-size transition so only the runtime's transform move plays; keep the box the same shape in both states (the date hangs below the time and fades out in the corner), so the FLIP scales evenly; a dark halo instead of the green glow; -0.02em tracking; write the text only when it changes.
2. Approval card: bigger headline; sentence-case meta line; the request in the monospace face (it is a real command); long requests clamp to two lines; the pulse becomes an outer ring whose opacity animates (composited, no repaint), and the permanent glow goes.
3. Task cards: sentence-case meta line; tabular figures; one-line titles with an ellipsis; `dir="auto"` on titles and projects so Hebrew reads right to left; more padding and larger type for distance.
4. Icons: draw the check and the warning sign as small SVG strokes in the text color.
5. Rings: the caption becomes two lines (name, then the time detail) that never wrap; the percent is semibold and tabular.
6. Page: a strong ease-out curve as a token; the entrance uses ease-out; antialiased text on the dark ground; secondary text at a fixed 0.68 to 0.72 white instead of opacity on the whole line.

## What changed

One line per change. Every animation still has a `prefers-reduced-motion` path, and every animated property is a transform or an opacity.

| Change | Why |
| --- | --- |
| Clock: no font-size transition | The runtime's FLIP move (transform only) now plays alone, with no layout per frame. |
| Clock: the date is out of flow under the time and fades out in the corner | The box keeps one shape, so the move scales evenly and the digits do not squash. |
| Clock: dark halo instead of the green glow, near-white tinted by the accent | Crisper edges over the rain. |
| Clock: -0.02em tracking on the time, date 3vmin | Tighter display type; the date reads from across the room. |
| Clock: writes its text only when it changes | No repaint every second for nothing. |
| Approval card: pulse is an outer ring whose opacity breathes (1.6 s, sine ease-in-out) | Same rhythm, but composited: no box-shadow repaint of a 55vw card per frame. Reduced motion shows the ring still. |
| Approval card: no resting glow; warm surface mixed from `--attention` and `--card-opacity` | One border is enough; the theme can still recolor it. |
| Approval card: headline 6vmin, meta 2.1vmin sentence case, project 3vmin | The most important words are now clearly the biggest. |
| Approval card: request in the system monospace, clamped to two lines | It is a real command; a long one no longer grows the card. |
| Approval card and finished cards: drawn SVG icons instead of "⚠" and "✓" | Text glyphs look different in every font and can turn into emoji. |
| Task cards: sentence-case meta line, tabular figures | Easier to read from a distance; "running 24m" no longer jitters as it counts. |
| Task cards: one line per field with an ellipsis, `dir="auto"` | Long titles never grow the card; Hebrew reads right to left with the ellipsis on its own end. |
| Task cards: more padding, title 2.6vmin, meta 1.8vmin | Distance legibility and a clear three-step rhythm. |
| Rings: caption on two lines (name, then detail), never wrapped | "Claude 5h" over "1h40m" instead of "CLAUDE 5H ·" over "1H40M". An empty detail keeps its line so rings line up. |
| Rings: percent semibold and tabular, ring 9vmin | The number people look for is the strongest thing in the ring. |
| Page: an `--ease-out` token (`cubic-bezier(0.23, 1, 0.32, 1)`); entrance 0.5 s ease-out; reduced motion keeps the fade only | A stronger curve than the default `ease`; reduced motion drops travel, not comprehension. |
| Page: antialiased text, `SF Pro Text` in the stack, halo on the service line | Light text on black looks lighter and sharper. |

### Checks and exceptions

- `impeccable detect`: no findings.
- `design-taste-frontend` lint (`--app`): clean. Four lines carry `taste-skill-ignore` with a reason: the ring's conic fill (it is the chart), the pulse ring (crisp spread, opacity only), and the static dark halos on the clock and captions (the rain is a moving picture; the halo is the scrim).
- `design-taste-frontend` inspector (`--app`, on the dev preview): it asks for an h1, no text halos, and nothing under 11px at 390px. A screen saver has no title, the halo is the scrim over moving rain, and the screen never runs at phone width (1024x768 is the smallest size). It also reads the task card border as opaque; it is the accent at 25%, about 1.5:1 on black.
- Not changed: the runtime's move curve (`ease-in-out`, 400 ms in `web/src/runtime.ts`) is out of scope for this task. The skills would use a stronger in-out curve there, such as `cubic-bezier(0.77, 0, 0.175, 1)`; a follow-up can change it.

### Screenshots

Taken with the saver harness (a scratchpad copy that takes the window size from `AGS_W`/`AGS_H`), from `web/dev/state-idle.json` and `state-attention.json` with live times. Not committed.

- Before: `<scratchpad>/shots/before-idle-1512x982.png`, `<scratchpad>/shots/before-idle-1024x768.png`, `<scratchpad>/shots/before-attention-1512x982.png`, `<scratchpad>/shots/before-attention-1024x768.png`
- After: `<scratchpad>/shots/after-idle-1512x982.png`, `<scratchpad>/shots/after-idle-1024x768.png`, `<scratchpad>/shots/after-attention-1512x982.png`, `<scratchpad>/shots/after-attention-1024x768.png`
- Worst case (Hebrew title, long title, long request, +1 more, four rings with stale data): `<scratchpad>/shots/after-state-stress-1512x982.png`, `<scratchpad>/shots/after-state-stress-1024x768.png`
