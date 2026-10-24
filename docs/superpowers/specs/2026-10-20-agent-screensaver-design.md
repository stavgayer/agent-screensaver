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
2. It shows agent status for Claude Code and Codex: waiting for approval, running (with time), finished. Cursor shows running and finished; its shell and MCP commands show on the working card, never as approvals (§7d). Users can add other agents as agent extensions (§7d).
3. It shows usage limits: Claude Max session (5 hours) and weekly, Codex session (5 hours) and weekly, in percent, with the time until reset.
4. When an agent waits for approval, the screen makes it obvious from far away, and a small sound plays.
5. It is fast and light. This is the top priority.
6. The background can change: Matrix rain (default), a photo or video of the user's choice, or Apple aerial videos that macOS already downloaded.

### Non-goals (not in version 1)

- Linux and Windows. The design must not block them later, but no work goes there now.
- Cursor usage limits. Cursor keeps usage on its server only; it is reachable only with the login token or cookies through undocumented endpoints (research file `cursor-usage-research.md`, 24/10/2026). (Cursor task status is in version 1, as a built-in agent extension, §7d. Owner decision, 22/10/2026.)
- Jira, GitHub, calendar, Slack, social networks (Reddit, Bluesky, YouTube, Gmail). The card model (section 6) must make them easy to add later.
- A drag-and-drop layout editor.
- Running another company's screen saver inside ours.

### Anti-goals (failure even if it "works")

