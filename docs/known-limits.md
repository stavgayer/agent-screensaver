# Known limits

These are the limits of version 1 that you can see or that change how you use it. Each one is known and was left as it is on purpose, or for a later version.

## Agents and hooks

- **A converter can time out on its first run.** macOS checks a new program the first time it starts, which can take up to a second. The first event of an agent with a converter can be lost. Later events are not affected.
- **`codex exec` runs show no card.** Codex sends no transcript for them, and the built-in Codex agent ignores events without a transcript.
- **Cursor has no usage ring and no approval cards.** Cursor keeps its usage on its server only, and it does not tell a hook whether it will ask you. See [agents.md](agents.md), "Known limits".

## Cards

- **A long Claude session can lose its name after a restart.** The name is read from the last 256 KB of the session's transcript. If the service restarts during a long session, the card shows the prompt until the name is written near the end of the transcript again.

## Screen and menu bar

- **The menu bar icon can hide behind the notch.** See the README, "Settings window and menu bar", for how to open the settings window directly.

## Themes

- **The Chrome theme needs WebGL2.** Without it, the background is only black with grain and scan lines: no chrome shape and no agent signs. The clock, the rings and the cards still show.
- **The Chrome theme uses more CPU than the others.** On the owner's Mac it used about 14% of one core, measured once while the machine was under heavy load ([how it was measured](decisions/0002-video-cost.md#addendum-chrome-theme-2026-10-26)). Matrix uses about 5.9%, measured in a different session ([decision 0002](decisions/0002-video-cost.md)), so compare the two only roughly. To keep it low, it draws 30 frames a second at half the screen size by default.
- **The live preview in the settings window is less smooth than the screen saver.** It gets about 22 frames a second, in uneven steps. WebKit slows animation inside sandboxed frames from another origin, and the preview runs in one so that extension code cannot reach the settings page. The real screen saver is not affected. This shows most with the animated themes (Chrome, Matrix).
