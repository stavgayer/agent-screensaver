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

- Before: not committed.
- After: not committed.
- Worst case (Hebrew title, long title, long request, +1 more, four rings with stale data): not committed.

## The Calm theme

`builtin.calm` is the quiet theme: the Apple aerials background, the same Spotlight layout, softer colors. Aerials are bright and they move, so the cards stay dark and the colors do the calming. It shows Matrix until aerial videos are downloaded, the same as any aerials theme.

| Value | Calm | Default | Why |
| --- | --- | --- | --- |
| `--accent` | `#6cc5b9` | `#00e07a` | A muted sea teal. It sits on the black disc of a ring, where it measures 10:1, and it tints the clock only 12%, so the clock stays near white. |
| `--attention` | `#ff9a4d` | `#ffb020` | A warm orange, a little redder than the default. Warm against a mostly blue or grey aerial is the strongest pull from across the room. The approval card is a dark brown box with a 2 px orange edge and the pulse ring, so it does not depend on the video for contrast. |
| `--done` | `#a9c4f5` | `#78c8ff` | A pale blue, 10.6:1 on the task card. It is clearly not the teal accent. |
| `--card-opacity` | `0.86` | `0.82` | The cards sit directly on video. At 0.86 over a pure white frame, the weakest text (the 68% white meta line) is 6.8:1 and the title 12.7:1. Over a dark frame they are 9:1 and 18.9:1. |

Numbers are WCAG contrast ratios computed for the card colors over a white frame, a bright sky (200, 225, 250) and a dark frame. The weakest case is white. Checked by eye on two real aerials, one hazy city at dusk and one dark night street: the approval card is the first thing the eye finds in both.

The settings card shows a dusk-sky tile for any theme whose background is `builtin.aerials`, with the clock underlined in the theme accent; the subtitle is the background's name, "Apple aerials".

## Understandable task cards (22 October)

The owner looked at real cards from across the room and could not tell what each card was about or whether anything still ran. The state was a small grey line ("Claude · running 24m") above the title, and nothing said that a done task still had jobs in the background.

### What each card answers now

Three lines, the same height in every state, so nothing shifts when a card changes:

1. **State**, in words and in its color, with a drawn mark: "Working · 24m" (accent, a dot that breathes), "Done · 3m ago" (`--done`, a check), "No activity · 1h" (white, an empty ring), "Check the terminal" (`--attention`, a dot). Under one minute, done reads "Done · just now".
2. **What**: the title, the start of the prompt. In privacy mode the project takes its place, and with neither, the agent.
3. **Where**: the project, the agent, and "N in background" in the accent when background jobs still run.

### What changed

| Change | Why |
| --- | --- |
| The state line comes first, in sentence case, 2.2vmin semibold in the state's color | The color and the first word answer "is it working, done or stuck?" from 3 m. The grey 1.8vmin meta line did not. |
| Plain words: Working, Done, No activity, Check the terminal | Everyday words instead of "running" and "finished"; each fits one line on the narrowest card. |
| Drawn marks: breathing dot, check, empty ring, solid dot | A second cue next to the color. The breathing dot (opacity only, 2.4 s, sine ease, alternate) says "alive"; reduced motion keeps it still. Its delay follows the clock, so the redraw every minute does not restart it. |
| The check moved from the title to the state line | The title is now only the task; the state lives in one place. |
| The card border takes the state's color (30% mix) | The state reads even when the text is too small to read. |
| The last line is "project · agent", then "N in background" in the accent | Where the task runs, and whether anything still runs after it is done. |
| On a narrow card the project shortens first, down to a few letters; with a count, a card under 33vmin wide leaves the agent out | The count is the answer to "is anything still running", so it is never cut. Checked with four cards at 1024x768. |
| Each part of the last line is isolated (`dir="auto"`) | A Hebrew project name keeps its own direction next to English words. |

The approval card in the center is unchanged.

### Checks

- `impeccable detect` on the task row: no findings. `design-taste-frontend` lint (`--app`) on the task row: 0 FAIL, 0 WARN.
- Motion: one animated property (opacity of the dot). No layout change: every line is one line with an ellipsis.

### Screenshots

Saver harness, live times, Default (Matrix) and Calm (a real aerial). Not committed.

- Before: not committed.
- After: the same names with `after-` in place of `before-`.
- Worst case (Hebrew title and project, long title, long project, no activity, check the terminal, 12 in background, four cards): not committed.
- Settings, General, Ignored projects (scratch copy of the page with a stub bridge): not committed.

## Richer task cards (23 October)

