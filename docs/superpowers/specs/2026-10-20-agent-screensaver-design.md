# Agent Screen Saver: design

Date: 20/10/2026. Status: draft for owner review. Platform for version 1: macOS.

Working name: `agent-screensaver` (my call, easy to change).

## 1. Why this exists

You start a long task in Claude Code or Codex. You walk away from the Mac. When you come back, you want to know at a glance, from across the room:

- Does an agent need you? (It is waiting for your approval.)
- Which tasks finished?
- Which tasks are still running, and for how long?
- How much of your usage limits is left, and when do they reset?

Today you must unlock the Mac and open each terminal to find out. The screen saver shows the time anyway when you are away. So it is the right place to show this too.

The owner is the first user. The project will be open source.

## 2. Goals, non-goals, anti-goals

### Goals (version 1)

1. A real macOS screen saver. It works with the system lock. When the owner comes back, macOS asks for the password, as it does today.
2. It shows agent status for Claude Code and Codex: waiting for approval, running (with time), finished.
3. It shows usage limits: Claude Max session (5 hours) and weekly, Codex session (5 hours) and weekly, in percent, with the time until reset.
4. When an agent waits for approval, the screen makes it obvious from far away, and a small sound plays.
5. It is fast and light. This is the top priority.
6. The background can change: Matrix rain (default), a photo or video of the user's choice, or Apple aerial videos that macOS already downloaded.

### Non-goals (not in version 1)

- Linux and Windows. The design must not block them later, but no work goes there now.
- Cursor. Its usage data is only available through cookies or internal endpoints.
- Jira, GitHub, calendar, Slack, social networks (Reddit, Bluesky, YouTube, Gmail). The card model (section 6) must make them easy to add later.
- A drag-and-drop layout editor.
- Running another company's screen saver inside ours.

### Anti-goals (failure even if it "works")

- The Mac does not lock while the screen saver shows.
- The screen saver keeps using CPU or battery after it stops.
- Any data leaves the Mac. No analytics, no server of ours.
- The code uses undocumented endpoints (for example `api.anthropic.com/api/oauth/usage` or `chatgpt.com/backend-api/wham/usage`), browser cookies, or copies login tokens.
- The project downloads, stores, or ships Apple video files.
- The screen shows full prompts, code, or secrets. Version 1 shows only tool name, project folder name, a short title (see "Card text" in section 5), and times.

## 3. What the screen looks like

The owner chose layout "B, Spotlight, with a clock". Mockup: `docs/superpowers/specs/mockups/layout-b-clock.html` (copy of the brainstorm mockup).

### State 1: nothing needs you

- Center: a big clock (24-hour, `HH:MM`) and the date under it.
- Top right: four rings: Claude 5h, Claude week, Codex 5h, Codex week. Each ring shows the percent used. The 5h rings also show the time until reset (for example `1h40m`).
- Bottom: a row of cards, one per running or recently finished task. A card shows: tool, project folder name, short task title, and either "running 24m" or "finished 8m ago".

### State 2: an agent needs you

- The clock moves to the top-left corner and gets smaller. The move is a smooth animation.
- Center: a large orange card that pulses. It shows: tool, how long it has waited, "Needs approval", project folder name and a short title of the request.
- A small sound plays once when the card appears.
- If more than one agent waits, the center shows the oldest one, and the card shows "+N more".
- The rings and the bottom row stay.

### Colors

- Orange, pulsing: needs approval.
- Normal (green-tinted dark card): running.
- Blue-tinted: finished.
- Ring color: green under 50%, yellow 50 to 80%, red above 80% (my call).

## 4. Where the data comes from (version 1)

All sources are local and are official parts of the tools. No network calls. No tokens to store.

### Agent status: hooks

Both tools can run a command on events. Each hook runs a small command from our project (for example `agent-screensaver hook --tool claude --event stop`). The command reads the JSON the tool gives it, and sends one small event to our background service.

| Meaning | Claude Code hook | Codex hook |
|---|---|---|
| Task started | `UserPromptSubmit` | `UserPromptSubmit` |
| Waiting for approval | `Notification` | `PermissionRequest` |
| Tool ran (approval answered) | `PostToolUse` | `PostToolUse` |
| Finished | `Stop` | `Stop` |
| Session closed | `SessionEnd` | `SessionEnd` |

Facts we checked:

