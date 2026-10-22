package agents

import (
	"strings"

	"agent-screensaver/internal/event"
)

// Field names (hook_event_name, session_id, cwd, prompt, tool_name,
// tool_input, notification_type, message) come from
// https://code.claude.com/docs/en/hooks and were checked against real payloads from Claude Code 2.1.308 in Task 6.1
// (no Notification payload was recorded; its fixtures come from the docs).

// Claude maps Claude Code hook input.
type Claude struct{}

func (Claude) Name() string { return "claude" }

func (adapter Claude) Map(input []byte, nowMs int64) (event.Event, bool) {
	hook, ok := hookFields(input)
	if !ok {
		return event.Event{}, false
	}
	if hook.hookEvent == hookNotification {
		return adapter.mapNotification(hook, nowMs)
	}
	return mapCommon(adapter.Name(), hook, nowMs)
}

// mapNotification keeps permission prompts only. Idle notifications are not
// approvals and would show a false "waiting" card.
func (adapter Claude) mapNotification(hook fields, nowMs int64) (event.Event, bool) {
	if !isPermissionNotification(hook) {
		return event.Event{}, false
	}
	return newEvent(adapter.Name(), hook, event.KindApproval, "", nowMs), true
}

func isPermissionNotification(hook fields) bool {
	if hook.notificationType != "" {
		return hook.notificationType == "permission_prompt"
	}
	return strings.Contains(strings.ToLower(hook.message), "permission")
}
