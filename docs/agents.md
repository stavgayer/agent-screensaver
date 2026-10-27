# Agent extensions

This page tells you how to teach agent-screensaver a new coding agent. You write an extension folder. You do not change our code.

Claude Code, Codex and Cursor are built-in agent extensions in the same format: `extensions/builtin.claude`, `extensions/builtin.codex` and `extensions/builtin.cursor`. Read them as examples. `builtin.cursor` shows the flat hook shape, a per-event reply and `new_file`.

A short step-by-step guide with tested samples is in [guides/add-an-agent.md](guides/add-an-agent.md). The design and the reasons are in the spec (§7d) and in `docs/decisions/0006-agent-extensions.md`.

## The folder

```
acme.opencode/
  extension.json
  mark.svg                      (required)
  plugin/agent-screensaver.js   (only if the recipe copies a plugin file)
  bin/convert                   (only if the manifest uses a converter)
```

Install it like any extension: `agent-screensaver ext install ./acme.opencode`, or "Install extension" in the settings window. Installing checks the manifest and the recipe and copies the folder. It does not add hooks. Add them as a second step: `agent-screensaver agent hooks add <id>` (or "Add hooks" in the Agents tab). It shows its plan first and needs the install record that `agent-screensaver install` writes.

### Installing a new version

When you install a new version of an agent extension that you did not get with agent-screensaver (the same `id` is already installed), the install first compares every file of the new folder with the installed copy. A converter can read or run any file next to it, so every file counts, not only the programs. If a file changed, is new, or is gone, the install shows the changes and asks before it replaces anything. The changes come in three groups:

- **Manifest**: `extension.json`, always as a text diff. It says what runs and what is read.
- **Programs and copied files**: the converter programs (`agent.hooks.converter`, `agent.statusline.converter` and every details converter) and every file that `agent.install.files` copies.
- **Other files**: every other file of the extension.

A text file (valid UTF-8, no NUL byte) gets a unified diff. Any other file shows its old and new size and the first 12 digits of its SHA-256. A file whose diff is longer than 2 MB is shown as "too large to show: review the files by hand", with the paths of the installed copy and of the new one. Characters that could hide or reorder text are shown as escapes: control characters as `\x1b`, and bidi and other invisible characters as `<U+202E>`.

`agent-screensaver ext install`:

- Shows the first 200 lines of each diff and the first 500 characters of each line. When it cuts anything, it says so and does not ask: run it again with `--full`, which prints all of it, and then it asks.
- Asks `Install this upgrade? [y/N]`. Any answer other than `y` or `yes`, and a stdin that gives no answer, leave the installed copy as it is.
- `--yes` skips the question; the changes are still printed. A file too large to show needs `--yes`: read the files by hand first.

"Install extension" in the settings window shows the same changes in a dialog, with the whole diff of each file in a scrollable monospace box. A line longer than 500 characters ends in a button that shows the rest. Install and Cancel close the dialog. When a file is too large to show, the button says "I reviewed the files by hand, install".

When you confirm, both copies are checked again. If either changed after you saw the review, the install is refused and the installed copy stays. When no file changed at all, the install runs at once and says that no files changed. In both cases the hooks you added stay.

An installed agent is not replaced by an extension of another type with the same `id`, because its hooks would stay with no agent to read them. Remove the agent first with `agent-screensaver ext remove <id>`. Built-in agents cannot be installed again, so none of this applies to them.

## The manifest

`extension.json` has the common fields of [extensions.md](extensions.md) (`id`, `name`, `version`, `type`, `api`) with `"type": "agent"`, and one `agent` object. An agent has no `entry` and no `options`.

| Field | Required | Meaning |
|---|---|---|
| `agent.source` | yes | The event `source`. Lowercase letters, digits and dashes, 1 to 32 characters. It is also the `--tool` value. Two installed agents cannot share a source; a second one is refused. |
| `agent.mark` | yes | The SVG file of the agent's mark, inside the folder. See "The mark". |
| `agent.hooks` | yes | How a hook payload becomes events: a mapping, or a converter. |
| `agent.install` | yes | How to add and remove our hooks. See "The install recipe". |
| `agent.statusline` | no | A mapping for a status line payload, for a tool that has a status line command. |
| `agent.details` | no | Where the service reads extra details for a card. See "Details". |
| `agent.limits` | no | `{ "provider": "<id>" }`, a limits reader built into the service. Known ids: `codex-app-server` (Codex) and `claude-usage-estimate` (the live Claude estimate, see [event-format.md](event-format.md)). Any other id is refused. |

