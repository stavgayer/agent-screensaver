package agents

import (
	"path/filepath"

	"agent-screensaver/internal/jsonedit"
)

// codexEvents are the hook events we subscribe to. A group without a matcher
// matches every occurrence (https://learn.chatgpt.com/docs/hooks).
var codexEvents = []string{
	hookUserPromptSubmit, hookPreToolUse, hookPermission,
	hookPostToolUse, hookStop, hookSessionEnd,
}

func (Codex) ConfigPath(home string) string {
	return filepath.Join(home, ".codex", "hooks.json")
}

func (Codex) AddHooks(cfg *jsonedit.Object, command string) error {
	targets := make([]hookTarget, len(codexEvents))
	for position, event := range codexEvents {
		targets[position] = hookTarget{event: event}
	}
	return addHookEntries(cfg, targets, command)
}

func (Codex) RemoveHooks(cfg *jsonedit.Object, command string) error {
	return removeHookEntries(cfg, command)
}
