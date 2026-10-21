package agents

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"agent-screensaver/internal/jsonedit"
)

const statuslineKey = "statusLine"

// claudeEvents are the hook events we subscribe to. Claude Code accepts an
// empty matcher as "match everything".
var claudeEvents = []string{
	hookUserPromptSubmit, hookPreToolUse, hookNotification, hookPermission,
	hookPostToolUse, hookStop, hookSessionEnd,
}

func (Claude) ConfigPath(home string) string {
	return filepath.Join(home, ".claude", "settings.json")
}

func (Claude) AddHooks(cfg *jsonedit.Object, command string) error {
	empty := ""
	targets := make([]hookTarget, len(claudeEvents))
	for position, event := range claudeEvents {
		targets[position] = hookTarget{event: event, matcher: &empty}
	}
	return addHookEntries(cfg, targets, command)
}

func (Claude) RemoveHooks(cfg *jsonedit.Object, command string) error {
	return removeHookEntries(cfg, command)
}

// SwapStatusline points the status line at ours and returns the command it
// replaced. It returns "" when there was none or when ours is already set, so
// the caller must keep an earlier saved value. Other keys of the user's
// statusLine object (for example padding) stay.
func SwapStatusline(settings *jsonedit.Object, ours string) (previous string, err error) {
	line := &jsonedit.Object{}
	if raw, ok := settings.Get(statuslineKey); ok {
		if line, err = jsonedit.Parse(raw); err != nil {
			return "", fmt.Errorf("%s is not an object: %w", statuslineKey, err)
		}
	}
	current, _ := stringValue(line, "command")
	if current == ours {
		return "", nil
	}
	if err := setString(line, "command", ours); err != nil {
		return "", err
	}
	if _, hasType := line.Get("type"); !hasType {
		if err := setString(line, "type", "command"); err != nil {
			return "", err
		}
	}
	return current, setObject(settings, statuslineKey, line)
}

// RestoreStatusline undoes SwapStatusline, but only while the command is still
// ours: if the user changed it since, their choice wins. It removes only the
// keys install set: the command (put back to previous when there was one) and,
// when there was no previous command, the type. The object goes only when that
// leaves it empty, so keys the user added, such as padding, survive.
func RestoreStatusline(settings *jsonedit.Object, previous, ours string) error {
	raw, ok := settings.Get(statuslineKey)
	if !ok {
		return nil
	}
	line, err := jsonedit.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not an object: %w", statuslineKey, err)
	}
	if current, _ := stringValue(line, "command"); current != ours {
		return nil
	}
	if previous != "" {
		if err := setString(line, "command", previous); err != nil {
			return err
		}
		return setObject(settings, statuslineKey, line)
	}
	line.Delete("command")
	if kind, _ := stringValue(line, "type"); kind == "command" {
		line.Delete("type")
	}
	if len(line.Keys()) == 0 {
		settings.Delete(statuslineKey)
		return nil
	}
	return setObject(settings, statuslineKey, line)
}

func stringValue(object *jsonedit.Object, key string) (string, bool) {
	raw, ok := object.Get(key)
	if !ok {
		return "", false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func setString(object *jsonedit.Object, key, value string) error {
	raw, err := marshalPlain(value)
	if err != nil {
		return err
	}
	return object.Set(key, raw)
}