Unknown fields are refused, so a typo is an error and not a rule that silently does nothing.

## The mapping language

A mapping is data. It has field paths, constants, named transforms and simple conditions. It has no loops, no expressions and no scripting. A payload it cannot describe needs a converter.

### Mapping fields

| Field | Meaning |
|---|---|
| `event` | The path of the hook event name in the payload. Or `{ "arg": "event" }` when the tool does not send it: then every hook command gets `--event <name>`. Not used in a status line mapping. |
| `session` | The value of the session id. A payload without one is ignored (a `limits` event uses the session `limits`). |
| `common` | Fields set on every event, before the rule's own `set`. |
| `rules` | A list of rules. In `hooks` the first rule that matches wins. In `statusline` every rule that matches makes one event. No match: the payload is ignored. |
| `reply` | What our hook prints on stdout. See "Replies". Not used in a status line mapping. |
| `converter` | A program that maps the payload instead of the rules. See "Converter programs". |

### Rules

```json
{ "on": "Notification", "if": { "path": "notification_type", "equals": "permission_prompt" }, "kind": "approval" }
```

| Field | Meaning |
|---|---|
| `on` | One event name or a list. Left out, the rule matches every event. Not used in a status line mapping. |
| `if` | One condition or a list. All must hold. |
| `kind` | `started`, `tool_intent`, `approval`, `resumed`, `finished`, `ended`, `limits`, `info` (see [event-format.md](event-format.md)), or `ignore` to drop the payload on purpose. |
| `set` | Event fields: `project`, `title`, `name`, `model`, `model_id`, `cwd`, `transcript`, `context_percent`, `background`, `context_window`, and for `limits` a `limits` list. |

A `limits` list holds readings: `{ "window": "5h", "used_percent": <value>, "resets_at": <value> }`. `window` is `5h`, `week` or `plan`. A reading without a used percent is dropped.

### Values

| Form | Meaning |
|---|---|
| `"a.b"` | Short for `{ "path": "a.b" }`. |
| `{ "path": "a.b" }` | The value at that path. Keys are joined by dots; a number picks an array item (`workspace_roots.0`). A key that holds a dot cannot be named. |
| `{ "path": ["a", "b"] }` | The first path that gives a value. A missing key, `null` and `""` give no value. An empty list is a value. |
| `{ "const": "text" }` | A fixed string, number or boolean. |
| `{ "tool": "tool_name", "input": "tool_input" }` | The tool description the cards use, for example `Bash: npm run migrate`. |
| `{ ..., "as": "<transform>" }` | The value after one transform. |

Transforms:

| Transform | Result |
|---|---|
| `basename` | The last folder name of a path. |
| `prompt_title` | A card title from a prompt: it skips tag blocks and banners that the tool added, masks secrets and cuts to 40 characters. |
| `short_title` | The first line, secrets masked, cut to 40 characters. |
| `model_name` | The short model name the cards show (`claude-opus-5-5[1m]` becomes `Opus 5.5`). |
| `count` | The length of an array. Anything else gives no value. |
| `percent` | A number, kept between 0 and 100. |
| `text` | A string, or an array of strings joined with spaces. |

A field gets a value only of its type: text fields take strings, `context_percent` a number, `background` a whole number of 0 or more, `context_window` a whole number above 0. Every text that can reach the screen (`title`, `name`, `model`, `project`) is masked and cut, whatever the mapping says.

### Conditions

A condition is `{ "path": "...", <operator> }` with exactly one operator.

| Operator | True when |
|---|---|
| `"equals": x` | The key exists and its value equals `x` (a string, number, boolean or `null`). `"equals": null` is false for a missing key. |
| `"in": [x, y]` | The key exists and its value equals one of the list. |
| `"contains": "text"` | The value is a string that holds the text, ignoring case. |
| `"present": true` | The key exists and is not `null`. `false` means the opposite. |

### Events that say nothing

An `info` event without a `name`, `model` or `context_percent`, and a `limits` event without readings, are dropped.

