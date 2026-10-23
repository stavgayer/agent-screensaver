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
| `background` | number | no | How many background jobs the session still runs, such as shells or subagents. A whole number, 0 or more. |
| `name` | string | no | The conversation name your tool shows, for example one the user set. It is shown instead of the title. |
| `model` | string | no | A short model name, for example `Opus 5.5` or `gpt-5.6`. |
| `context_percent` | number | no | How full the context window is, from 0 to 100. |
| `cwd` | string | no | The session's folder. The service reads the git branch there. It is never shown. |
| `transcript` | string | no | The session's log file. The service reads the conversation name or the context fullness there (see below). It is never shown. |

## Card titles from prompts

The Claude Code and Codex hooks make the card title from the prompt the user typed. Tools often add text to a prompt that the user did not write. The hooks skip that text:

- Tag blocks at the start, such as `<system-reminder>...</system-reminder>`, `<task-notification>`, `<agent-message ...>` or `<command-name>`. A lone tag on its own line is skipped too.
- Banner lines made of dashes, such as `--- MODE SWITCH ---`.

The title is the first human text that remains. If nothing human remains, the event has no title, and the card keeps the title it already had. A prompt that is fully wrapped in one tag, such as `<command-name>/clear</command-name>`, therefore gives no title. Tags that come after human text are not skipped, so `fix <Button> please` keeps its tag in the title. A file mention such as `@"/Users/me/photo.png"` shows only the file name (`photo.png`). Secrets are masked first, then the title is cut to 40 characters.

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
| `info` | New details of a session: `name`, `model`, `context_percent`. It never makes a card and never changes the card's state or time. | status line | not used |

A field that an event leaves out keeps the value the card has.

## Card details

A card can show a conversation name, the model, how full the context window is and the git branch. They come from these places, first match wins:

| Detail | Claude Code | Codex |
|---|---|---|
| Name | 1. `session_name` in the status line input (set with `/rename`). 2. The last `custom-title` entry in the transcript, else the last `ai-title` entry, of the same session. 3. `session_title` in a hook payload. | The last `thread_name` of the thread in `~/.codex/session_index.jsonl` (`$CODEX_HOME` when set). Codex writes it about 5 seconds into the first turn. |
| Model | `model.display_name` in the status line input, made short (`Opus 5.5 (1M context)` becomes `Opus 5.5`); `message.model` of the last answer in the transcript (the desktop app runs no status line); `model` in a hook payload. | `model` in the hook payload. |
| Context | `context_window.used_percentage` in the status line input. The transcript does not have the window size. | The last `token_count` event in the rollout file: `last_token_usage.total_tokens` divided by `model_context_window`. The running total (`total_token_usage`) counts every call of the thread, so it is not used. No window size means no bar. |
| Branch | `.git/HEAD` in the session's folder or a folder above it (a worktree's `.git` file is followed). A detached head shows the short commit. | Same. |

When no name is known, the card shows the title made from the prompt, then the project.

The hooks only copy the paths: they must exit fast. The service reads the files, and only the last 256 KB of a log. It reads a session at the start and at the end of each turn, and at most every 30 seconds in between. The Claude Code status line runs on every turn of a terminal session, so it keeps the name, model and context fresh there; Claude Code's desktop app does not run it.

In privacy mode the name and the branch are left out of `state.json`, like the title.

## Background jobs

An agent can leave jobs running after it answers, for example a build in a background shell or a subagent. The `background` field says how many still run.

- Leave the field out when you do not know. The card then keeps the count it has.
- Send `0` when the last job is done.
- The newest event wins. An event older than the card's last event does not change the count.
- A done card with jobs left shows "Done" and the count. It does not go away after 30 minutes while the count is above 0. It still goes after 12 hours with no event.
- An `ended` event sets the count to 0: the jobs end with their session.

The Claude Code hook sends the length of the `background_tasks` list in the `Stop` payload. That list holds the jobs still in flight. When a job ends, Claude Code wakes the session with a `<task-notification>` prompt, and that turn ends with a new `Stop`, so the count is refreshed then. Other Claude Code events carry no list and leave the count as it is. The Codex hooks send no count.

The `SubagentStart` and `SubagentStop` hooks are not used. `SubagentStart` carries no list. `SubagentStop` fires for subagents in the foreground too, while the main task still works, and the `Stop` that follows has the new list anyway.

## Ignored projects

The service drops every event whose `project` is in the `ignored_projects` list of the settings (General, Ignored projects, in the settings window). Such an event never makes or changes a card, never asks for approval and never plays the sound. An entry matches when it equals the project exactly, case included; an entry written as a path matches by its last folder name. The default list is `["observer-sessions"]`, the folder where the claude-mem plugin runs hidden Claude sessions. A settings file without the field gets the default. Events without a project are never dropped.

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
| `window` | string | yes | `5h` for the five-hour limit. `week` for the weekly limit. `plan` for a usage-based plan limit with no fixed period. |
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
