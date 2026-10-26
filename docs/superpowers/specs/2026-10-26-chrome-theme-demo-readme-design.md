# Chrome theme, demo scene and public README

Date: 2026-10-26. Owner: Stav Gayer. Status: draft for owner review.

## Why

The project goes public at `github.com/stavgayer/agent-screensaver`, with downloads on that repo's GitHub Releases page. People share screenshots and short videos, not feature lists. So this work gives them:

1. A theme people stop scrolling for.
2. A demo scene, so a perfect video can be recorded at any time without real agents and without showing real prompts.
3. A README that explains what it does, how it works, how to install it and how to extend it, with a video at the top.

## Goals

- A new opt-in theme with a chrome 3D look inspired by the visual language of Fontaines D.C. (the "Romance" era): mostly black, acid lime as the loud color, a little hot pink, liquid chrome, film grain, scan lines, light distortion, huge condensed type used as image, tiny monospace technical labels.
- The whole scene reacts to the agents: each agent is part of the 3D scene, and the scene changes when an agent needs the user.
- A `demo` command that plays a scripted scene on the real screen saver.
- A new README.

## Non-goals

- Pixel-art theme. It comes later as its own theme.
- T3 Code support and any Cursor usage ring. Research is running (see "Follow-ups"); they get their own design.
- Changing the default theme. The new theme is opt-in.
- Signing, notarizing, Homebrew.

## Anti-goals (a failure even if it works)

- A theme that makes the Mac fan spin or drains the battery.
- Copying band artwork, logos, album titles or fonts we have no licence for. We take the feel, not the assets.
- Any network access, including fonts or the 3D library from a CDN.
- An approval card that is hard to read. Looks never beat "what does the agent want".
- README text that describes anything not built.

## Constraints (from the project, unchanged)

- Everything visible is an extension using the public API. The new theme uses only what any extension author can use.
- Fast and light. Simple, readable, modular, testable, DRY. Document only what exists.
- No network calls except the local codex app-server. No login tokens. Prompts masked.
- English, plain words, no em-dashes, in docs and UI copy.
- Design skills (emil-design-eng, impeccable, design-taste-frontend) for all UI work.

## Part 1: background API gets agent state

Owner decision (26/10): option B, the agents live inside the 3D scene, made possible by one small API change.

- `BackgroundExt` gets one optional method: `update?(state: ViewState): void`.
- The runtime calls it on start (after `start`) and every time the state changes, the same way components get `update`.
- Backgrounds without it keep working with no change. `api` stays `1` (the method is optional, so no old extension breaks).
- On a secondary display the background gets the state too; it decides what to show (the chrome theme shows no agents there, see Part 2).
- Docs: `docs/extensions.md` (the background section and the API types), `docs/guides/add-a-background.md` (a short "react to agents" step with a tested example under `docs/guides/examples/`).

## Part 2: the chrome theme

Working name "Chrome", id `builtin.chrome` (my call; the owner can rename). Pieces, each an extension:

- `builtin.chrome-scene` (background, WebGL):
  - A slow liquid-chrome shape that warps and catches light from an environment map made in code (no image assets needed).
  - Each agent with a card is a glowing chrome sign that carries its letter mark (the same drawn marks, no company logos). Running: slow pulse. Finished: one lime flash, then a calm glow. Approval: the sign turns hot pink with a short glitch burst, and the whole scene shifts toward pink while any approval is open. No cards: only the chrome shape.
  - Film grain, scan lines and a light chromatic distortion as one final pass.
  - Secondary display: the shape and grain only, no signs.