### Replies

Some tools read our hook's answer. `reply` says what our hook prints. It is one value for every event, or `{ "default": <reply>, "events": { "<event>": <reply> } }`.

| Reply | Our hook prints |
|---|---|
| `none` | Nothing. This is the default. |
| `empty_object` | `{}` |

No other value is accepted. A manifest can never make our hook answer a tool's permission question: not allow, not deny, and not ask. (An earlier `ask` reply was removed: Cursor 3.27 obeyed it and waited for an approval the user had never asked for. See decision 0006.) Commands written by an older install can still carry `--reply ask`; our hook prints nothing for it. Our hook prints the reply first, before it reads the payload. It also prints it when the payload is invalid, when the agent is off, and when a converter fails. An event with a reply of its own gets `--event <name>` in its command, so the reply never depends on the payload.

## Converter programs

A converter is the escape hatch for a payload the mapping cannot describe:

```json
"hooks": { "event": { "arg": "event" }, "converter": { "run": "bin/convert", "timeout_ms": 200 } }
```

- `run` is a regular file (not a symlink) with the execute bit, inside the folder. It runs directly, with no shell and no arguments.
- Standard input: the raw payload, at most 1 MiB.
- Environment: only `HOME`, `PATH`, `AGS_SOURCE` (the manifest's source) and `AGS_EVENT` (the `--event` value, or empty).
- Standard output: zero to 4 events, one JSON object per line, in the format of [event-format.md](event-format.md), at most 64 KiB. Empty output means "ignore". Lines after the fourth are dropped. More than 64 KiB drops everything.
- Our hook fills `v` and `at_ms` when they are missing and always sets `source` to the manifest's source. It masks and cuts the text fields and checks each event. An invalid event is dropped.
- `timeout_ms` is 200 by default and at most 500. On a timeout the program is killed and nothing is sent.
- Errors go to `hook-errors.log` in the data folder. Our hook still exits 0.

A converter is code that you ask the user to trust. Adding its hooks shows a warning first. No built-in agent uses one.

## The install recipe

`agent.install` says how to add and remove our hooks. The service does the editing, the same way for every agent.

| Field | Meaning |
|---|---|
| `detect` | A path that exists when the tool is on this Mac. `agent-screensaver install` skips an agent whose `detect` path is missing. |
| `hooks.file` | The tool's hook config file. It starts with `~/` and ends in `.json`. |
| `hooks.format` | `json`. `toml` is refused in this version. |
| `hooks.shape` | `grouped`: `hooks.<Event>` is a list of groups, each with its own `hooks` list (Claude Code, Codex). `flat`: `hooks.<event>` is a list of handlers. |
| `hooks.events` | The events that get our handler. Names use letters, digits and `. _ : -`. |
| `hooks.group` | `grouped` only. Extra keys of our group, for example `{ "matcher": "" }`. |
| `hooks.handler` | Extra keys of our handler, for example `{ "timeout": 5 }`. |
| `hooks.new_file` | Top-level keys written only when we create the file, for example `{ "version": 1 }`. |
| `statusline` | `{ "key": "statusLine" }`: our status line command replaces `<key>.command` in the hook file. The user's command is saved and still runs. Needs `agent.statusline`. |
| `files` | Files copied from the folder to the home folder, for tools whose hooks are plugin files: `[{ "from": "plugin/x.js", "to": "~/.config/tool/plugins/x.js" }]`. |
| `notes` | Short lines shown after install. |

The command is never in the manifest. The service always writes `'<bin>' hook --tool <source>` (plus `--event <name>` when needed, and `--reply <reply>` when the event's reply is not `none`) and `'<bin>' statusline --tool <source>`. When the manifest cannot be read, our hook prints the reply named by `--reply`, so the tool still gets a valid answer. Uninstall removes only commands of exactly these forms, whatever the record says. Our handler is `{ "type": "command", "command": <ours>, <handler keys> }`.

Rules the service keeps for every agent:

- Install only adds. It never changes an entry it did not write. Running it twice adds nothing.
- Every file is read and edited in memory first. If one file is invalid, nothing is written.
- Key order stays, and values we do not touch keep their content. The file is written with two-space indentation.
- Before the first write to a file of the user's, the service writes `<file>.agent-screensaver.bak`, once.
- The install record (`install-record.json`, version 2) lists per agent: the file, the shape, the events, each exact command, whether we created the file and its `new_file` keys, the status line key, command and the saved user command, and each copied file with its SHA-256.
- Uninstall uses only the record, never the manifest. It removes exactly the recorded commands from the recorded events. A group, an event or the `hooks` key goes only when our removal emptied it. A file we created goes when only its `new_file` keys are left. A copied file goes only while it still has the recorded SHA-256.
- Removing an agent extension (`ext remove`) removes its hooks first. If that fails, the extension stays.

### Safety rules

The service checks every recipe when the extension is installed and before every change. Built-in agents go through the same checks.

- Every path starts with `~/`, has no `..`, and stays in the home folder after symlinks are resolved.
- No path may be in our data folder, `~/Library/LaunchAgents`, `~/Library/Keychains`, `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.config/gh`, `~/.gitconfig`, or a shell start file (`~/.zshrc`, `~/.zprofile`, `~/.zshenv`, `~/.zlogin`, `~/.zlogout`, `~/.bashrc`, `~/.bash_profile`, `~/.bash_login`, `~/.bash_logout`, `~/.profile`, `~/.config/fish/`). Paths are compared without regard to case, as the Mac's file system does.
- Group and handler keys may only be `matcher` (a string) and `timeout` (a number from 1 to 30). `command`, `type` and keys such as `failClosed` are refused, so our hook can never block the tool.
- A copied file may replace only our own earlier copy.

Before any change, the service shows the plan: each file, each event, each command, each copied file, and whether the agent is built in or "from <id> <version>". Lines about an outside agent's own program or plugin file are marked as warnings. The CLI asks `Proceed? [y/N]`.

## Details

`agent.details` says where the service reads extra details for a card: the conversation name, the context use and the model. The hook never reads them. The service reads them after the event kinds in `read_on` (default `started`, `finished`, `ended`), at most once per card every 5 seconds, away from the service's main loop. A value that came in an event (for example the Claude status line) wins over a value read here. The git branch is read for every card from `.git/HEAD` and is not declared.

```json
"details": {
  "read_on": ["started", "tool_intent", "resumed", "finished", "ended"],
  "name": [
    { "jsonl": "~/.codex/session_index.jsonl",
      "last": { "path": "id", "equals": "{session}" },
      "value": { "path": "thread_name", "as": "short_title" } }
  ],
  "context_percent": [
    { "jsonl": "{transcript}",
      "last": [{ "path": "payload.type", "equals": "token_count" }, { "path": "payload.info", "present": true }],
      "value": { "ratio": ["payload.info.last_token_usage.total_tokens", "payload.info.model_context_window"] } }
  ]
}
```

`name`, `context_percent` and `model` are each a list of sources. The first source that gives a value wins. A source is one of:

| Source | Meaning |
|---|---|
| `jsonl` | A file of JSON lines. The service reads its last `tail_kb` KiB (default 256, at most 1024), drops the first, cut line, and takes the last line that matches `last`. That line decides, even when it has no value. |
| `json` | A JSON file of at most 1 MiB, read whole. `last`, when set, must hold for it. |
| `converter` | `{ "run": "bin/details", "timeout_ms": 1000 }` (at most 2000). Standard input `{"session", "cwd", "transcript"}`; standard output one object with any of `name`, `context_percent`, `model`. It runs once per read, however many fields name it. |
| `provider` | A reader built into the service. The only id is `claude-transcript-context`: Claude's context use from its transcript and the window sizes learned from the status line. |

`last` is a condition like a rule's `if`. `value` is a value like in a rule, plus `{ "ratio": [a, b] }`: a divided by b, as a whole percent from 0 to 100. Paths may start with `~/`, `{transcript}` or `{cwd}` (from the card's last event). In conditions, `{session}` is the card's session. Every file must be a regular file in the home folder after symlinks are resolved. The service only reads.

## The mark

The card shows the agent's mark and name. The mark is a small SVG file (not a symlink) of at most 8 KiB with an `<svg>` root. It may not hold, also after character references such as `&#40;` are decoded, `<script>`, `<foreignObject>`, `on...=` attributes, an `href` that does not start with `#`, a `url(` that does not start with `url(#`, or `@import`. The runtime shows it with `<img>`, so a mark can run no code and load nothing.

The project ships no company logos. The built-in marks are plain letter tiles of our own: "C" for Claude Code, "X" for Codex and "Cu" for Cursor.

You can use your own icon for any agent without editing its extension: put an SVG file named `<source>.svg` in the `marks` folder of the data folder, for example `~/Library/Application Support/agent-screensaver/marks/codex.svg`. It must pass the same checks. A file that fails them is ignored and the extension's mark is shown.

A card whose source has no agent extension (any program that writes events) shows a neutral dot and the source name.

## Known limits

- Paths in `details` are fixed. A tool that moves its folder with an environment variable is not followed: Codex's `$CODEX_HOME` is not read, so `builtin.codex` reads `~/.codex/session_index.jsonl`.
- A new agent's `limits` provider is used after the service starts again.
- macOS checks a new program on its first start, which can take up to a second. A converter can time out on its first run.
- Cursor also runs Claude Code hooks from `~/.claude/settings.json`, with its own payloads (Cursor event names and fields such as `cursor_version`). `builtin.claude` ignores every payload that has `cursor_version`, so a Cursor chat gives one Cursor card, not a Claude card too.
- Cursor never shows an approval card. Cursor sends `beforeShellExecution` and `beforeMCPExecution` before every shell command and MCP call, also when it will not ask (in its run mode "Run Everything" it never asks), and the payload does not say whether it will ask. So `builtin.cursor` maps them to `tool_intent`: the working card shows the command or the MCP tool name. Our hook prints nothing to Cursor, so Cursor decides as it would without us. (An earlier `{"permission": "ask"}` made Cursor 3.27.12 wait for an approval in "Run Everything" mode.) The live tests (24/10/2026) were done only in "Run Everything" mode; a run mode where Cursor asks was not tested.
- No usage ring for Cursor. Cursor keeps the percent used, the limit and the reset date on its server only. Reading them would need its login token and undocumented endpoints, so there is no `limits` provider for Cursor.
- Cursor sends `transcript_path` only on `stop`, and the full model name (with the effort, for example `grok-4.7-high`) only on `beforeSubmitPrompt` and `stop`, so `builtin.cursor` sets the model only there. Cursor has no declared details: its chat name is in an undocumented database.

## Turning an agent off

`agents_off` in the settings file lists agent ids that are off. An agent that is off keeps its hooks, but its hook only prints its reply and exits. The service drops its events, and does not read its limits, estimate its usage or read its transcripts.

## An untested example

The samples in [guides/examples](guides/examples/) are tested: `acme.quill` (a mapping and a hook file) and `acme.tern` (a converter and a plugin file). The opencode-shaped example below is not.

This opencode-shaped example shows a plugin file, the event name from `--event`, and a converter-free mapping. It has not been tested with opencode. The event names come from https://opencode.ai/docs/plugins/.

```json
{
  "id": "acme.opencode",
  "name": "opencode",
  "version": "0.1.0",
  "type": "agent",
  "api": 1,
  "agent": {
    "source": "opencode",
    "mark": "mark.svg",
    "hooks": {
      "event": { "arg": "event" },
      "session": "sessionID",
      "common": {
        "project": { "path": "cwd", "as": "basename" },
        "cwd": "cwd"
      },
      "rules": [
        { "on": "session.status", "if": { "path": "status", "equals": "busy" }, "kind": "started" },
        { "on": "tool.execute.before", "kind": "tool_intent",
          "set": { "title": { "path": "tool", "as": "short_title" } } },
        { "on": "permission.asked", "kind": "approval",
          "set": { "title": { "path": "title", "as": "short_title" } } },
        { "on": ["permission.replied", "tool.execute.after"], "kind": "resumed" },
        { "on": "session.idle", "kind": "finished" }
      ]
    },
    "install": {
      "detect": "~/.config/opencode",
      "files": [
        { "from": "plugin/agent-screensaver.js", "to": "~/.config/opencode/plugins/agent-screensaver.js" }
      ],
      "notes": ["Restart opencode to load the agent-screensaver plugin."]
    }
  }
}
```

The plugin file would run `agent-screensaver hook --tool opencode --event <name>` with a JSON payload on standard input for each of these events.
