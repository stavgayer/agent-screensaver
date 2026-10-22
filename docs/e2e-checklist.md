# End-to-end checklist

Run this on a real Mac after installing the release zip. Tick each box only after you see the result yourself.

- [ ] Claude task running: the bottom card shows the start of the prompt and a growing time.
- [ ] Claude asks permission while the saver shows: orange card in the center, clock moves top-left, one sound.
- [ ] Claude idle ("waiting for your input"): no orange card.
- [ ] Codex `PermissionRequest`: orange card. Deny the command: the card becomes "check the terminal" after 10 minutes.
- [ ] Finished task: blue card, gone after 30 minutes.
- [ ] Claude Max and Codex rings match `/usage` (Claude) and `/status` (Codex) within 1%.
- [ ] Mac locks: the password is asked when you come back.
- [ ] After the saver stops: no `WebContent` process of ours and no `legacyScreenSaver` left after 10 seconds.
- [ ] CPU while showing Matrix stays near the spike's ~6.5% of one core.
- [ ] Second display shows only the background and the clock.
- [ ] Existing status line still prints the same as before install.
- [ ] Menu bar icon changes when an agent waits; menu lines match the screen saver.
- [ ] Settings window: a theme change shows in the preview and in the real screen saver.
- [ ] `uninstall` leaves `~/.claude/settings.json` and `~/.codex/hooks.json` as they were, and removes both LaunchAgents.

## Real payload check

Done on 22 Oct 2026 on the owner's Mac with the installed app. Versions: Claude Code 2.1.308, codex-cli 0.162.0. One payload of each event was recorded for both tools: `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, `Stop`, `SessionEnd`. The recorded files had real prompts, so they were deleted after the check. The fixtures in `internal/agents/testdata/` now have the real shape with fake ids and `/Users/example` paths.

What matched the old fixtures: field names (`session_id`, `cwd`, `hook_event_name`, `prompt`, `tool_name`, `tool_input`), nesting, and the event names. Every real payload maps to the expected event kind.

What was different:

- Codex runs hidden helper sessions (model `gpt-5.6-luna`) that write the task title. They send their own `UserPromptSubmit`, `Stop` and `SessionEnd` with `"transcript_path": null`. Before the fix each one made a card titled with the helper prompt. Fix: the Codex adapter ignores events that have `"transcript_path": null`. Claude events always have a path, so Claude is not changed. Side effect: a Codex session with no transcript file would also be hidden.
- Codex `tool_name` is `Bash` (capital B) and `tool_input.command` is a string, not a list. The title is `Bash: touch /tmp/hello3.txt`. The list form is still accepted and has its own fixture.
- Codex `PermissionRequest` has `tool_input.description` ("Allow creating ... with touch?"). The title uses `command`, so the card says `Bash: touch /tmp/hello3.txt`.
- Codex `PostToolUse` has `tool_response` as a string (`""`) for Bash and as an object for MCP tools. The mapping does not read it.
- Codex MCP tools arrive as `mcp__<server>__<tool>`, for example `mcp__engram__mem_session_summary`. The title falls back to the tool name. The secret mask used to turn a long name like that into `•••`; it now keeps lowercase snake_case names and still masks random keys (`internal/text`).
- Claude has extra fields the mapping does not read: `prompt_id`, `scratchpad_dir`, `permission_mode`, `effort`, `tool_use_id`, `duration_ms`, `permission_suggestions`, `background_tasks`, `session_crons`. `SessionEnd.reason` was `prompt_input_exit`.
- Claude `tool_response` is an object with `stdout`, `stderr`, `interrupted` and more. Not read.

Not verified:

- No Claude `Notification` payload was recorded (`idle_prompt` did not fire within about a minute). The two Notification fixtures stay as they were, written from the docs.
- No Codex `apply_patch` payload was recorded. The `apply_patch` fixture is still a guess, and the real `tool_input` may not have a `path` field.
- Claude `Edit` permission: only Bash was recorded. The Edit fixture is kept from the docs.

Codex runs hook commands through a shell. The installed command has a quoted path with a space (`.../Application Support/...`), and the payloads were recorded, so the quoting works.
