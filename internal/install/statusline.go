package install

import (
	"encoding/json"
	"fmt"
	"slices"

	"agent-screensaver/internal/jsonedit"
)

// swapStatusline points the status line key at ours and returns the user's
// command it replaced. It returns "" when there was none, or when the
// command was ours already (ours or one of earlier, the commands an older
// install wrote), so the caller keeps the command it saved before. Other
// keys of the user's object (for example padding) stay.
func swapStatusline(settings *jsonedit.Object, key, ours string, earlier []string) (previous string, err error) {
	line, err := objectOf(settings, key)
	if err != nil {
		return "", err
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
	if slices.Contains(earlier, current) {
		current = ""
	}
	return current, setObject(settings, key, line)
}

// restoreStatusline undoes swapStatusline, but only while the command is
// still ours: if the user changed it since, their choice wins. It removes
// only the keys install set: the command (put back to previous when there
// was one) and, when there was no previous command, the type. The object
// goes only when that leaves it empty, so keys the user added survive.
func restoreStatusline(settings *jsonedit.Object, key, previous, ours string) error {
	if _, ok := settings.Get(key); !ok {
		return nil
	}
	line, err := objectOf(settings, key)
	if err != nil {
		return err
	}
	if current, _ := stringValue(line, "command"); current != ours {
		return nil
	}
	if previous != "" {
		if err := setString(line, "command", previous); err != nil {
			return err
		}
		return setObject(settings, key, line)
	}
	line.Delete("command")
	if kind, _ := stringValue(line, "type"); kind == "command" {
		line.Delete("type")
	}
	if len(line.Keys()) == 0 {
		settings.Delete(key)
		return nil
	}
	return setObject(settings, key, line)
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
		return fmt.Errorf("encode %s: %w", key, err)
	}
	return object.Set(key, raw)
}