- Codex `notify` only reports `agent-turn-complete`, so it is not enough. Codex hooks (`~/.codex/hooks.json`) have `PermissionRequest`. Checked on codex-cli 0.144.4.
- `codex app-server` cannot see approvals of a different Codex process. The session files in `~/.codex/sessions` have no approval events.
- Codex has no "approval answered" event. So "waiting" clears on the next `PostToolUse`, `Stop` or `UserPromptSubmit`. If the user denies a command, no event may come. Rule: after 10 minutes without a new event, the card changes from "waiting" to "unknown, check the terminal" (my call on the number).
- The owner already has entries in `~/.codex/hooks.json` (Superset `notify.sh`). Installing our hooks must ADD entries and never change or remove existing ones. The same rule applies to Claude Code settings.
- Codex asks the user to trust new hooks once, through `/hooks`.

### Claude Max limits: status line

Claude Code sends JSON to the status line command each time it updates. For Pro and Max plans it includes:

```
rate_limits.five_hour.used_percentage   rate_limits.five_hour.resets_at
rate_limits.seven_day.used_percentage   rate_limits.seven_day.resets_at
```

`resets_at` is Unix time in seconds. Each part may be missing. The data only appears after the first answer in a session.

Our status line command writes these numbers to the service, then prints the user's normal status line. If the user already has a status line command, ours must call it and pass its output through, so nothing changes for them.

When no session runs, the screen shows the last numbers with "updated X min ago". The reset countdown keeps running from `resets_at`. After the reset time passes, the ring shows "reset, waiting for new data".

Source: https://code.claude.com/docs/en/statusline

### Codex limits: `codex app-server`

The service starts `codex app-server` and calls `account/rateLimits/read` over JSON-RPC on standard input and output. The result has a primary window (5 hours) and a secondary window (weekly), with reset times.

It asks every 5 minutes, and only while the screen saver is showing or the service thinks it will show soon (my call). The process stops after each call.

