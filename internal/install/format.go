package install

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	"agent-screensaver/internal/jsonedit"
)

// configFormat reads and writes a tool's config file while keeping its key
// order and every value we do not touch. JSON is the only one in version 1:
// the standard library has no TOML editor that keeps a file byte for byte.
type configFormat interface {
	parse(path string, data []byte) (*jsonedit.Object, error)
	// marshal writes config in the layout of original, the user's file
	// (nil for a new file): the same indent per level.
	marshal(config *jsonedit.Object, original []byte) ([]byte, error)
}

const formatJSON = "json"

var configFormats = map[string]configFormat{formatJSON: jsonFormat{}}

type jsonFormat struct{}

var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// parse refuses anything that is not exactly one JSON object, so a damaged
// file is never replaced.
func (jsonFormat) parse(path string, data []byte) (*jsonedit.Object, error) {
	if bytes.HasPrefix(data, byteOrderMark) {
		return nil, fmt.Errorf("%s starts with a UTF-8 byte order mark; remove it and run again", path)
	}
	config, err := jsonedit.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid JSON (%v)", path, err)
	}
	return config, nil
}

func (jsonFormat) marshal(config *jsonedit.Object, original []byte) ([]byte, error) {
	return config.MarshalIndent(jsonedit.DetectIndent(original))
}

// readConfig reads a user file. A missing file is an empty object with a nil
// original.
func readConfig(format configFormat, path string) (*jsonedit.Object, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &jsonedit.Object{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	config, err := format.parse(path, data)
	return config, data, err
}
