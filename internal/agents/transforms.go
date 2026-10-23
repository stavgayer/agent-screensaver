package agents

import (
	"path/filepath"
	"strings"

	"agent-screensaver/internal/text"
)

// transforms are the named changes a manifest can apply to a value. They
// are the shared card text rules; a manifest can only name them.
var transforms = map[string]func(any) (any, bool){
	"basename":     basename,
	"prompt_title": stringTransform(text.PromptTitle),
	"short_title":  stringTransform(text.ShortTitle),
	"model_name":   stringTransform(text.ShortModel),
	"count":        count,
	"percent":      percent,
	"text":         joinedText,
}

func stringTransform(change func(string) string) func(any) (any, bool) {
	return func(value any) (any, bool) {
		input, ok := value.(string)
		if !ok {
			return nil, false
		}
		return nonEmpty(change(input))
	}
}

// basename is the last folder name of a path.
func basename(value any) (any, bool) {
	path, ok := value.(string)
	if !ok || path == "" {
		return nil, false
	}
	return filepath.Base(path), true
}

// count is the length of an array; anything else gives no value.
func count(value any) (any, bool) {
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	return float64(len(list)), true
}

// percent is a number clamped to 0 to 100.
func percent(value any) (any, bool) {
	number, ok := value.(float64)
	if !ok {
		return nil, false
	}
	return min(max(number, 0), 100), true
}

// joinedText is a string, or the strings of an array joined with spaces
// (Codex sends a command as a list of words).
func joinedText(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		return nonEmpty(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			if word, ok := part.(string); ok {
				parts = append(parts, word)
			}
		}
		return nonEmpty(strings.Join(parts, " "))
	}
	return nil, false
}

// The tool name lists of the tool description, shared by every agent.
var (
	shellTools = map[string]bool{"bash": true, "shell": true, "local_shell": true, "exec_command": true}
	editTools  = map[string]bool{"edit": true, "multiedit": true, "write": true, "notebookedit": true, "apply_patch": true}
)

// DescribeTool names a tool call for the screen: the tool name plus the
// command or file it works on, masked and shortened.
func DescribeTool(name string, input map[string]any) string {
	detail := toolDetail(strings.ToLower(name), input)
	if detail == "" {
		return text.ShortTitle(name)
	}
	return text.ShortTitle(name + ": " + detail)
}

func toolDetail(lowerName string, input map[string]any) string {
	switch {
	case shellTools[lowerName]:
		command, _ := joinedText(input["command"])
		detail, _ := command.(string)
		return detail
	case editTools[lowerName]:
		for _, key := range []string{"file_path", "path"} {
			if path, ok := input[key].(string); ok && path != "" {
				return path
			}
		}
	}
	return ""
}
