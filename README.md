# agent-screensaver

A macOS screen saver that shows what your AI agents are doing. You start a long task in Claude Code or Codex and walk away. When you come back, you see at a glance from across the room if an agent needs you, which tasks finished, which are still running and for how long, and how much of your usage limits is left. The screen saver shows the time anyway while you are away, so it is the right place to show this too.

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
| `option` | `./agent-screensaver option set builtin.matrix color "#00ff66"` |
| `privacy` | `./agent-screensaver privacy on` |
| `sound` | `./agent-screensaver sound on` |
| `fps` | `./agent-screensaver fps max 30` |
| `ext` | `./agent-screensaver ext install ./my-extension` |

Run `./agent-screensaver theme list` to see the themes you can use.

## Uninstall

Run `./agent-screensaver uninstall` and confirm the plan. Your settings and extensions in the data folder are kept.

## Build from source

You need Go, Node, and the Xcode command line tools. Run:

```
scripts/build-release.sh
```

It builds a universal binary and the screen saver, and writes `dist/agent-screensaver-<version>-macos.zip`.

## More

- [Writing extensions](docs/extensions.md)
- [Event format](docs/event-format.md)
