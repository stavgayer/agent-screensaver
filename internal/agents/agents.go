// Package agents turns the hook input of coding agents (JSON on stdin) into
// events. Adding a tool means one new file and one line in All.
package agents

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/jsonedit"
	"agent-screensaver/internal/text"
)

// Adapter maps one tool's hook input to an event.
type Adapter interface {
	// Name is the tool name, for example "claude". It is also the event Source.
	Name() string
	// Map returns false when the input must be ignored.
	Map(input []byte, nowMs int64) (event.Event, bool)
}

// All is the one registry of adapters.
func All() []Adapter {
	return []Adapter{Claude{}, Codex{}}
}

// ByName finds an adapter by its name.
func ByName(name string) (Adapter, bool) {
	for _, adapter := range All() {
		if adapter.Name() == name {
			return adapter, true
		}
	}
	return nil, false
}

// Hook event names shared by the tools.
const (
	hookUserPromptSubmit = "UserPromptSubmit"
	hookPreToolUse       = "PreToolUse"
	hookNotification     = "Notification"
	hookPermission       = "PermissionRequest"
	hookPostToolUse      = "PostToolUse"
	hookStop             = "Stop"
	hookSessionEnd       = "SessionEnd"
)

// The tool name lists live here so both adapters share them.
var (
	shellTools = map[string]bool{"bash": true, "shell": true, "local_shell": true, "exec_command": true}
	editTools  = map[string]bool{"edit": true, "multiedit": true, "write": true, "notebookedit": true, "apply_patch": true}
)

// fields holds what every adapter reads from a hook payload.
type fields struct {
	hookEvent        string
	session          string
	project          string
	prompt           string
	toolName         string
	toolInput        map[string]any
	notificationType string
	message          string
	// noTranscript is true when the payload has "transcript_path": null.
	noTranscript bool
}

// hookFields decodes a payload. It returns false for invalid JSON or when
// there is no session id.
func hookFields(input []byte) (fields, bool) {
	var raw map[string]any
	if err := json.Unmarshal(input, &raw); err != nil {
		return fields{}, false
	}
	session := firstString(raw, "session_id", "thread_id", "conversation_id")
	if session == "" {
		return fields{}, false
	}
	toolInput, _ := raw["tool_input"].(map[string]any)
	project := ""
	if cwd := stringField(raw, "cwd"); cwd != "" {
		project = filepath.Base(cwd)
	}
	transcript, hasTranscript := raw["transcript_path"]
	return fields{
		hookEvent:        stringField(raw, "hook_event_name"),
		session:          session,
		project:          project,
		prompt:           stringField(raw, "prompt"),
		toolName:         stringField(raw, "tool_name"),
		toolInput:        toolInput,
		notificationType: stringField(raw, "notification_type"),
		message:          stringField(raw, "message"),
		noTranscript:     hasTranscript && transcript == nil,
	}, true
}

// mapCommon handles the events both tools share.
func mapCommon(source string, hook fields, nowMs int64) (event.Event, bool) {
	var kind event.Kind
	title := ""
	switch hook.hookEvent {
	case hookUserPromptSubmit:
		kind, title = event.KindStarted, text.ShortTitle(hook.prompt)
	case hookPreToolUse:
		kind, title = event.KindToolIntent, DescribeTool(hook.toolName, hook.toolInput)
	case hookPermission:
		kind, title = event.KindApproval, DescribeTool(hook.toolName, hook.toolInput)
	case hookPostToolUse:
		kind = event.KindResumed
	case hookStop:
		kind = event.KindFinished
	case hookSessionEnd:
		kind = event.KindEnded
	default:
		return event.Event{}, false
	}
	return newEvent(source, hook, kind, title, nowMs), true
}

func newEvent(source string, hook fields, kind event.Kind, title string, nowMs int64) event.Event {
	return event.Event{
		V:       event.Version,
		Source:  source,
		Session: hook.session,
		Kind:    kind,
		AtMs:    nowMs,
		Project: hook.project,
		Title:   title,
	}
}

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
		return commandText(input["command"])
	case editTools[lowerName]:
		return firstString(input, "file_path", "path")
	}
	return ""
}

// commandText accepts a string or, as Codex sends it, a list of strings.
func commandText(value any) string {
	switch command := value.(type) {
	case string:
		return command
	case []any:
		parts := make([]string, 0, len(command))
		for _, part := range command {
			if word, ok := part.(string); ok {
				parts = append(parts, word)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(values, key); value != "" {
			return value
		}
	}
	return ""
}

// HookInstaller edits one tool's hook config file. Both adapters implement it,
// so install loops over All instead of naming tools.
type HookInstaller interface {
	// ConfigPath is the tool's hook config file under home.
	ConfigPath(home string) string
	// AddHooks appends our entries for command. It is idempotent.
	AddHooks(cfg *jsonedit.Object, command string) error
	// RemoveHooks removes only the entries whose command is exactly command.
	RemoveHooks(cfg *jsonedit.Object, command string) error
}
