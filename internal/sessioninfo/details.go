package sessioninfo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/text"
)

// maxJSONBytes caps a "json" details file, read whole.
const maxJSONBytes = 1 << 20

// detailsRead is one read of one card. A converter that several fields
// name runs once per read.
type detailsRead struct {
	reader    Reader
	card      state.Card
	converted map[string]converterOutput
}

// converterOutput is what a details converter prints.
type converterOutput map[string]any

// first returns the value of the first source that gives one. field names
// the detail, for a converter's output.
func (d detailsRead) first(sources []agents.DetailSource, field string) (any, bool) {
	for _, source := range sources {
		if value, ok := d.value(source, field); ok {
			return value, true
		}
	}
	return nil, false
}

func (d detailsRead) value(source agents.DetailSource, field string) (any, bool) {
	switch {
	case source.Provider == agents.DetailsClaudeTranscriptContext:
		path, ok := d.homeFile("{transcript}")
		if !ok {
			return nil, false
		}
		if percent := ClaudeContext(path, d.reader.ContextSizes); percent != nil {
			return *percent, true
		}
		return nil, false
	case source.DetailsConverter() != nil:
		value, ok := d.convert(*source.DetailsConverter())[field]
		return value, ok && value != nil
	case source.JSONL != "":
		return d.fromJSONLines(source)
	default:
		return d.fromJSON(source)
	}
}

// fromJSONLines takes the last line of the file's tail that matches the
// source's condition, and its value.
func (d detailsRead) fromJSONLines(source agents.DetailSource) (any, bool) {
	path, ok := d.homeFile(source.JSONL)
	if !ok {
		return nil, false
	}
	var found any
	var matched bool
	eachLastLine(path, source.TailBytes(), func(line []byte) bool {
		var root any
		if json.Unmarshal(line, &root) != nil || !source.Last.Hold(root, d.card.Session) {
			return true
		}
		found, matched = source.Value.Evaluate(root)
		return false // the last matching line decides, even without a value
	})
	return found, matched
}

func (d detailsRead) fromJSON(source agents.DetailSource) (any, bool) {
	path, ok := d.homeFile(source.JSON)
	if !ok {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxJSONBytes+1))
	if err != nil || len(data) > maxJSONBytes {
		return nil, false
	}
	var root any
	if json.Unmarshal(data, &root) != nil || !source.Last.Hold(root, d.card.Session) {
		return nil, false
	}
	return source.Value.Evaluate(root)
}

// convert runs a details converter once per read, with the card on stdin.
func (d detailsRead) convert(converter agents.Converter) converterOutput {
	if output, done := d.converted[converter.Run]; done {
		return output
	}
	output := converterOutput{}
	d.converted[converter.Run] = output
	input, err := json.Marshal(struct {
		Session    string `json:"session"`
		Cwd        string `json:"cwd"`
		Transcript string `json:"transcript"`
	}{d.card.Session, d.card.Cwd, d.card.Transcript})
	if err != nil {
		return output
	}
	env := agents.ConverterEnv(d.reader.Home, os.Getenv("PATH"), d.card.Source, "")
	printed, err := agents.RunProgram(context.Background(), converter, input, env)
	if err != nil {
		return output
	}
	_ = json.Unmarshal(bytes.TrimSpace(printed), &output) // not an object: no details
	return output
}

// homeFile fills the placeholders of a details path and checks that it is
// a regular file under the user's home after symlinks are resolved.
func (d detailsRead) homeFile(pattern string) (string, bool) {
	var path string
	switch {
	case strings.HasPrefix(pattern, "~/"):
		path = filepath.Join(d.reader.Home, pattern[2:])
	case strings.HasPrefix(pattern, "{transcript}"):
		path = d.card.Transcript + pattern[len("{transcript}"):]
	case strings.HasPrefix(pattern, "{cwd}"):
		path = d.card.Cwd + pattern[len("{cwd}"):]
	}
	if d.card.Transcript == "" && strings.HasPrefix(pattern, "{transcript}") || d.card.Cwd == "" && strings.HasPrefix(pattern, "{cwd}") {
		return "", false
	}
	return insideHome(d.reader.Home, path)
}

func insideHome(home, path string) (string, bool) {
	if home == "" || path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", false
	}
	relative, err := filepath.Rel(realHome, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return resolved, true
}

// cleanName and cleanModel apply the card text rules to what a file or a
// program gave: secrets masked, cut to the card's length.
func cleanName(name string) string   { return text.ShortTitle(name) }
func cleanModel(model string) string { return text.ShortModel(text.MaskSecrets(model)) }
