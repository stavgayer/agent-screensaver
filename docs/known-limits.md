# Known limits

These are the limits of version 1 that you can see or that change how you use it. Each one is known and was left as it is on purpose, or for a later version.

## Agents and hooks

- **An edited settings file can change its layout.** When agent-screensaver adds or removes its hooks in a file such as `~/.claude/settings.json`, it keeps the keys, their order, every value it did not touch, the indent per level and the final newline. It writes every key and every list item on its own line, so a list or object written on one line is spread over several lines. Windows line ends (CRLF) come back as plain line ends (LF), and the spaces around `:` and `,` become the standard ones. So uninstall gives a file back byte for byte only when it already had that layout.
- **`codex exec` runs show no card.** Codex sends no transcript for them, and the built-in Codex agent ignores events without a transcript.
- **Cursor has no usage ring and no approval cards.** Cursor keeps its usage on its server only, and it does not tell a hook whether it will ask you. See [agents.md](agents.md), "Known limits".

## Screen and menu bar

- **The menu bar icon can hide behind the notch.** Open Spotlight or Launchpad and search for "Agent Screensaver" instead. See the README, "Settings window and menu bar".

## Themes

- **The Chrome theme needs WebGL2.** Without it, the background is only black with grain and scan lines: no chrome shape and no agent signs. The clock, the rings and the cards still show.
- **The Chrome theme uses more CPU than the others.** On the owner's Mac it used about 14% of one core, measured once while the machine was under heavy load ([how it was measured](decisions/0002-video-cost.md#addendum-chrome-theme-2026-10-26)). Matrix uses about 5.9%, measured in a different session ([decision 0002](decisions/0002-video-cost.md)), so compare the two only roughly. To keep it low, it draws 30 frames a second at half the screen size by default.
- **The live preview in the settings window is less smooth than the screen saver.** It draws 15 frames a second, at one pixel per point, so the settings window stays light. It pauses, and keeps its last picture, 10 seconds after the window goes to the background, and while it is scrolled out of sight. It runs in a sandboxed frame from another origin so that extension code cannot reach the settings page. The real screen saver is not affected. This shows most with the animated themes (Chrome, Matrix).