- The Mac does not lock while the screen saver shows.
- The screen saver keeps using CPU or battery after it stops.
- Any data leaves the Mac. No analytics, no server of ours.
- The code uses undocumented endpoints (for example `api.anthropic.com/api/oauth/usage` or `chatgpt.com/backend-api/wham/usage`), browser cookies, or copies login tokens.
- The project downloads, ships, or makes real copies of Apple video files. (APFS clones of videos macOS already downloaded are allowed; they share the original's disk blocks and are removed on uninstall.)
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

Since 22/10/2026 the table above is not code. Each tool is an agent extension (§7d): its manifest says which hook events map to which event kinds, and where its hooks go. Claude Code, Codex and Cursor ship as built-in agent extensions. Users can add others.

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
Claude Code / Codex / Cursor / other agents: hooks and status line
        │  hook payload (JSON on stdin)
        ▼
agent-screensaver hook --tool <source>
  - finds the agent extension for <source> (§7d)
  - maps the payload with the manifest's rules
    (or runs the extension's converter program)
        │  small events (JSON, spool folder)
        ▼
Go background service (always running, light)
  - keeps the current state in memory
  - reads the details the agent manifests declare
    (conversation name, context use), bounded reads
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
3. Apple aerials: Landscapes, Cities, Space, Underwater. On macOS 26 the list comes from `~/Library/Application Support/com.apple.wallpaper/aerials/manifest/entries.json` (156 entries on the owner's Mac, 21/10/2026), and the downloaded videos are `~/Library/Application Support/com.apple.wallpaper/aerials/videos/<asset id>.mov`. If that manifest is missing, the list comes from the older catalog `/Library/Application Support/com.apple.idleassetsd/Customer/entries.json` (137 entries, no videos); videos are still looked up in the wallpaper `videos` folder. We play ONLY video files macOS already downloaded. The saver's web view can load files only inside the data directory, so the daemon makes an APFS clone of each video of the chosen group in `DATA_DIR/media/aerials/<asset id>.mov` (it never makes a real copy; a video that cannot be cloned is skipped), removes clones that are no longer chosen or whose video is gone, and uninstall removes the folder. If none are downloaded for the chosen group, settings show: "Pick it once in System Settings → Screen Saver, and macOS will download the videos." These folders are not an official API. If Apple moves them, this background must fail quietly and fall back to Matrix.

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
| Agent | Data that teaches the service one coding agent: a mark for its cards, how its hook payloads become events, how to add and remove its hooks, where to read extra details (§7d). Claude Code, Codex and Cursor are built-in agent extensions | Data read by the Go service and the `hook` command; an optional converter program runs outside the saver |
| Source | Any program that sends cards and limits into the service (Jira or GitHub later). It needs no manifest | Outside the saver |

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
| Agent extension (manifest `type: "agent"`, §7d): data, not a Go interface. One generic Go mapping engine and one install engine read it | `builtin.claude`, `builtin.codex`, `builtin.cursor` | opencode, pi, Gemini CLI, others, added by users without Go changes |
| `install` config format (small interface inside `internal/install`): reads a tool config, adds and removes our hook entries, writes it back | JSON (`internal/jsonedit`) | TOML, when a byte-preserving TOML editor exists |
| `install` hook shape (small interface inside `internal/install`): where our entry sits inside the hooks object | `grouped` (Claude Code, Codex), `flat` (Cursor) | a new shape only when a real tool needs one |
| `limits.Provider`: reads usage limits on a schedule | Codex (`codex app-server`). An agent manifest names it by id (§7d) | other tools with a local limits source |
| `platform.ServiceManager`: starts and stops a background process at login (the daemon and the tray) | launchd (macOS) | systemd (Linux), Task Scheduler (Windows) |
| `platform.SaverWatcher`: tells when the screen saver starts and stops | macOS distributed notifications | Linux, Windows |
| `platform.SoundPlayer`: plays the attention sound | `afplay` (macOS) | Linux, Windows |
| `platform.FilePicker`: native open-file dialog for installing extensions | `NSOpenPanel` (macOS) | Linux, Windows |

Each provider or platform piece registers itself in one list. Adding a new agent tool needs no Go code: the user installs an agent extension folder (§7d). Claude's status line is declared in the `builtin.claude` manifest too; the Go part is the generic status line runner, which any agent manifest can use. (Before 22/10/2026 each tool was a Go `agents.Adapter`. §7d replaces it.)

When two principles pull against each other, the order is: correct, then simple and readable, then efficient. A small amount of repetition is fine when removing it would make the code harder to read.

## 7c. Menu bar app (owner decision, 20/10/2026)

Version 1 includes a menu bar app. It replaces "settings window later" from §7a. It is built so the same code runs on macOS, Windows (system tray) and Linux (AppIndicator tray) later; version 1 ships and tests it on macOS only.

### How it is built

- It is part of the same Go binary. The tray icon and menu use `fyne.io/systray`, the only third-party Go library in the project. The settings window is our own small window behind a `platform.SettingsWindow` interface: on macOS an `NSWindow` with a `WKWebView` and a script-message bridge, through cgo (owner decision, 21/10/2026, after the library check in docs/decisions/0003-tray-and-webview.md found `webview_go` unmaintained). Windows (WebView2) and Linux (WebKitGTK) get their own small implementations later.
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
- Connections: a tab with one line per agent extension, its state (not installed, no hooks, active, idle, off), the time of its last event, and buttons to add or remove its hooks and to turn it on or off (§7d).

## 7d. Agent extensions (owner decision, 22/10/2026)

### The problem

Users run many coding agents: Claude Code, Codex, Cursor, opencode, pi, Hermes and more. Each tool has its own hook events, payload fields and config file. Until now each tool was Go code (`agents.Adapter`). A user could not add a tool without changing our code.

The owner decided: support for an agent is an extension. Users add and remove agents without changing our code. Claude Code, Codex and Cursor ship as built-in agent extensions in the same format, with no private shortcuts (§7a).

### What an agent extension is

It is an extension folder with `extension.json` of type `agent`. It holds data, and at most two small optional programs (converters). The folder layout:

```
acme.opencode/
  extension.json
  mark.svg              (required, the small drawn mark for the card)
  plugin/agent-screensaver.js   (only if the recipe copies a plugin file)
  bin/convert           (only if the manifest uses a converter)
```

The manifest has the common fields of §7a (`id`, `name`, `version`, `type`, `api`) and one `agent` object with five parts:

| Part | Required | What it says |
|---|---|---|
| `source`, `mark` | yes | The event `source` name (for example `cursor`) and the mark file. |
| `hooks` | yes | How a hook payload becomes an event: mapping rules, or a converter program. |
| `install` | yes | Where the hooks go, which events, and the file shape. |
| `statusline` | no | A status line payload mapping, for a tool that has a status line command (Claude Code). |
| `details` | no | Where the service reads extra details: conversation name, context use, model. |
| `limits` | no | The id of a built-in Go limits provider (`codex-app-server` and `claude-usage-estimate` in version 1). |

### Manifest fields

| Field | Type | Rule |
|---|---|---|
| `agent.source` | string | Lowercase letters, digits and dashes, 1 to 32 characters. It is the event `source`, the `--tool` value and the card's source. Two installed agents cannot have the same source. An install that would make a second one is refused. |
| `agent.mark` | string | Path of an SVG file inside the folder. At most 8 KiB. See "The mark". |
| `agent.hooks` | object | See "The mapping language" and "Converter programs". |
| `agent.install` | object | See "The install recipe". |
| `agent.statusline` | object | Optional. A mapping with the same language as `hooks`, plus `install.statusline`. |
| `agent.details` | object | Optional. See "Details". |
| `agent.limits` | object | Optional. `{ "provider": "codex-app-server" }` or `{ "provider": "claude-usage-estimate" }`. An unknown id is refused. |

An agent manifest has no `entry` and no `options` in version 1.

### Example: the built-in Cursor extension

The Cursor field names come from https://cursor.com/docs/hooks (fetched 23/10/2026) and the research file `.superpowers/sdd/2026-10-20-agent-screensaver-v1/richer-cards-research.md`. Task 6.5 checks them against real payloads.

```json
{
  "id": "builtin.cursor",
  "name": "Cursor",
  "version": "1.0.0",
  "type": "agent",
  "api": 1,
  "agent": {
    "source": "cursor",
    "mark": "mark.svg",
    "hooks": {
      "event": "hook_event_name",
      "session": "conversation_id",
      "common": {
        "project": { "path": ["workspace_roots.0", "cwd"], "as": "basename" },
        "transcript": "transcript_path"
      },
      "rules": [
        { "on": "beforeSubmitPrompt", "kind": "started",
          "set": { "title": { "path": "prompt", "as": "prompt_title" },
                   "model": { "path": "model", "as": "model_name" } } },
        { "on": "beforeShellExecution", "kind": "tool_intent",
          "set": { "title": { "path": "command", "as": "short_title" } } },
        { "on": "beforeMCPExecution", "kind": "tool_intent",
          "set": { "title": { "path": "tool_name", "as": "short_title" } } },
        { "on": ["afterShellExecution", "afterMCPExecution", "postToolUse"], "kind": "resumed" },
        { "on": "stop", "kind": "finished",
          "set": { "model": { "path": "model", "as": "model_name" } } },
        { "on": "sessionEnd", "kind": "ended" },
        { "on": "preCompact", "kind": "info",
          "set": { "context_percent": { "path": "context_usage_percent", "as": "percent" } } }
      ]
    },
    "install": {
      "detect": "~/.cursor",
      "hooks": {
        "file": "~/.cursor/hooks.json",
        "format": "json",
        "shape": "flat",
        "new_file": { "version": 1 },
        "handler": { "timeout": 5 },
        "events": ["beforeSubmitPrompt", "beforeShellExecution", "afterShellExecution",
                   "beforeMCPExecution", "afterMCPExecution", "postToolUse",
                   "stop", "sessionEnd", "preCompact"]
      }
    }
  }
}
```

**Cursor commands are working steps, not approvals (owner decision, 24/10/2026; replaces the 23/10 approval cards).** Cursor has no approval event that only reports. Its events before a command are permission hooks: Cursor reads the hook's answer, and an invalid answer blocks the action. The built-in Cursor extension subscribes to `beforeShellExecution` and `beforeMCPExecution`, and our hook prints nothing for them, so Cursor decides as it would without us. It first answered `{"permission": "ask"}`; on 24/10/2026 (Cursor 3.27.12, "Run Everything") that answer made Cursor wait 49 minutes for an approval it would not have asked for, while the card said "Working" (Task 6.7). Our hook never answers allow or deny (a hard rule of the engine, see "Safety rules"). The two events map to `tool_intent`: the working card shows the command or the MCP tool name. They are not approvals, because Cursor does not tell the hook whether it will really ask the user, and in "Run Everything" mode it sends them before every command, which would show a false approval card and play its sound for each one. So Cursor never makes an approval card. `afterShellExecution` and `afterMCPExecution` map to `resumed`.

- `preToolUse` is not used. The docs say Cursor accepts `ask` there but does not enforce it today, so its effect is unclear.
- Risk: Cursor can run some commands without asking (an allow list or auto-run mode). The docs do not say whether a hook's `ask` makes Cursor ask for such a command. Task 6.5 tested this live on the owner's Mac, with and without our hook. Answer (24/10/2026): Cursor 3.27 does obey `ask`, so the engine no longer has an `ask` reply (see `reply` below).
- Task 6.5 (live test, 24/10/2026, Cursor 3.26.16, run mode "Run Everything"): Cursor behaved the same with and without our hooks, and showed no error when our hook printed nothing, so the reply default is `none`. The model is set only on `beforeSubmitPrompt` and `stop`, the events that carry the full model name. A run mode where Cursor asks was not tested. Results: `docs/e2e-checklist.md`.
- Cursor also runs the Claude Code hooks of `~/.claude/settings.json` with its own payloads. `builtin.claude` ignores payloads that have `cursor_version`, so a Cursor chat is never also a Claude card.

### The mapping language

The mapping is data. It has no scripting, no loops and no expressions. It has field paths, constants, a short list of named transforms, and simple matching on the event name or on a field value. The Go engine reads it.

**Field paths.** A path is keys joined by dots, for example `tool_input.command`. A number picks an array item: `workspace_roots.0`. A key that holds a dot cannot be named; such a tool needs a converter.

**Values.** Every field we set takes a value in one of these forms:

| Form | Meaning |
|---|---|
| `"a.b"` | Short form of `{ "path": "a.b" }`. |
| `{ "path": "a.b" }` | The value at that path. A missing path gives no value. |
| `{ "path": ["a", "b"] }` | The first path that gives a non-empty value. |
| `{ "const": "text" }` | A fixed string, number or boolean. |
| `{ ..., "as": "<transform>" }` | The value, changed by one transform. |
| `{ "tool": "tool_name", "input": "tool_input" }` | The tool description used today (`Bash: npm run migrate`): tool name plus the command or file, masked and cut. |

Transforms (all in Go, the same functions the built-ins use today):

| Transform | Result |
|---|---|
| `basename` | The last folder name of a path. |
| `prompt_title` | The card title from a prompt: skips injected tag blocks and banners, masks secrets, cuts to 40 characters (`docs/event-format.md`). |
| `short_title` | First line, secrets masked, cut to 40 characters. |
| `model_name` | The short model name the cards show. |
| `count` | The length of an array. No array gives no value. |
| `percent` | A number, clamped to 0 to 100. |
| `text` | A string, or an array of strings joined with spaces. |

Every text that can reach the screen (`title`, `name`, `model`, `project`) goes through secret masking, whatever the mapping says.

**Mapping fields.**

| Field | Meaning |
|---|---|
| `event` | Path of the hook event name in the payload. Or `{ "arg": "event" }` when the tool does not send it: then the recipe adds `--event <name>` to the command. |
| `session` | Value of the session id. No value: the payload is ignored. |
| `common` | Fields set on every event, before the rule's own `set`. |
| `rules` | A list of rules. The first rule that matches wins. No match: the payload is ignored. |
| `reply` | What our hook prints on stdout, for tools that want an answer. A string for every event, or `{ "default": <reply>, "events": { "<event>": <reply> } }` for a fixed reply per event. A reply is one of: `none` (print nothing, the default), `empty_object` (print `{}`). The engine refuses any other value (`allow`, `deny`, `ask` and the rest), so a manifest can never make our hook answer a tool's permission question. An older `ask` reply was removed on 24/10/2026 (owner decision): Cursor 3.27 obeyed it and waited for an approval the user had not asked for. Commands written by older installs can still carry `--reply ask`; the hook accepts the flag and prints nothing, and uninstall still removes those commands. |

**Rules.**

```json
{ "on": "Notification", "if": { "path": "notification_type", "equals": "permission_prompt" }, "kind": "approval" }
```

| Field | Meaning |
|---|---|
| `on` | One event name or a list of names. Left out, the rule matches every event. |
| `if` | Optional. One condition or a list; all must hold. |
| `kind` | An event kind from `docs/event-format.md` (`started`, `tool_intent`, `approval`, `resumed`, `finished`, `ended`, `limits`, `info`), or `ignore`. |
| `set` | Fields of the event: `project`, `title`, `name`, `model`, `context_percent`, `background`, `cwd`, `transcript`, and for `limits` a `limits` list. |

A condition is `{ "path": "...", <operator> }` with exactly one operator:

| Operator | True when |
|---|---|
| `"equals": x` | The value equals `x` (string, number, boolean or `null`). |
| `"in": [x, y]` | The value equals one of the list. |
| `"contains": "text"` | The value is a string that contains the text, ignoring case. |
| `"present": true` | The key exists and is not `null`. `false` means the opposite. |

This is enough for every rule the Go adapters have today. Two examples from `builtin.claude` and `builtin.codex`:

```json
{ "on": "Notification", "if": { "path": "notification_type", "equals": "permission_prompt" }, "kind": "approval" },
{ "on": "Notification", "if": { "path": "notification_type", "present": true }, "kind": "ignore" },
{ "on": "Notification", "if": { "path": "message", "contains": "permission" }, "kind": "approval" },
{ "on": "Stop", "kind": "finished", "set": { "background": { "path": "background_tasks", "as": "count" } } }
```

```json
{ "if": { "path": "transcript_path", "equals": null }, "kind": "ignore" }
```

(The Codex helper-session rule. It is the first rule, with no `on`, so it applies to every event. `equals: null` is true only when the key exists and is `null`, as the Go code checks today.)

**Status line mapping.** `agent.statusline` uses the same fields, without `event` and `on`. Every rule whose `if` holds makes one event, so one status line payload can give a `limits` event and an `info` event. An event with no fields beyond its keys is dropped. A `limits` reading is `{ "window": "5h", "used_percent": <value>, "resets_at": <value> }`; a reading without `used_percent` is dropped.

### Converter programs (the escape hatch)

Some payloads cannot be mapped with the language above. Then `hooks` (or `statusline`, or a `details` source) names a program instead of rules:

```json
"hooks": { "converter": { "run": "bin/convert", "timeout_ms": 200 } }
```

The contract:

- `run` is a path inside the extension folder. The file must be a regular file with the execute bit. It runs directly, with no shell and no arguments.
- Standard input: the raw payload, at most 1 MiB, the same bytes the tool gave our hook.
- Environment: only `HOME`, `PATH`, `AGS_SOURCE` (the manifest source) and `AGS_EVENT` (the `--event` value, or empty).
- Standard output: zero to 4 events, one JSON object per line, in the format of `docs/event-format.md`, at most 64 KiB in total. Empty output means "ignore".
- Our hook fills `v` and `at_ms` when missing, and always sets `source` to the manifest's source: a converter cannot send events as another agent. Then it masks every text field, cuts titles, and validates each event with the normal event rules. An invalid event is dropped.
- Timeout: `timeout_ms`, default 200, at most 500. On timeout the program is killed and nothing is sent.
- Errors go to `hook-errors.log`. Our hook still exits 0 and prints only what `reply` says. The reply comes from the manifest, never from the converter.
- A converter runs only while its agent is on, and only after the user agreed to "this agent runs its own program" (see "Safety rules").

A converter is code. The mapping is the normal way; a converter is for the rare tool that needs it. No built-in extension uses one.

### The install recipe

`agent.install` says how to add and remove our hooks. The Go install engine does the work, with `internal/jsonedit`, for every agent the same way.

| Field | Meaning |
|---|---|
| `detect` | Optional path. When it does not exist, `agent-screensaver install` skips this agent (owner decision, 23/10/2026) and the Connections tab shows "not installed". |
| `hooks` | The hook config part. Required, unless `files` is set (a tool whose hooks are plugin files has no hook config). |
| `hooks.file` | The tool's hook config file. A path that starts with `~/`. |
| `hooks.format` | `json`. (`toml` is reserved and refused in version 1: no TOML editor keeps a file byte for byte with the standard library.) |
| `hooks.shape` | `grouped`: `hooks.<Event>` is a list of groups, each with its own `hooks` list of handlers (Claude Code, Codex). `flat`: `hooks.<event>` is a list of handlers (Cursor). |
| `hooks.events` | The events to add our handler to. |
| `hooks.group` | `grouped` only. Extra keys on our group, for example `{ "matcher": "" }` for Claude Code. |
| `hooks.handler` | Extra keys on our handler, for example `{ "timeout": 5 }`. |
| `hooks.new_file` | Optional. Top-level keys written only when the file does not exist yet, for example `{ "version": 1 }` for Cursor. |
| `statusline` | Optional. `{ "key": "statusLine" }`: our status line command takes the place of `<key>.command` in the same file as the hooks, and the user's command is saved and still runs (§5 "Status line"). |
| `files` | Optional. Files copied from the folder to the user's home, for tools whose hooks are plugin files (opencode, pi): `[{ "from": "plugin/agent-screensaver.js", "to": "~/.config/opencode/plugins/agent-screensaver.js" }]`. |
| `notes` | Optional. Short lines shown after install, for example "In Codex, approve the new hooks once with /hooks." |

The command is never in the manifest. The engine always writes exactly `'<bin>' hook --tool <source>` (plus ` --event <name>` when the mapping uses `{ "arg": "event" }` or the event has its own reply, and ` --reply <reply>` when the event's reply is not `none`, so the hook still answers when the manifest cannot be read), and for the status line `'<bin>' statusline --tool <source>`. Our handler is `{ "type": "command", "command": <ours>, <handler keys> }`.

Rules the engine keeps for every agent (they are the rules of §5 and the plan's Global Constraints, now in one engine):

- Install only adds. It never changes or removes an entry it did not write.
- Uninstall removes exactly what the install record lists, and nothing else. A group, an event list or the `hooks` key goes only when our removal emptied it. A file that install created goes only when nothing but `new_file` keys is left.
- Key order stays. Values we do not touch keep their content. (Note: `jsonedit` writes the file with two-space indentation, so the whitespace of a file indented differently changes. This is true today too.)
- Before the first write to a file, it writes `<file>.agent-screensaver.bak`, once.
- Every file is read, parsed and edited in memory first. If one file is invalid, nothing is written.
- Running install twice adds nothing the second time.

**The install record** (version 2) lists, per agent: the id and source, the hook file, the events, each exact command written, whether the file was created, the status line key and the saved command, and each copied file with its SHA-256. Uninstall of an agent uses only this record. A record of version 1 (today's `agents: ["claude", "codex"]`) is read as the built-in Claude and Codex entries with today's commands. An upgrade removes the old entries and adds the new ones in one in-memory edit, so each file is written once.

### Safety rules for recipes

These rules apply to every agent extension. Built-in ones go through the same checks; a test runs the checker on every built-in manifest.

What a manifest may make us write:

1. Our handler, inside the top-level `hooks` object of the declared hook file, for the declared events.
2. The `new_file` keys, only when we create the file.
3. The `command` of the declared status line key, in the same file as the hooks.
4. The declared `files`, only when the target does not exist or holds our earlier copy (same SHA-256).

What a manifest may not do:

1. Choose the command. It is always our binary.
2. Write outside the user's home. Every path starts with `~/`, has no `..`, and stays under home after symlinks are resolved.
3. Write to these places: our data directory, `~/Library/LaunchAgents`, `~/Library/Keychains`, `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.config/gh`, `~/.gitconfig`, and shell start files (`~/.zshrc`, `~/.zprofile`, `~/.zshenv`, `~/.zlogin`, `~/.zlogout`, `~/.bashrc`, `~/.bash_profile`, `~/.bash_login`, `~/.bash_logout`, `~/.profile`, `~/.config/fish/`). The check compares paths case-insensitively, as APFS does.
4. Use a hook file that does not end in `.json` (version 1).
5. Set handler or group keys other than `matcher` (string) and `timeout` (number, 1 to 30). `command`, `type` and keys such as Cursor's `failClosed` are refused, so our hook can never block the tool.
6. Make our hook decide for the user. Our hook prints only what `reply` allows: nothing or `{}`. It never prints allow, deny or ask. It prints the reply first, before it maps anything, and also when the payload is invalid, the agent is off or a converter fails, so the tool always gets a valid answer at once. When an event has its own reply, the engine adds `--event <name>` to that event's command, so the reply never depends on reading the payload.
7. Run a converter or copy a plugin file without a clear warning and the user's explicit confirmation (owner decision, 23/10/2026: allowed after both). The plan says so in its own line, for example: "acme.opencode runs its own program bin/convert on each opencode event" or "Copy a plugin file that opencode will run: ~/.config/opencode/plugins/agent-screensaver.js".

Before any write, install shows the plan: each file, each event, each command, each copied file, and whether the agent is built in or from outside (with its id and version). The CLI asks `Proceed? [y/N]`. The settings window shows the same lines in a dialog with "Add hooks" and "Cancel". The extension is installed (copied into the extensions folder) first; its hooks are added only after this confirmation. Removing an agent extension (`ext remove`) removes its hooks first; if that fails, the extension stays.

### Details: conversation name, context use, model

`agent.details` says where the daemon reads extra details for a card. The hook never reads them: hooks must stay fast. The daemon reads them after `started`, `finished` and `ended` events of that source (the list is `read_on`, default these three), at most once per card per 5 s.

```json
"details": {
  "name": [
    { "jsonl": "~/.codex/session_index.jsonl",
      "last": { "path": "id", "equals": "{session}" },
      "value": "thread_name" }
  ],
  "context_percent": [
    { "jsonl": "{transcript}",
      "last": { "path": "payload.type", "equals": "token_count" },
      "value": { "ratio": ["payload.info.last_token_usage.total_tokens", "payload.info.model_context_window"] } }
  ]
}
```

| Field | Meaning |
|---|---|
| `name`, `context_percent`, `model` | Each a list of sources, in order. The first source that gives a value wins. |
| `jsonl` | A file of JSON lines. The daemon reads only its last `tail_kb` kilobytes (default 256, at most 1024), drops the first partial line, and takes the last line that matches `last`. |
| `json` | A JSON file of at most 1 MiB, read whole. |
| `last` | A condition, the same as a rule's `if`. |
| `value` | A value, the same as in a rule, plus `{ "ratio": [a, b] }`: a divided by b, as a percent. |
| `converter` | The escape hatch: `{ "run": "bin/details", "timeout_ms": 1000 }` (at most 2000). Standard input `{"session", "cwd", "transcript"}`; standard output one object with any of `name`, `context_percent`, `model`. The daemon runs details reads off its main loop, so a slow converter never holds up events. |
| `provider` | A Go details reader named by id. Version 1 has one: `claude-transcript-context` (Claude's context use from its transcript and the window sizes learned from the status line, which data cannot express). |

Placeholders in file paths: `~/`, `{transcript}` and `{cwd}` (from the card's last event). In conditions: `{session}`. Every file must be a regular file under the user's home after symlinks are resolved. The daemon only reads. A value that arrived in an event wins over a value read here (for example the Claude status line `session_name` wins over the transcript title).

The git branch is not declared. The daemon reads it the same way for every agent, from `.git/HEAD` under the card's `cwd` (Task 6.3).

Claude's `custom-title` and `ai-title` transcript lines and Codex's `session_index.jsonl` and rollout `token_count` lines fit this schema (research file, sections 1 and 3). Cursor has no declared details in version 1: its name lives in an undocumented SQLite file, and its context use comes only with `preCompact`.

**Limits stay Go for version 1.** Codex limits come over JSON-RPC from `codex app-server`. Declaring a JSON-RPC session as data would need a small protocol language; that is too much for one user. So `limits.Provider` stays a Go interface, and `builtin.codex` names it with `"limits": { "provider": "codex-app-server" }`. The live Claude estimate (Task 6.3b: exact readings from local files plus a transcript token count) is Go for the same reason; `builtin.claude` names it with `"limits": { "provider": "claude-usage-estimate" }`. Any agent can send limits another way: through its status line mapping, a converter, or any program that writes `limits` events (`docs/event-format.md`).

### The mark

The card shows the agent's mark next to the state line. The mark is a small drawn SVG, made by us for the built-ins: simple shapes, own colors, nothing that looks like an official logo (owner decision, 22/10/2026). Product names appear as plain text.

The service checks every mark before it accepts the extension: at most 8 KiB, an `<svg>` root, and none of `<script>`, `<foreignObject>`, `on...=` attributes, `href` that does not start with `#`, `url(` that does not start with `url(#`, or `@import`. The runtime shows it with `<img src>`, so even a mark that passed the check by mistake cannot run code or load anything. `state.json` gives the runtime, per source, the agent's name and the mark path inside the data directory. A card whose source has no agent extension (any program that writes events) shows a neutral dot and the source name.

### What stays in Go

| Go code | Why |
|---|---|
| The mapping engine (`internal/agents`): loads agent manifests, finds one by source, applies rules, runs converters. | One engine for every agent, tested once. |
| The transforms (`basename`, `prompt_title`, `model_name`, the tool description, secret masking). | They are the shared rules of the card text. A manifest can only name them. |
| The install engine (`internal/install`): the plan, safety checks, config formats, hook shapes, record, backups. | It edits the user's real files; it must be one careful piece of code. |
| The status line runner (`agent-screensaver statusline --tool <source>`): maps the payload with the manifest, runs the saved user command with the 300 ms timeout, prints its output. | The pass-through is behavior, not data. Any agent with a status line can use it; only Claude Code has one today. |
| The details reader in the daemon, and the git branch reader. | Bounded file reads with no hidden inputs. |
| The Codex limits provider. | JSON-RPC (see above). |

The hook command stays `agent-screensaver hook --tool <source>`. It reads the config (to see if the agent is off) and the agent manifests in the extensions folder, maps the payload, writes the event, and exits 0. It stays under 100 ms with 50 installed extensions (test in Task 6.4).

### Moving Claude Code and Codex

- `builtin.claude` and `builtin.codex` are written from today's Go adapters (`internal/agents/claude.go`, `codex.go`, `install_claude.go`, `install_codex.go`). A test maps every fixture in `internal/agents/testdata` through the old adapter and through the manifest and requires the same event. Only then are the old adapters deleted.
- The install tests of today run unchanged against the manifests. For Claude and Codex the written files are byte for byte the same as today.
- The hook commands of today stay the same (`hook --tool claude`, `hook --tool codex`), so a user's installed hooks keep working without a reinstall. The status line command becomes `statusline --tool claude`; the next install changes it through the upgrade path, and the old command keeps working until then.
- The Claude 6.3 details (status line `session_name`, `context_window.used_percentage`, `model`; transcript titles) and the Codex 6.3 details (session index name, rollout context) move into the manifests. The Go readers that Task 6.3 adds become the generic details reader.

### Turning agents on and off

The config gets `agents_off`: a list of agent ids. An agent that is off keeps its hooks (owner decision, 23/10/2026), but it is silent: its hook prints its reply and exits at once (no mapping, no converter), and the daemon drops events with its source. Turning an agent on does not add hooks; adding and removing hooks are separate actions, because they change the user's files.

CLI: `agent-screensaver agent list`, `agent on|off <id>`, `agent hooks add|remove <id>` (each shows the plan and asks first). `install` adds hooks for every built-in agent whose `detect` path exists; `uninstall` removes every agent's hooks from the record.

### Last event per source

The daemon records, for each source, the time of the last valid event it read from the spool (`last_event_ms`). Events dropped for an ignored project still count: they prove the hooks work. It writes this map to `state.json` as `sources: { "<source>": { "last_event_ms": ... } }` and reads it back when it starts, so a restart does not lose it. The saver does not use it.

### The Connections tab in the settings window

The settings window has a tab "Connections" (owner decision, 23/10/2026). It lists every installed agent extension, one row each: the mark, the name, "built in" or "from <id> <version>", the state, the time of the last event ("last event 4m ago", or "no events yet"), and the buttons.

| State | When | Text |
|---|---|---|
| Not installed | The `detect` path does not exist: the tool is not on this Mac. | "Not installed" |
| No hooks | The tool is found, and the install record has no hooks for this agent. | "Hooks not added" |
| Active | On, hooks in place, and an event in the last 10 minutes. | "Active · 4m ago" |
| Idle | On, hooks in place, and no event for 10 minutes or more, or none yet. | "Idle · 3h", "Idle · no events yet" |
| Off | The id is in `agents_off`. | "Off" |

Ten minutes (my call): a working agent sends an event at least on each prompt, tool call and stop, so 10 minutes without one means nobody is using it. It is the same number as the approval-to-unknown rule (§4). When the record says our hooks are added but the file no longer holds our entries, the row also says "Hooks were changed outside agent-screensaver" and offers "Add hooks".

Buttons: "Add hooks" or "Remove hooks" (whichever applies), and "Turn off" or "Turn on". The hook buttons first show the plan in a dialog. Nothing changes before the user presses "Add hooks" or "Remove hooks" in the dialog. For an outside agent with a converter or a plugin file, the dialog shows the warning lines of "Safety rules" and the confirm button says "I trust <name>, add hooks". Installing a new agent uses the existing "Install extension" button.

The settings bridge rules stay: the page sends only agent ids and choices, never paths or commands; every call maps onto one `settings.Service` method; calls are refused while a file dialog is open. The plan is computed in Go twice: once to show it and once to apply it. The apply call carries the hash of the plan the page showed; if Go's new plan has another hash (a file changed in between), it refuses and the page shows the new plan.

### Other platforms

Paths in recipes start with `~/` and are joined with the platform's separator, so a recipe names no macOS folder unless the tool keeps its config there. A manifest may give per-platform values later (`"file": { "darwin": "...", "linux": "..." }`); version 1 reads a plain string only. The hook command quoting is behind the install engine, so Windows (PowerShell, which Cursor uses there) gets its own quoting later.

### Fit with other agents (checked 23/10/2026)

- **opencode** (https://opencode.ai/docs/plugins/): it has no JSON hook config. Plugins are JavaScript or TypeScript files that load from `~/.config/opencode/plugins/`, with events such as `session.idle`, `session.status`, `permission.asked`, `permission.replied`, `tool.execute.before` and `tool.execute.after`. An opencode agent extension copies a small plugin file with `install.files`, and that plugin runs `agent-screensaver hook --tool opencode --event <name>` with a JSON payload on stdin. The mapping then uses `{ "arg": "event" }`. `permission.asked` maps to `approval` and `permission.replied` to `resumed`, which is better than Codex. The recipe needs no hook file (`install.hooks` is optional when `install.files` is set).
- **pi** (https://pi.dev/docs/latest/extensions): it has no JSON hook config either. Extensions are TypeScript or JavaScript files in `~/.pi/agent/extensions/`, with events such as `session_start`, `session_shutdown`, `agent_start`, `agent_end`, `tool_call` and `tool_result`. pi has no built-in approval prompt; approval gates are extensions themselves. A pi agent extension copies a plugin file the same way as opencode. `agent_start` maps to `started`, `agent_end` to `finished`, `session_shutdown` to `ended`.

Both fit the design with `install.files` and `{ "arg": "event" }`. Hermes was not checked.

### Anti-goals for agent extensions

- A scripting language in the mapping (no JavaScript, no expressions, no templates beyond the placeholders above).
- Loading outside code into the Go service. Converters are separate processes.
- A manifest that chooses the command a tool runs, or writes outside the user's home.
- Any change to entries we did not write, or any answer to a tool's permission question (allow, deny or ask).
- A hook that slows the tool: over 100 ms without a converter, or over the converter timeout with one.
- Network access by the hook or the details reader. (We cannot stop a converter program from using the network. The plan tells the user that the agent runs its own program, so the user decides.)
- Official logos in the repository.
- An agent store or downloads (as in §7a).
- TOML editing in version 1.

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
7. An outside agent extension adds hooks to a tool's config, and may copy a plugin file or run a converter program. The safety rules of §7d limit what it can write, and the user sees and confirms each change, but a converter or plugin is still code the user chose to trust.
8. Hook payload fields are not a stable API of the tools. When a tool changes a field, a manifest fix is enough, and a user can ship it without waiting for our release.

## 10. Left to the builder

- Name of the binary and of the bundle.
- How hook events reach the service (Unix socket, local file, or similar), as long as it is local only and fast.
- How the saver learns about state changes (reading a file on change, or a local socket).
- Project layout, build scripts, tests, CI.
- Exact sound file (must be free to use and small).
- The open-source licence (MIT or Apache-2.0 suggested).
