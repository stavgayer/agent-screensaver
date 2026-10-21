package agents

import "agent-screensaver/internal/event"

// Field names (hook_event_name, session_id, thread_id, cwd, prompt,
// tool_name, tool_input) come from https://learn.chatgpt.com/docs/hooks and
// are re-checked in Task 6.1.

// Codex maps Codex hook input.
type Codex struct{}

func (Codex) Name() string { return "codex" }

func (adapter Codex) Map(input []byte, nowMs int64) (event.Event, bool) {
	hook, ok := hookFields(input)
	if !ok {
		return event.Event{}, false
	}
	return mapCommon(adapter.Name(), hook, nowMs)
}
