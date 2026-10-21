# Event format

This page describes the event that tells agent-screensaver what is happening. The Claude Code and Codex hooks use it. The status line uses it. Your own program can use it too.

Any program can be a source. To be one, write one JSON file with one event into the spool folder. The spool folder is `$HOME/Library/Application Support/agent-screensaver/spool/`. The background service reads the file. You do not need to link to any code of ours. You can write the file in any language.

## Shape

```json
{
  "v": 1,
  "source": "claude",
  "session": "s1",
  "kind": "started",
  "at_ms": 1760000000000,
  "project": "my-app",
  "title": "Fix the login bug"
}
```

| Field | Type | Required | Meaning |
|---|---|---|---|
| `v` | number | yes | Format version. It must be `1`. |
| `source` | string | yes | Who sends the event, for example `claude` or `codex`. 1 to 128 characters. |
| `session` | string | yes | The session id from your tool. 1 to 128 characters. |
| `kind` | string | yes | What happened. See the list below. |
| `at_ms` | number | yes | When it happened. Unix time in milliseconds. |
| `project` | string | no | The project name shown on the card. |
| `title` | string | no | The text shown on the card. |
| `limits` | list | no | Usage limit readings. Used with the `limits` kind. |

Two sessions with the same `source` and different `session` values are two cards.

## Kinds

| Kind | Meaning | Claude Code hook | Codex hook |
|---|---|---|---|
| `started` | A task started. | `UserPromptSubmit` | `UserPromptSubmit` |
| `tool_intent` | A tool is about to run. It tells the service what a later approval is for. | `PreToolUse` | `PreToolUse` |
| `approval` | The agent waits for the user to approve something. | `Notification` (permission prompts only, idle notifications are ignored) and `PermissionRequest` | `PermissionRequest` |
| `resumed` | The agent works again. The approval was answered. | `PostToolUse` | `PostToolUse` |
| `finished` | The task finished. | `Stop` | `Stop` |
| `ended` | The session closed. | `SessionEnd` | `SessionEnd` |
| `limits` | New usage limit numbers. It is not about one task. | status line | `codex app-server` |

## Limits

A `limits` event carries a list of readings:

```json
{
  "v": 1,
  "source": "claude",
  "session": "s1",
  "kind": "limits",
  "at_ms": 1760000000000,
  "limits": [
    { "window": "5h", "used_percent": 62.0, "resets_at": 1800000000 },
    { "window": "week", "used_percent": 31.5 }
  ]
}
```

| Field | Type | Required | Meaning |
|---|---|---|---|
| `window` | string | yes | `5h` for the five-hour limit. `week` for the weekly limit. |
| `used_percent` | number | yes | How much of the limit is used. |
| `resets_at` | number | no | When the limit resets. Unix time in seconds. Leave it out if you do not know it. |

## Rules for writers

- Write exactly one event in each file.
- Never write straight to the final name. First write `<at_ms>-<pid>-<nanos>.json.tmp`, then rename it to `.json`. The rename means the service never sees a half-written file.
- Keep each file under 64 KiB (65536 bytes). A bigger file is rejected.
- Use `"v": 1`. Any other version is rejected.
- Use one of the kinds above. Any other kind is rejected.
- Do not leave `source` or `session` empty. Keep each under 128 characters.
- Leave out optional fields you do not have. Do not send empty strings or made-up values.
