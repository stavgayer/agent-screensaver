package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"agent-screensaver/internal/jsonedit"
)

// Both tools use the same file shape: a top-level "hooks" object maps an event
// name to an array of groups, and each group holds a "hooks" array of
// handlers ({"type": "command", "command": ...}).
const hooksKey = "hooks"

// hookTarget is one event we subscribe to. matcher is written only when set,
// because Codex accepts a group without one.
type hookTarget struct {
	event   string
	matcher *string
}

type hookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookGroup struct {
	Matcher *string       `json:"matcher,omitempty"`
	Hooks   []hookHandler `json:"hooks"`
}

// addHookEntries appends one group per target, skipping an event that already
// runs command. Foreign groups are kept byte for byte.
func addHookEntries(cfg *jsonedit.Object, targets []hookTarget, command string) error {
	hooks, err := hooksObject(cfg)
	if err != nil {
		return err
	}
	for _, target := range targets {
		groups, err := eventGroups(hooks, target.event)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(groups, func(group json.RawMessage) bool { return groupRuns(group, command) }) {
			continue
		}
		entry, err := marshalPlain(hookGroup{Matcher: target.matcher, Hooks: []hookHandler{{Type: "command", Command: command}}})
		if err != nil {
			return err
		}
		if err := setGroups(hooks, target.event, append(groups, entry)); err != nil {
			return err
		}
	}
	return setObject(cfg, hooksKey, hooks)
}

// removeHookEntries removes the handlers whose command is command, from every
// event. A group, event or "hooks" key is dropped only when our removal
// emptied it; a container that was already empty stays as the user left it.
func removeHookEntries(cfg *jsonedit.Object, command string) error {
	raw, ok := cfg.Get(hooksKey)
	if !ok {
		return nil
	}
	hooks, err := jsonedit.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not an object: %w", hooksKey, err)
	}
	removedAny := false
	for _, event := range hooks.Keys() {
		groups, err := eventGroups(hooks, event)
		if err != nil {
			return err
		}
		kept, removed, err := dropHandlers(groups, command)
		if err != nil {
			return fmt.Errorf("event %s: %w", event, err)
		}
		if !removed {
			continue
		}
		removedAny = true
		if len(kept) == 0 {
			hooks.Delete(event)
		} else if err := setGroups(hooks, event, kept); err != nil {
			return err
		}
	}
	if !removedAny {
		return nil
	}
	if len(hooks.Keys()) == 0 {
		cfg.Delete(hooksKey)
		return nil
	}
	return setObject(cfg, hooksKey, hooks)
}

// dropHandlers removes command's handlers from each group and drops groups
// that end up with none. Groups it did not touch keep their original bytes.
func dropHandlers(groups []json.RawMessage, command string) (kept []json.RawMessage, removed bool, err error) {
	for _, raw := range groups {
		group, err := jsonedit.Parse(raw)
		if err != nil {
			return nil, false, fmt.Errorf("group is not an object: %w", err)
		}
		handlers, err := handlersOf(group)
		if err != nil {
			return nil, false, err
		}
		remaining := slices.DeleteFunc(slices.Clone(handlers), func(handler json.RawMessage) bool {
			return handlerCommand(handler) == command
		})
		switch {
		case len(remaining) == len(handlers):
			kept = append(kept, raw)
		case len(remaining) == 0:
			removed = true
		default:
			removed = true
			if err := group.Set("hooks", joinArray(remaining)); err != nil {
				return nil, false, err
			}
			marshaled, err := group.Marshal()
			if err != nil {
				return nil, false, err
			}
			kept = append(kept, marshaled)
		}
	}
	return kept, removed, nil
}

func hooksObject(cfg *jsonedit.Object) (*jsonedit.Object, error) {
	raw, ok := cfg.Get(hooksKey)
	if !ok {
		return &jsonedit.Object{}, nil
	}
	hooks, err := jsonedit.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not an object: %w", hooksKey, err)
	}
	return hooks, nil
}

func eventGroups(hooks *jsonedit.Object, event string) ([]json.RawMessage, error) {
	raw, ok := hooks.Get(event)
	if !ok {
		return nil, nil
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, fmt.Errorf("event %s is not an array: %w", event, err)
	}
	return groups, nil
}

func handlersOf(group *jsonedit.Object) ([]json.RawMessage, error) {
	raw, ok := group.Get("hooks")
	if !ok {
		return nil, nil
	}
	var handlers []json.RawMessage
	if err := json.Unmarshal(raw, &handlers); err != nil {
		return nil, fmt.Errorf("group hooks is not an array: %w", err)
	}
	return handlers, nil
}

func groupRuns(raw json.RawMessage, command string) bool {
	group, err := jsonedit.Parse(raw)
	if err != nil {
		return false
	}
	handlers, err := handlersOf(group)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(handlers, func(handler json.RawMessage) bool {
		return handlerCommand(handler) == command
	})
}

func handlerCommand(raw json.RawMessage) string {
	var handler struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(raw, &handler) != nil {
		return ""
	}
	return handler.Command
}

func setGroups(hooks *jsonedit.Object, event string, groups []json.RawMessage) error {
	return hooks.Set(event, joinArray(groups))
}

func setObject(parent *jsonedit.Object, key string, child *jsonedit.Object) error {
	raw, err := child.Marshal()
	if err != nil {
		return err
	}
	return parent.Set(key, raw)
}

// joinArray builds the array by hand: json.Marshal would rewrite the user's
// raw values (it escapes <, > and &).
func joinArray(items []json.RawMessage) json.RawMessage {
	out := []byte{'['}
	for position, item := range items {
		if position > 0 {
			out = append(out, ',')
		}
		out = append(out, item...)
	}
	return append(out, ']')
}

// marshalPlain encodes without HTML escaping so commands stay readable.
func marshalPlain(value any) (json.RawMessage, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode hook entry: %w", err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
