# Add an agent

Goal: show the sessions of a new coding agent (for example opencode or pi) as cards on the screen.

You write one folder with a manifest. You do not change our code. The full reference is [agents.md](../agents.md). This page is the short path.

Two samples are in [examples/](examples/). Both tools are made up, so the hook payloads are ours, not from a real tool:

- `examples/acme.quill`: JSON payloads, a mapping, a hook file, a reply, card details.
- `examples/acme.tern`: plain-text payloads, a converter program, a plugin file.

Before you start:

- You can run `agent-screensaver` from the unzipped folder (`./agent-screensaver`, see the README).
- You ran `agent-screensaver install` once. Adding hooks needs its install record.
- You know how your tool runs a command on an event (a hook or a plugin).

## 1. Record what the tool sends

You need real payloads. Install a recorder first. It maps nothing, so it makes no cards.

1. Make a folder `acme.recorder` with an `extension.json` and a `mark.svg` (step 3 shows a mark). Use this body:

```json
"agent": {
  "source": "recorder",
  "mark": "mark.svg",
  "hooks": { "event": { "arg": "event" }, "session": "session_id", "rules": [{ "kind": "ignore" }] },
  "install": { "hooks": { "file": "~/.recorder/hooks.json", "format": "json", "shape": "flat", "events": ["prompt", "stop"] } }
}
```

2. Change the file, the shape and the events to what your tool uses (step 4).
3. Install it and add its hooks (step 5).
4. Start your tool from a shell with `AGS_RECORD_PAYLOADS=1`, and use it.
5. Read `~/Library/Application Support/agent-screensaver/payloads/`. Each file is one raw payload.
6. Remove the recorder when you are done: `agent-screensaver ext remove acme.recorder`.

A file name is `<source>-<event>-<time>.json`. The event is the `--event` value, or the `hook_event_name` of the payload. If the payload has neither, the name says `unknown`.

Payloads hold your prompts and file paths. Keep them private. Delete the folder after you copy what you need.

## 2. Make the folder

```
acme.quill/
  extension.json
  mark.svg
```

The folder name must be the `id`. The manifest has the common fields of [extensions.md](../extensions.md) and `"type": "agent"`, plus one `agent` object. An agent has no `entry` and no `options`.

The `source` is the name written on every event: 1 to 32 characters of lowercase letters, digits and dashes. Two installed agents cannot share a source. Unknown fields are refused, so a typo is an error.

## 3. Add a mark

The mark is a small SVG file. The card shows it next to the agent name.

```
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect x="1" y="1" width="22" height="22" rx="6" fill="#7a5cff"/><text x="12" y="17" text-anchor="middle" font-family="-apple-system, Helvetica, Arial, sans-serif" font-size="15" font-weight="700" fill="#ffffff">Q</text></svg>
```

Rules: at most 8 KiB, an `<svg>` root, no symlink. No `<script>`, no `<foreignObject>`, no `on...=` attribute, no link that is not `#...`, no `@import`. The project ships no company logos. Make a letter tile like this one, or use your own icon.

A user can replace any agent's mark. They put `<source>.svg` in the `marks` folder of the data folder. It must pass the same checks. If it fails, the extension's mark shows.

## 4. Map the payload to events

`agent.hooks` is data. It names where the event name and the session id are, and which rule gives which kind of event.

```json
"hooks": {
  "event": "event",
  "session": "session_id",
  "common": { "project": { "path": "cwd", "as": "basename" }, "cwd": "cwd" },
  "rules": [
    { "on": "prompt", "kind": "started", "set": { "title": { "path": "text", "as": "prompt_title" } } },
    { "on": "permission", "kind": "approval", "set": { "title": { "tool": "tool", "input": "input" } } },
    { "on": "stop", "kind": "finished" },
    { "on": "debug", "kind": "ignore" }
  ]
}
```