The owner asked each card to say which agent it is, the conversation's name, whether anything runs in the background, whether the agent works or is done, and how full the context is. Each card must still read at a glance from about 3 m.

### What each card shows now

Four lines, top to bottom:

1. **State and agent.** The 6.2 state line ("Working · 24m", "Done · 3m ago", ...) on the left. On the right, the agent's mark and its name in muted white (24 October: the mark now leads the state line, see below). The mark comes from the agent extension: a letter tile of our own ("C" for Claude Code, "X" for Codex), never a product logo. A user can put their own icon in `marks/<source>.svg` in the data folder (docs/agents.md). A source without an agent extension shows a neutral dot and its name.
2. **What.** The conversation name when the agent has one (Claude `/rename` or its own title, the Codex thread name), else the prompt title, else the project.
3. **Context.** "Context", a thin bar whose fill is the true share of the window, and the percent. The fill is muted white; from 80% the fill and the percent take the attention color, because Claude Code compacts the conversation near the end of the window. No bar when the agent does not say.
4. **Where.** Project · model · branch, then "N in background" in the accent.

The last line sits at the bottom of the card, so a card without a context bar still lines up with its neighbours.

### Hierarchy and narrow cards

The state and the name stay the two big lines (2.2vmin and 2.6vmin). Everything added is small (1.7 to 1.9vmin) and quiet: color is only on the state, the high context bar and the background count. When the last line is too long, the branch gives way first, then the project shortens to a few letters, and the model last (24 October, see below). Narrow cards drop words in this order: under 40vmin the branch and the agent's name (its mark stays), and the project when the card has background jobs; under 33vmin the word "Context"; under 26vmin the state's own small mark. The agent's mark, the state, the model and the count are never dropped. Checked with four cards at 1024x768: every state line fits.

Not added: cost, Codex background terminals, a plan-mode chip (it would need one more field from every hook and one more mark on the top line).

### Limit rings never show old numbers as current

Claude limits come only from the Claude Code status line, so a reading can be many hours old. After a window's `resets_at` the ring is empty and says "Reset" until a newer reading comes. A reading older than 30 minutes (5h window) or 6 hours (week and plan) is dimmed to 45% and its caption adds the age ("20h ago"). Fresh readings look as before.

### Checks

- `impeccable detect` on the task row and the rings: no findings. `design-taste-frontend` lint (`--app`): 0 FAIL, 0 WARN.
- No new motion. Every line is one line with an ellipsis.

### Screenshots

Saver harness, live times, Default (Matrix) and Calm (a real aerial), 1512x982 and 1024x768. Not committed. "Before" is the 6.2 runtime fed the same state.

- Idle: not committed.
- Attention: not committed.
- Worst case (Hebrew name and project, long name, long project and branch, long model, 97% context, no context, 12 in background, four cards): not committed.
- Rings (Claude 5h reset with an old 20%, Claude week 20 h old, Codex plan fresh): not committed.

## Fixes from the real screen (24 October)

The owner photographed the live screen with four cards and read it from about 3 m.

| Change | Why |
| --- | --- |
| The agent's mark leads the state line, 1.5 times the state's font size, so its letter stands about as tall as the state's capitals | The mark was a small badge in each card's top-right corner and could not be read from the room. Next to the state it answers "which agent, and how is it doing" in one look. |
| The agent's name moves to the far end of the state line, in muted white, and only on cards wider than 40vmin | The mark already names the agent; its alt text keeps the name for screen readers. |
| On the narrowest cards (under 26vmin) the state's own small mark goes before anything else; the agent's mark never goes | The state's color still tells the state. |
| The last line keeps the model: the branch gives way first, then the project shortens to a few letters, and the model last. A card under 40vmin wide with background jobs leaves the project out | With four cards the line showed only the project. Now it reads "agent-screensaver · Opus 5.5"; with a count, "GPT-5.5 · 1 in background". |
| Ring reset times: a countdown within 24 hours, the weekday and time within 6 days, else the date ("2 Nov 04:27", or "2 Nov" when the caption also gives the age or "est.") | "Sun 02:00" for a reset 8 days away read as tomorrow. |

Rules checked by `web/test/task-row-style.test.ts`. `impeccable detect`: no findings. `design-taste-frontend` lint (`--app`): 0 FAIL, 0 WARN. No new motion.

Screenshots (saver harness, live times, four cards like the owner's photo; not committed):

- Before: not committed.
- After: the same names with `after-` in place of `before-`. Card rows side by side: not committed.
- Worst case and wide cards: not committed.
