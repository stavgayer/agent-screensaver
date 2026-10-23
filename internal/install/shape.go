package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"agent-screensaver/internal/jsonedit"
)

// hooksKey is the top-level key of a hook file that holds every event.
const hooksKey = "hooks"

// hookShape says where our handler sits inside the hooks object. Each
// event maps to a list; the shape says what the list holds.
type hookShape interface {
	// add appends our handler to the event's list unless the list already
	// runs command. groupKeys is the recipe's group object (grouped only).
	add(list []json.RawMessage, handler, groupKeys json.RawMessage, command string) ([]json.RawMessage, error)
	// remove drops the handlers whose command is one of commands, and a
	// group that our removal emptied. Items it did not touch keep their bytes.
	remove(list []json.RawMessage, commands []string) (kept []json.RawMessage, removed bool, err error)
	// runs reports whether the list holds a handler with this command.
	runs(list []json.RawMessage, command string) bool
}

// Shape names of the recipe (spec §7d "The install recipe").
const (
	shapeGrouped = "grouped"
	shapeFlat    = "flat"
)

var hookShapes = map[string]hookShape{shapeGrouped: groupedShape{}, shapeFlat: flatShape{}}

// groupedShape is hooks.<Event> = [{<group keys>, "hooks": [handler]}]
// (Claude Code, Codex).
type groupedShape struct{}

func (groupedShape) add(list []json.RawMessage, handler, groupKeys json.RawMessage, command string) ([]json.RawMessage, error) {
	if (groupedShape{}).runs(list, command) {
		return list, nil
	}
	group, err := jsonedit.Parse(groupKeys)
	if err != nil {
		return nil, fmt.Errorf("group: %w", err)
	}
	if err := group.Set(hooksKey, joinArray([]json.RawMessage{handler})); err != nil {
		return nil, err
	}
	entry, err := group.Marshal()
	if err != nil {
		return nil, err
	}
	return append(list, compactJSON(entry)), nil
}

func (groupedShape) remove(list []json.RawMessage, commands []string) ([]json.RawMessage, bool, error) {
	var kept []json.RawMessage
	removedAny := false
	for _, raw := range list {
		group, err := jsonedit.Parse(raw)
		if err != nil {
			return nil, false, fmt.Errorf("group is not an object: %w", err)
		}
		handlers, err := arrayOf(group, hooksKey)
		if err != nil {
			return nil, false, err
		}
		remaining, removed, _ := flatShape{}.remove(handlers, commands)
		switch {
		case !removed:
			kept = append(kept, raw)
		case len(remaining) == 0:
			removedAny = true
		default:
			removedAny = true
			if err := group.Set(hooksKey, joinArray(remaining)); err != nil {
				return nil, false, err
			}
			marshaled, err := group.Marshal()
			if err != nil {
				return nil, false, err
			}
			kept = append(kept, marshaled)
		}
	}
	return kept, removedAny, nil
}

func (groupedShape) runs(list []json.RawMessage, command string) bool {
	return slices.ContainsFunc(list, func(raw json.RawMessage) bool {
		group, err := jsonedit.Parse(raw)
		if err != nil {
			return false
		}
		handlers, err := arrayOf(group, hooksKey)
		return err == nil && flatShape{}.runs(handlers, command)
	})
}

// flatShape is hooks.<event> = [handler] (Cursor).
type flatShape struct{}

func (flatShape) add(list []json.RawMessage, handler, _ json.RawMessage, command string) ([]json.RawMessage, error) {
	if (flatShape{}).runs(list, command) {
		return list, nil
	}
	return append(list, handler), nil
}

func (flatShape) remove(list []json.RawMessage, commands []string) ([]json.RawMessage, bool, error) {
	kept := slices.DeleteFunc(slices.Clone(list), func(handler json.RawMessage) bool {
		return slices.Contains(commands, handlerCommand(handler))
	})
	return kept, len(kept) != len(list), nil
}

func (flatShape) runs(list []json.RawMessage, command string) bool {
	return slices.ContainsFunc(list, func(handler json.RawMessage) bool { return handlerCommand(handler) == command })
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

// arrayOf reads the array under key; a missing key is an empty array.
func arrayOf(object *jsonedit.Object, key string) ([]json.RawMessage, error) {
	raw, ok := object.Get(key)
	if !ok {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%s is not an array: %w", key, err)
	}
	return items, nil
}

// objectOf reads the object under key; a missing key is an empty object.
func objectOf(object *jsonedit.Object, key string) (*jsonedit.Object, error) {
	raw, ok := object.Get(key)
	if !ok {
		return &jsonedit.Object{}, nil
	}
	child, err := jsonedit.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not an object: %w", key, err)
	}
	return child, nil
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

func compactJSON(data []byte) json.RawMessage {
	var out bytes.Buffer
	if json.Compact(&out, data) != nil {
		return data
	}
	return out.Bytes()
}

// marshalPlain encodes without HTML escaping so commands stay readable.
func marshalPlain(value any) (json.RawMessage, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode %v: %w", value, err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