- `event` is the path of the event name in the payload. If the tool does not send it, write `{ "arg": "event" }`. Every hook command then gets `--event <name>`.
- `session` is the session id. A payload without one is ignored. Two sessions are two cards.
- The first rule that matches wins. A payload that matches no rule is ignored.
- `kind` is one of `started`, `tool_intent`, `approval`, `resumed`, `finished`, `ended`, `limits`, `info`, or `ignore`.

What each kind does on the screen:

| Kind | Use it when | Card |
|---|---|---|
| `started` | The user sent a task. | Working. Its `title` is the card title. |
| `tool_intent` | A tool is about to run. | Its `title` becomes the card's request text. The state does not change. |
| `approval` | The agent waits for the user. | Needs approval. The screen switches to the attention layout. |
| `resumed` | The agent works again. | Working. |
| `finished` | The task is done. | Done. |
| `ended` | The session closed. | The card goes away. A Done card stays until it expires. |
| `info` | New name, model or context use. | Details only. It makes no card. |

The reference lists every field, transform and condition: [agents.md](../agents.md), "The mapping language". Use only those names.

## 5. Add a reply, if the tool needs one

Some tools read what our hook prints. `reply` says what it prints:

```json
"reply": { "events": { "before_tool": "empty_object" } }
```

The only replies are `none` (default), `empty_object` (`{}`) and `ask` (`{"permission":"ask"}`, which lets the tool ask the user as it would without us). A manifest can never make our hook allow or deny an action. Any other value is refused when you install.

## 6. Write the install recipe

`agent.install` says how to add and remove our hooks. You write data. The service edits the files.

```json
"install": {
  "detect": "~/.quill",
  "hooks": {
    "file": "~/.quill/hooks.json",
    "format": "json",
    "shape": "flat",
    "new_file": { "version": 1 },
    "handler": { "timeout": 5 },
    "events": ["prompt", "before_tool", "permission", "after_tool", "stop", "exit"]
  },
  "notes": ["Restart Quill to load the new hooks."]
}
```

- `detect`: a path that exists when the tool is on this Mac.
- `hooks.file`: starts with `~/` and ends in `.json`. `hooks.format` is `json`. `toml` is refused.
- `hooks.shape`: `flat` (`hooks.<event>` is a list of handlers) or `grouped` (a list of groups, each with a `hooks` list, as Claude Code and Codex do).
- `hooks.new_file`: keys written only when we create the file.
- `hooks.handler` and `hooks.group`: only `timeout` (1 to 30) and `matcher` (a string) are allowed.
- `files`: for a tool whose hooks are plugin files, `[{ "from": "plugin/x.sh", "to": "~/.config/tool/plugins/x.sh" }]`. The `acme.tern` sample does this.
- `notes`: lines shown after the install.

You never write the command. The service writes `'<bin>' hook --tool <source>`, and adds `--event <name>` and `--reply <reply>` where the manifest needs them.

## 7. Add card details (optional)

`agent.details` says where the service reads a conversation name, the context use or the model. The hook never reads files.

```json
"details": {
  "name": [
    { "jsonl": "~/.quill/sessions.jsonl",
      "last": { "path": "id", "equals": "{session}" },
      "value": { "path": "title", "as": "short_title" } }
  ]
}
```

The service reads the last lines of the file, takes the last line where `last` holds, and reads `value` from it. The sources are `jsonl`, `json`, `converter` and `provider`. See [agents.md](../agents.md), "Details". The path must start with `~/`, `{transcript}` or `{cwd}`.

## 8. Install and connect

1. Install the folder:

```
agent-screensaver ext install ./acme.quill
```

This checks the manifest and the recipe, then copies the folder. It adds no hooks.

2. Add the hooks. The command shows a plan first and asks `Proceed? [y/N]`:

```
agent-screensaver agent hooks add acme.quill
```

The plan lists each file, each event and each command. The settings window shows the same plan (Connections, then "Add hooks"). "Add another agent" in that tab installs a folder.

3. Check the state:

```
agent-screensaver agent list
```

