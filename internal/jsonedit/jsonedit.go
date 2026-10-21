// Package jsonedit edits the top-level keys of a JSON object without
// disturbing the rest of the file. Go's map[string]any sorts keys on write,
// which would reorder a user's hand-kept settings; here keys keep their
// original order and untouched values keep their original bytes.
//
// The same API works on nested objects: Parse the raw value returned by Get,
// edit it, and Set the bytes of its Marshal back on the parent.
package jsonedit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

type entry struct {
	key string
	raw json.RawMessage
}

// Object is a JSON object whose keys stay in insertion order.
type Object struct {
	entries []entry
}

// Parse reads a JSON object. Empty or whitespace-only input is an empty
// object. Anything else that is not exactly one well-formed object with unique
// top-level keys is an error, so callers can abort instead of guessing.
func Parse(data []byte) (*Object, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &Object{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := expectObjectStart(decoder); err != nil {
		return nil, err
	}
	object := &Object{}
	for decoder.More() {
		if err := object.readEntry(decoder); err != nil {
			return nil, err
		}
	}
	if _, err := decoder.Token(); err != nil { // closing brace
		return nil, fmt.Errorf("jsonedit: read object end: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("jsonedit: unexpected data after object")
	}
	return object, nil
}

func expectObjectStart(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("jsonedit: read object start: %w", err)
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return errors.New("jsonedit: top-level value is not an object")
	}
	return nil
}

func (o *Object) readEntry(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("jsonedit: read key: %w", err)
	}
	key, ok := token.(string)
	if !ok {
		return fmt.Errorf("jsonedit: unexpected key token %v", token)
	}
	if o.index(key) >= 0 {
		return fmt.Errorf("jsonedit: duplicate key %q", key)
	}
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("jsonedit: read value of %q: %w", key, err)
	}
	o.entries = append(o.entries, entry{key: key, raw: raw})
	return nil
}

func (o *Object) index(key string) int {
	return slices.IndexFunc(o.entries, func(e entry) bool { return e.key == key })
}

// Get returns a copy of the raw value stored under key.
func (o *Object) Get(key string) (json.RawMessage, bool) {
	position := o.index(key)
	if position < 0 {
		return nil, false
	}
	return slices.Clone(o.entries[position].raw), true
}

// Set stores a copy of raw under key. An existing key keeps its position; a
// new key is appended. raw must be valid JSON so Marshal can never fail on it.
func (o *Object) Set(key string, raw json.RawMessage) error {
	if !json.Valid(raw) {
		return fmt.Errorf("jsonedit: value for %q is not valid JSON", key)
	}
	value := entry{key: key, raw: slices.Clone(raw)}
	if position := o.index(key); position >= 0 {
		o.entries[position] = value
		return nil
	}
	o.entries = append(o.entries, value)
	return nil
}

// Delete removes key if present.
func (o *Object) Delete(key string) {
	if position := o.index(key); position >= 0 {
		o.entries = slices.Delete(o.entries, position, position+1)
	}
}

// Keys returns the keys in their current order.
func (o *Object) Keys() []string {
	keys := make([]string, len(o.entries))
	for position, e := range o.entries {
		keys[position] = e.key
	}
	return keys
}

// Marshal writes the object with 2-space indentation and a trailing newline.
func (o *Object) Marshal() ([]byte, error) {
	var compact bytes.Buffer
	compact.WriteByte('{')
	for position, e := range o.entries {
		if position > 0 {
			compact.WriteByte(',')
		}
		key, err := marshalKey(e.key)
		if err != nil {
			return nil, err
		}
		compact.Write(key)
		compact.WriteByte(':')
		compact.Write(e.raw)
	}
	compact.WriteByte('}')

	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("jsonedit: indent: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// marshalKey quotes a key without HTML escaping, so "<&>" stays readable.
func marshalKey(key string) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(key); err != nil {
		return nil, fmt.Errorf("jsonedit: encode key %q: %w", key, err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
