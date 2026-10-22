package agents

import "agent-screensaver/internal/event"

// Field names (hook_event_name, session_id, thread_id, cwd, prompt,
// tool_name, tool_input) come from https://learn.chatgpt.com/docs/hooks and
// were checked against real payloads from codex-cli 0.162.0 in Task 6.1.

// Codex maps Codex hook input.
type Codex struct{}

func (Codex) Name() string { return "codex" }

func (adapter Codex) Map(input []byte, nowMs int64) (event.Event, bool) {
	hook, ok := hookFields(input)
	if !ok {
		return event.Event{}, false
	}
	// Codex runs hidden helper sessions, for example to write the task title.
	// They have their own session id and "transcript_path": null, and would
	// show up as a card with the helper prompt as its title.
	if hook.noTranscript {
		return event.Event{}, false
	}
	return mapCommon(adapter.Name(), hook, nowMs)
}
