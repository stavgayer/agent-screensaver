# agent-screensaver

A macOS screen saver that shows what your AI agents are doing. You start a long task in Claude Code, Codex or Cursor and walk away. When you come back, you see at a glance from across the room if an agent needs you, which tasks finished, which are still running and for how long, and how much of your usage limits is left. The screen saver shows the time anyway while you are away, so it is the right place to show this too.

Supported agents: Claude Code, Codex and Cursor are built in. Usage limits are shown for Claude Code and Codex only: Cursor keeps its usage on its own server. Cursor cards show working (with the current command) and done, but never "waiting for approval", because Cursor does not tell us when it will really ask you. You can add other agents yourself, see [docs/agents.md](docs/agents.md).

## Requirements

- macOS 14 or later.
- Apple Silicon or Intel.

## Install

1. Download the zip and unzip it.
2. Open Terminal in the unzipped folder and run `./agent-screensaver install`.
3. Read the plan it prints. It lists every change it will make.
4. Type `y` to confirm.

### Open Anyway (one time)

The screen saver is not signed with an Apple Developer account. macOS blocks it the first time. To allow it:

1. Open System Settings, then Privacy & Security.
2. Find the message about AgentScreensaver and click "Open Anyway".

You do this once.

### Choose the screen saver

Open System Settings, then Screen Saver, and pick AgentScreensaver.

### Approve the Codex hooks

In Codex, run `/hooks` and approve the new agent-screensaver hooks. Codex asks you to trust new hooks once.

### Lock the screen right away

Open System Settings, then Lock Screen. Set "Require password after screen saver begins" to Immediately.

## Settings

Change settings with `agent-screensaver` commands, run from the unzipped folder. One example each:

| Command | Example |
| --- | --- |
| `theme` | `./agent-screensaver theme set builtin.default` |
| `background` | `./agent-screensaver background set builtin.aerials` |
| `option` | `./agent-screensaver option set builtin.matrix color "#00ff66"` |
| `privacy` | `./agent-screensaver privacy on` |
| `sound` | `./agent-screensaver sound on` |
| `fps` | `./agent-screensaver fps max 30` |
| `ext` | `./agent-screensaver ext install ./my-extension` |
| `demo` | `./agent-screensaver demo --seconds 60` |

Run `./agent-screensaver theme list` to see the themes you can use.

`demo` plays a made-up scene with three agents, for a screen recording. It starts when the screen saver next shows and plays for the seconds you give (10 to 600, 60 if you give none). Start the screen saver within 5 minutes, or the demo is dropped. It ends when the screen saver stops, and a real approval request ends it at once. `./agent-screensaver demo stop` ends it early. Privacy mode does not hide the demo text, because none of it is real.

To hide the sessions of a folder, add its name under General, Ignored projects, in the settings window. They then get no card, no approval and no sound. The list starts with `observer-sessions`, the folder where the claude-mem plugin runs its hidden Claude sessions.

The menu bar app has a settings window too. On a Mac with a notch, a full menu bar can hide the app's icon behind the notch. Then run `./agent-screensaver settings` to open the settings window directly.

## Uninstall

Run `./agent-screensaver uninstall` and confirm the plan. Your settings and extensions in the data folder are kept.

## Build from source

You need Go, Node, and the Xcode command line tools. Run:

```
scripts/build-release.sh
```

It builds a universal binary and the screen saver, and writes `dist/agent-screensaver-<version>-macos.zip`.

## More

- [How-to guides](docs/guides/README.md): add an agent, a widget or a background, change the layout, create a theme, test and share. Each has a tested sample.
- [Writing extensions](docs/extensions.md)
- [Agent extensions](docs/agents.md): teach agent-screensaver a new coding agent. The project ships no company logos; you may add your own icon for any agent.
- [Event format](docs/event-format.md)
- [Known limits](docs/known-limits.md)