This method is the same one CodexBar uses (https://github.com/steipete/CodexBar, `docs/codex.md`).

## 5. Architecture

```
Claude Code / Codex hooks and status line
        │  small events (JSON)
        ▼
Go background service (always running, light)
  - keeps the current state in memory
  - asks codex app-server for Codex limits
  - plays the sound when a new approval appears
  - writes the state file
        │  state file (JSON)
        ▼
macOS screen saver (.saver bundle, thin Swift layer)
  - WKWebView fills the screen
  - HTML/CSS/JS draws the background and the cards
  - reads the state file
```

Decided:

- The service and the CLI are written in Go (owner decision, 20/10/2026: the owner reads and maintains Go more easily, and the service does very little work, so Go is light enough).
- The screen saver is a real `.saver` bundle in Swift. Apple gives no other way.
- Inside the saver, a `WKWebView` draws everything with HTML, CSS and canvas. The spike (section 8) showed this is lighter than drawing natively with Core Graphics.
- The Swift layer stays thin: it loads the page and passes the state in. All screen logic is in the web page.
- The service plays the sound, not the saver.
- When the saver gets `com.apple.screensaver.willstop`, it stops the web view and then ends its own process. Reason: macOS leaves `legacyScreenSaver` processes alive after stop (we saw this, see section 8).
- Frame rate (owner decision, 20/10/2026): no fixed project-wide cap. Each background declares its own frame rate as an option; the built-in Matrix defaults to 24 fps because it looks the same and costs less. The user setting `max_fps` (default: the display's refresh rate, so 120 on a 120 Hz display; allowed 1 to 240) is the one ceiling the runtime enforces for every background, so a user can lower it to save battery. Videos play at their own frame rate.

Languages:

- **Go:** the background service, the CLI (`install`, `uninstall`, `hook`, `statusline`, `theme`, `ext`), and the event and state formats.
- **Swift:** only the thin `.saver` layer (a web view, the state handoff, the stop fix).
- **TypeScript:** the web runtime inside the saver (layout engine, extension loader, the built-in backgrounds and components). It is compiled to plain JavaScript, so outside authors can write extensions in plain JavaScript and get a published type file for the API. One build tool (esbuild), no web framework.
- **JSON:** manifests, layouts, themes, events, state, config.

### Details settled after review (my calls)

- **First task of the build:** prove that the saver, running inside `legacyScreenSaver`, can read the state from the service. The spike only proved drawing. `legacyScreenSaver` is sandboxed, so a file in `~/Library/...` or a Unix socket may be blocked. If it is blocked, try a location the saver can read (for example `/Users/Shared/...` with user-only permissions, or the saver's own container). Nothing else gets built until this works.
- **Starting the service:** a launchd LaunchAgent starts it at login and restarts it if it stops.
- **Saver without the service:** it shows the background and the clock, plus a small line "service not running".
- **Knowing when the saver shows:** the service listens for the system notifications `com.apple.screensaver.didstart` and `com.apple.screensaver.didstop`. No second channel from the saver is needed.
- **Sound:** plays only while the screen saver shows. If the saver is not showing, no sound (the owner is at the Mac).
- **Codex limits polling:** every 5 minutes while the saver shows, plus once after each Codex `Stop` event.
- **Card key:** tool plus the session id from the hook input. Two sessions in the same project are two cards.
- **Stale cards:** a "running" card with no event for 60 minutes changes to "no activity". Any card with no event for 12 hours disappears.
- **Status line:** the source of truth is `statusLine` in `~/.claude/settings.json`. On install we save the user's existing command and put ours in its place. Ours runs theirs with a 300 ms timeout and prints its output. If it fails or is slow, ours prints nothing extra and never blocks Claude. If the saved command is already ours, it does not call itself.
- **Install and uninstall:** one command, `agent-screensaver install`, adds the hooks, the status line wrapper, the LaunchAgent, and the `.saver`. It always shows what it will change first. `agent-screensaver uninstall` removes exactly what install added and restores the old status line.
- **Card text (owner decision, 20/10/2026):** the owner's own Mac, so useful text wins over hiding it.
  - Running and finished cards: the first 40 characters of the prompt, from the `prompt` field of `UserPromptSubmit`, cut at a word end, with "…".
  - Approval card: the tool name and the start of what it wants to run (for example `Bash: npm run migrate`), 40 characters max. Codex `PermissionRequest` gives `tool_name` and `tool_input`. Claude `Notification` gives only a message, so take the tool input from the last `PreToolUse` of the same session. (To verify: if Claude Code has a `PermissionRequest` hook with the tool input, use it instead.)
  - Before showing any text, hide anything that looks like a secret: values after `token`, `key`, `secret`, `password`, `Bearer`, and long random-looking strings, replaced by `•••` (my call).
  - Privacy mode is off by default. When on, cards show only tool, project and times.
- **Displays:** the main display shows the full layout. Other displays show the background and the clock.
- **Supported Macs:** macOS 14 or later, Apple Silicon and Intel (universal build).
- **Signing (owner decision, 20/10/2026):** no Apple Developer account for now. The `.saver` is signed ad hoc only. Users who download a release must use "Open Anyway" in System Settings → Privacy & Security once. The README must explain this step clearly. Users who build from source should not see the warning, because macOS only checks files marked as downloaded (to verify during the build). Paid signing and notarization can be added later without design changes.

## 6. The card model

Every source turns its data into the same small structure. The screen only knows this structure. Adding a source later means one new module and no change to the screen code.

```
Card {
  source      // "claude" | "codex" | later "jira", "github", ...
  kind        // "approval" | "running" | "finished" | "unknown"
  project     // folder name, not the full path
  title       // short, safe text
  started_at
  updated_at
  link        // optional, not used on screen in version 1
}

Limits {
  source, window ("5h" | "week"), used_percent, resets_at, updated_at
}
```

Finished cards stay for 30 minutes, then disappear (my call).

## 7. Backgrounds and customization

Backgrounds in version 1:

1. Matrix rain (default). Options: color, speed, characters.
2. The user's own image or video file.
3. Apple aerials: Landscapes, Cities, Space, Underwater. The list comes from `/Library/Application Support/com.apple.idleassetsd/Customer/entries.json` (137 entries on the owner's Mac, 20/10/2026). We play ONLY video files macOS already downloaded. If none are downloaded for the chosen group, settings show: "Pick it once in System Settings → Screen Saver, and macOS will download the videos." This folder is not an official API. If Apple moves it, this background must fail quietly and fall back to Matrix.

Customization in version 1: background choice and its options, card size, how see-through the cards are, sound on or off, privacy mode on or off (privacy mode hides titles and project names and shows only counts).

## 7a. Extensibility: the core idea of the project (owner, 20/10/2026)

The project is a platform, not one fixed screen. People must be able to set their own background, change the layout, add components, install themes, and later install plugins and download themes. Version 1 does not build a store or a download service. But version 1 MUST be built so that these come later without a rewrite.

The rule that makes this real: **everything built in is an extension too.** The Matrix background, the clock, the limit rings, the task cards and layout B are written exactly the way an outside author would write them. If the built-in parts need a private shortcut, the extension system is not good enough yet.

### Extension types

| Type | What it is | Where it runs |
|---|---|---|
| Background | Draws the full-screen background (Matrix, video, aerials, image) | Web page in the saver |
| Component | One piece on the screen that draws from the state (clock, rings, card row, approval card) | Web page in the saver |
| Layout | A declarative file that places components in regions, for each screen state (`idle`, `attention`) | Web page in the saver |
| Theme | A package: a background, a layout and style values (colors, fonts, sizes) with chosen options | Web page in the saver |
| Source | Sends cards and limits into the service (Claude and Codex in version 1, Jira or GitHub later) | Outside the saver |

### Contracts

- Every extension has a manifest file (`extension.json`): id, name, version, type, entry file, the API version it needs, and an options schema. The settings come from the options schema, so a new extension gets settings without new settings code.
- Web extensions (background, component) get the state and their options through one small, versioned JavaScript API. They get nothing else.
- A layout is data only (JSON), no code.
- A source is any program that sends events to the service through the same local channel the hooks use, in the documented event format. This is how Claude and Codex work in version 1. So a new source can be written in any language, and the Go service never loads outside code.

### Safety rules for outside extensions

The screen saver runs while the Mac is locked, so outside code must be limited:

- Web extensions run with no network access (Content Security Policy blocks it) and no file access. They see only the state the service gives them.
- If an extension throws an error or freezes, the saver drops it and shows the default for that slot.
- The runtime enforces the user's `max_fps` ceiling for every background that draws through the API, including outside ones.

### Version 1 scope for extensions

- In: the manifest format, the web extension API, layouts as JSON, themes as folders, installing an extension from a local folder or `.zip` with the CLI, listing and removing installed extensions, a documented event format for sources.
- Out (later): a theme or plugin store, downloading from the internet, signing extensions, a visual editor.

### Settings

Settings change through the menu bar app's settings window (§7c), or through CLI commands (for example `agent-screensaver theme set builtin.default`). Both use the same Go settings package and the same option schemas.

## 7b. Code principles (owner, 20/10/2026)

The code must be simple, readable, efficient, extendable, idiomatic, modular, testable and DRY. Each word has a concrete meaning here, so a reviewer can check it:

| Principle | What it means in this project |
|---|---|
| Simple | The smallest solution that meets the spec. Abstractions are welcome from the start where they make the project extendable (owner, 20/10/2026), and each one stays small: one interface with a few methods, a plain registry, no frameworks, no generic "plugin engine". An abstraction that no extension point needs is not added. No speculative options or config. |
| Readable | Names say what things are, in full words. Short functions with one job. Comments explain why, not what. A new contributor can follow the data from a hook to the screen in one sitting. |
| Efficient | Respect the budgets: about 6.5% of one core while the Matrix shows, near 0% CPU when idle, hooks finish well under 100 ms, the Matrix at its default 24 fps. Measure before optimizing; no optimization that makes code harder to read without a measured need. |
| Extendable | New backgrounds, components, layouts, themes and sources are added as extensions (spec §7a), never by editing core code. Built-in parts use only the public extension API. |
| Idiomatic | Go: standard library first, errors returned and wrapped with `%w`, no panics in normal flow, small interfaces defined where they are used, `gofmt` and `go vet` clean. TypeScript: strict mode, no `any` in public types, plain modules, no framework. Swift: kept thin, standard AppKit and WebKit APIs only. |
| Modular | One package or module per responsibility (spec §6, plan file layout). Packages talk through small, documented types. No import cycles; the web runtime and built-ins never import service code. |
| Testable | Logic is pure where possible and takes time, paths and system calls as inputs (no hidden clock, no hidden `HOME`). Anything that touches the system sits behind a small interface with a fake for tests. Every behavior in this spec has a test. |
| DRY | One source of truth for each format and rule: the event format, the state file shape, the card rules and the extension manifest are each defined once and reused. Shared logic lives in one place; no copy-pasted parsing or validation between Claude and Codex paths. |

### Extension points in the Go code (built in version 1)

These sit exactly where the spec already names a second implementation that will come later:

| Interface | Version 1 implementations | Later |
|---|---|---|
| `agents.Adapter`: maps hook input to events, adds and removes its hooks, says where its config lives | Claude Code, Codex | Cursor, Gemini CLI, others |
| `limits.Provider`: reads usage limits on a schedule | Codex (`codex app-server`) | other tools with a local limits source |
| `platform.ServiceManager`: starts and stops a background process at login (the daemon and the tray) | launchd (macOS) | systemd (Linux), Task Scheduler (Windows) |
| `platform.SaverWatcher`: tells when the screen saver starts and stops | macOS distributed notifications | Linux, Windows |
| `platform.SoundPlayer`: plays the attention sound | `afplay` (macOS) | Linux, Windows |
| `platform.FilePicker`: native open-file dialog for installing extensions | `osascript` `choose file` (macOS) | Linux, Windows |

Each adapter or provider registers itself in one list. Adding a new agent tool means one new file that implements `agents.Adapter`, plus one line in that list. Claude's status line limits stay inside the Claude adapter, because they arrive through Claude's own status line.

When two principles pull against each other, the order is: correct, then simple and readable, then efficient. A small amount of repetition is fine when removing it would make the code harder to read.

## 7c. Menu bar app (owner decision, 20/10/2026)

Version 1 includes a menu bar app. It replaces "settings window later" from §7a. It is built so the same code runs on macOS, Windows (system tray) and Linux (AppIndicator tray) later; version 1 ships and tests it on macOS only.

### How it is built

- It is part of the same Go binary, using two cross-platform libraries: `fyne.io/systray` for the tray icon and menu, and `github.com/webview/webview_go` for the settings window (WebKit on macOS, WebView2 on Windows, WebKitGTK on Linux). These are the only third-party Go libraries in the project. The first task of this phase checks that both are maintained and work on macOS 26 (see the plan).
- Three separate processes, each small: the daemon (no UI), `agent-screensaver tray` (icon and menu, started at login like the daemon), and `agent-screensaver settings` (the settings window, started by the tray only when the user opens it, and closed after). Reason: both libraries want the main thread, and a crash in the UI must never stop the daemon.
- The settings page is HTML and TypeScript, the same on every OS. The window talks to Go through the webview's function binding, in the same process. No network port is opened.
- All settings logic lives in one Go package used by the CLI, the tray and the settings window. No logic is written twice.

### The menu

- A summary line: "1 waiting for approval · 2 running", or "All quiet".
- One line per limit: "Claude 62% · resets in 1h40m", "Codex week 55%".
- One item per agent that waits for approval (tool, project, how long).
- "Preview screen saver", "Settings…", "Sound on/off", "Privacy mode on/off", "Quit menu bar app".
- The icon changes when an agent waits for approval, so the owner sees it even when the screen saver is not showing.

### The settings window

- Theme gallery: every installed theme, with a live preview. The preview is the real screen saver runtime inside the page, fed with the current state.
- Background, layout and per-extension options. The form is built from each extension's option schema, so a new extension gets its settings with no new UI code.
- Install an extension from a folder or `.zip` (native file picker), list and remove installed extensions.
- General: sound, sound file, privacy mode, max frame rate.

## 8. Spike results (throwaway test, 20/10/2026)

Machine: Apple M4 Pro, macOS 26.6.2. Matrix rain, 24 fps, 1512x982 window.

| Way of drawing | CPU | Memory |
|---|---|---|
| WKWebView + canvas | about 6.5% of one core, all processes together | about 218 MB, mostly WebKit |
| Native Core Graphics | about 20% of one core | about 235 MB |

Real install test with the system screen saver:

- The web view processes went away after stop. Our stop code worked.
- Two `legacyScreenSaver` processes stayed alive after stop, at 0% CPU, about 60 MB together. This is a known macOS behavior. Section 5 adds the fix.

Not measured yet: a video background.

## 9. Risks

1. `legacyScreenSaver` bugs on future macOS versions.
2. Hooks or status line fields change in Claude Code or Codex updates.
3. The Apple aerials folder moves.
4. Codex approval state can get stuck after a denied command (handled by the 10-minute rule).
5. A WKWebView in the saver uses about 200 MB while it shows.
6. The two tray and webview libraries are third-party projects; if one stops being maintained, the tray or settings window needs another library. They sit behind the `tray` and `settings` packages, so only those packages change.

## 10. Left to the builder

- Name of the binary and of the bundle.
- How hook events reach the service (Unix socket, local file, or similar), as long as it is local only and fast.
- How the saver learns about state changes (reading a file on change, or a local socket).
- Project layout, build scripts, tests, CI.
- Exact sound file (must be free to use and small).
- The open-source licence (MIT or Apache-2.0 suggested).