- Chrome-styled clock and limit rings as new components, `builtin.chrome-clock` and `builtin.chrome-rings` (my call: a theme's style values cannot load fonts, and the built-in clock and rings set their fonts in code). Each component ships its font files in its own folder and loads them with the standard `FontFace` API from its own entry file, so any extension author can do the same; no API change. A small shared helper in `web/src/builtins/shared/` keeps the font loading in one place. Huge condensed type for the clock, tiny monospace labels for tokens, context and countdowns. The approval card stays the built-in one with chrome style values.
- The approval card keeps its content and stays fully readable: solid dark panel, high contrast, pink accent. It may glitch in, but text never distorts.
- `builtin.chrome` (theme): picks the scene, a layout (existing `builtin.spotlight` unless it needs another) and the style values.

Fonts: bundled, open licence (SIL OFL or similar), a condensed display face and a monospace face. Licence files ship next to them. The 3D library (if one is used, for example three.js) is bundled into the extension at build time, with its licence.

Performance budget (my call, to be measured and reported):

- 30 fps cap, through `ctx.requestFrame(cb, 30)`.
- Internal render size at most half the screen size in each direction, scaled up under the grain.
- Paused when the saver is hidden (the host already hides the view; the scene must stop its loop and release nothing it needs again).
- Target: at most about 3 times the CPU of Matrix (Matrix is about 6.5% of one core), measured on the owner's Mac with the method in `docs/e2e-checklist.md`. If it costs more, lower the render size first. This is an owner checkpoint (my call): the owner runs the measurement and sees the theme before the README media are recorded.
- `prefers-reduced-motion`: the shape stops warping and glitches become simple color changes.

## Part 3: demo scene

Command: `agent-screensaver demo [--seconds N]` (default 60, my call).

- The daemon stays the only writer of `state.json`. The command tells the daemon to play the demo (for example through a file in DATA_DIR the daemon watches, like the other settings); the builder picks the existing mechanism that fits best.
- The command does not start the screen saver. It arms the demo and prints "Start the screen saver now". The N seconds start when the saver appears (the daemon already knows when the saver shows), so the recording always begins at 0 s and the approval sound plays. An armed demo that never sees the saver expires after 5 minutes (my call).
- While the demo plays, the daemon writes a scripted view state instead of the real one. Real events still arrive and are kept; they show again when the demo ends.
- Safety: a real approval request ends the demo at once, so a real agent never waits unseen (my call).
- Demo text is never masked by privacy mode: it is made up, and masking would spoil the video (my call). The demo state goes in after masking.
- The demo ends by itself after N seconds, or at once with `agent-screensaver demo stop`.
- Script (times from the start of each 30 s loop; the loop repeats until the end, my call):
  - 0 s: Claude Code card running ("refactor auth middleware", context 34%, branch `feat/auth`), Codex card running ("add rate limit tests"), Cursor card running.
  - 6 s: Codex card finishes.
  - 10 s: Claude Code asks for approval ("Bash: npm run migrate"). The approval sound plays, as in real use.
  - 18 s: approval answered, Claude card back to running.
  - 24 s: Claude finishes. 24 to 30 s: all three cards done, then the loop restarts.
  - Limits rise once over the whole demo, not per loop: Claude 5h ring from 42% to 61%, week 28%; Codex 5h 17%, week 9%; reset countdowns ticking.
- All text in the script is made up. No real names, prompts or paths.
- The menu bar icon follows the demo state too, so a screen recording of the whole Mac looks right.
- Tests: the script is a pure function from (elapsed time, total length) to view state, unit-tested; the daemon switch, the start when the saver appears, the end on a real approval and the automatic end are tested.

## Part 4: README

Replaces the current `README.md`. Order:

1. Name, one-line pitch ("A macOS screen saver that shows what your AI coding agents are doing."), badges: macOS 14+, MIT, works offline.
2. Hero video. GitHub plays a README video only when the .mp4 is dragged into GitHub's web editor. So: a short looping GIF committed in the repo (`docs/media/`), plus a clearly marked place where the owner drops the uploaded video link. The owner records both with the demo scene (Cmd+Shift+5).
3. What it does: 3 to 4 screenshots (approval card, done card, limit rings, Calm and Chrome themes), one line each. Screenshots taken by the owner from the demo scene; marked placeholders until then.
4. Install: download the zip that `scripts/build-release.sh` makes (`agent-screensaver-<version>-macos.zip`) from Releases; the owner publishes it there, no release automation in this work. Then run `./agent-screensaver install`, read the plan, confirm, "Open Anyway" once, pick it in System Settings. Uninstall, and the promise that hook files are restored.
5. How it works: a small diagram (agent hooks, local service, `state.json`, screen saver) and the privacy promise (no network except the local codex app-server, no tokens, prompts masked).
6. Supported agents: a table for Claude Code, Codex and Cursor with what each shows (cards, approval, limits).
7. Make it yours: links to every guide in `docs/guides/` and to `docs/extensions.md`, `docs/agents.md`, `docs/event-format.md`.
8. Settings window and menu bar, FAQ (from `docs/known-limits.md`), contributing, license.

Every claim is checked against the code before it goes in. Links are relative so they work on GitHub and in forks.

## Left to the builder

- Shader and scene details, exact colors and type sizes, within the look above (use the design skills).
- Whether the clock and rings need new components or only style values.
- How the demo command reaches the daemon.
- Diagram format in the README (a committed SVG or Mermaid).

## Follow-ups (not in this design)

- T3 Code support and what its source teaches about providers and Cursor usage. A research note is being written; the owner decides scope after reading it. Any Cursor usage ring must obey the project rules (local only, no tokens, no undocumented endpoints).
- Pixel-art theme.