A line shows `Hooks not added`, `Idle · no events yet` (hooks added, nothing seen yet), `Active · 4m ago`, `Not installed` (the `detect` path is missing) or `Off`.

4. Use your tool. A card appears on the screen. The same list is the Connections tab of the settings window.

To go quiet without removing hooks: `agent-screensaver agent off acme.quill` (and `on`). To remove the hooks: `agent-screensaver agent hooks remove acme.quill`. `agent-screensaver ext remove acme.quill` removes the hooks first, then the extension. If the hooks cannot be removed, the extension stays.

## 9. Test with recorded payloads

You do not need the real tool to test a mapping. Pipe a recorded payload into the hook:

```
agent-screensaver hook --tool quill < payloads/prompt.json
```

The hook always exits 0 and prints only its reply. Look at the card on the screen, or at `agent-screensaver agent list`. Errors go to `hook-errors.log` in the data folder.

The samples keep their payloads in [examples/payloads/](examples/payloads/). `go test ./internal/settings -run Guide` maps each one and checks the event kind and title. Copy that table for your agent.

If a card does not appear, check in this order:

1. `hook-errors.log` says `unknown tool`: the source in the command is not installed.
2. The payload has no session id. It is ignored.
3. No rule matches the event name. It is ignored.
4. The project is in the ignored projects list (settings, General).
5. The agent is off (`agent list` says `Off`).

## 10. A converter, when the mapping is not enough

Use a converter when the payload is not JSON, or the mapping cannot describe it. `acme.tern` sends plain text:

```json
"hooks": {
  "event": { "arg": "event" },
  "converter": { "run": "bin/convert", "timeout_ms": 500 }
}
```

The contract:

- `run` is a regular file (not a symlink) with the execute bit, inside the folder. It runs directly, with no shell and no arguments.
- Standard input: the raw payload, at most 1 MiB.
- Environment: only `HOME`, `PATH`, `AGS_SOURCE` (the source) and `AGS_EVENT` (the `--event` value, or empty).
- Standard output: zero to 4 events, one JSON object per line, in the [event format](../event-format.md), at most 64 KiB. Empty output means ignore.
- Our hook fills `v` and `at_ms` when missing, forces `source`, masks and cuts the text fields, and drops an invalid event.
- `timeout_ms` is 200 by default and at most 500. On a timeout nothing is sent. The first start of a new program can be slow on macOS, so the first event after an install can time out. Send it again.
- A converter replaces `session`, `common` and `rules`. Use one or the other.

A converter is code you ask the user to trust. The plan shows a warning line for it, and the settings window asks the user to confirm with "I trust <name>, add hooks". Prefer a mapping when it can do the job.

## Why an install is refused

`ext install` refuses the folder, and leaves nothing behind, when:

| Cause | Message starts with |
|---|---|
| A hook file under `~/.ssh`, `~/.aws`, `~/.gnupg`, `~/.config/gh`, `~/Library/LaunchAgents`, `~/Library/Keychains`, or a shell start file such as `~/.zshrc` | `hooks: path "..." is a protected place` |
| A path with `..` or outside the home folder | `hooks: path ... ..` |
| A handler key other than `timeout` or `matcher` (for example `failClosed`) | `hooks: handler: key "failClosed" is not allowed` |
| A source another agent already uses | `agent source "quill" is already used by ...` |
| A mark with a script, a link or a `url(` | `mark holds a <script> element` |
| An unknown field | `agent.hooks: json: unknown field "rulez"` |
| A reply that is not `none`, `empty_object` or `ask` | `reply "allow" is not one of ...` |
| A hook file format that is not `json` | `toml is not supported yet` |

The rules exist so that an agent extension can never block your tool, edit your shell, or read your keys. They are the same for the built-in agents.

## Before you share it

- Run `agent-screensaver ext install` on the final folder, then `agent hooks add`, then use the real tool once.
- Test every event name your tool can send.
- Say in your README which tool versions you tested.
- See [test-and-share.md](test-and-share.md) to package a zip.
