package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"agent-screensaver/internal/event"
)

// The mapping language (docs/agents.md): field paths, constants, named
// transforms and simple conditions. It has no loops and no expressions; a
// payload it cannot describe needs a converter program.

// kindIgnore is the rule kind that drops a payload on purpose.
const kindIgnore event.Kind = "ignore"

var knownKinds = map[event.Kind]bool{
	event.KindStarted: true, event.KindToolIntent: true, event.KindApproval: true,
	event.KindResumed: true, event.KindFinished: true, event.KindEnded: true,
	event.KindLimits: true, event.KindInfo: true, kindIgnore: true,
}

// LimitsSession is the session of a limits event whose payload has no
// usable session id. Limits are per source and window, not per session.
const LimitsSession = "limits"

// Path is a field path: keys joined by dots, where a number picks an array
// item ("workspace_roots.0").
type Path []string

func parsePath(text string) (Path, error) {
	parts := strings.Split(text, ".")
	if slices.Contains(parts, "") {
		return nil, fmt.Errorf("path %q has an empty part", text)
	}
	return parts, nil
}

// lookup returns the value at the path and whether the key exists. A JSON
// null exists and is nil.
func (p Path) lookup(root any) (any, bool) {
	current := root
	for _, part := range p {
		switch node := current.(type) {
		case map[string]any:
			value, ok := node[part]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func (p *Path) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("a path must be a string: %w", err)
	}
	parsed, err := parsePath(text)
	if err != nil {
		return err
	}
	*p = parsed
	return nil
}

// Value is one field's value in one of the forms of docs/agents.md.
type Value struct {
	paths       []Path
	constant    any
	hasConstant bool
	tool        *toolValue
	ratio       []Path // details only: a divided by b, as a percent
	transform   string
}

type toolValue struct{ name, input Path }

// valueManifest is the object form of a Value.
type valueManifest struct {
	Path  json.RawMessage `json:"path,omitempty"`
	Const json.RawMessage `json:"const,omitempty"`
	Tool  *Path           `json:"tool,omitempty"`
	Input *Path           `json:"input,omitempty"`
	Ratio []Path          `json:"ratio,omitempty"`
	As    string          `json:"as,omitempty"`
}

func (v *Value) UnmarshalJSON(data []byte) error {
	var short string
	if json.Unmarshal(data, &short) == nil {
		path, err := parsePath(short)
		*v = Value{paths: []Path{path}}
		return err
	}
	var form valueManifest
	if err := decodeStrict(data, &form); err != nil {
		return fmt.Errorf("value: %w", err)
	}
	return v.fromManifest(form)
}

func (v *Value) fromManifest(form valueManifest) error {
	forms := 0
	for _, filled := range []bool{len(form.Path) > 0, len(form.Const) > 0, form.Tool != nil, form.Ratio != nil} {
		if filled {
			forms++
		}
	}
	if forms != 1 {
		return errors.New("a value needs exactly one of path, const, tool and ratio")
	}
	if form.As != "" {
		if _, ok := transforms[form.As]; !ok {
			return fmt.Errorf("unknown transform %q", form.As)
		}
	}
	*v = Value{transform: form.As}
	switch {
	case len(form.Path) > 0:
		paths, err := parsePaths(form.Path)
		v.paths = paths
		return err
	case len(form.Const) > 0:
		return v.setConstant(form.Const)
	case form.Tool != nil:
		if form.Input == nil {
			return errors.New("a tool value needs an input path")
		}
		v.tool = &toolValue{name: *form.Tool, input: *form.Input}
	default:
		if len(form.Ratio) != 2 {
			return errors.New("ratio needs two paths")
		}
		v.ratio = form.Ratio
	}
	return nil
}

// parsePaths reads one path or a list of paths.
func parsePaths(raw json.RawMessage) ([]Path, error) {
	var many []Path
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		if err := json.Unmarshal(raw, &many); err != nil {
			return nil, err
		}
		if len(many) == 0 {
			return nil, errors.New("a path list must not be empty")
		}
		return many, nil
	}
	var one Path
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	return []Path{one}, nil
}

func (v *Value) setConstant(raw json.RawMessage) error {
	var constant any
	if err := json.Unmarshal(raw, &constant); err != nil {
		return err
	}
	switch constant.(type) {
	case string, float64, bool:
		v.constant, v.hasConstant = constant, true
		return nil
	}
	return errors.New("const must be a string, number or boolean")
}

// Evaluate returns the value for a payload, after its transform. Missing,
// null and empty values give no value.
func (v Value) Evaluate(root any) (any, bool) {
	value, ok := v.raw(root)
	if !ok {
		return nil, false
	}
	if v.transform != "" {
		return transforms[v.transform](value)
	}
	return value, true
}

func (v Value) raw(root any) (any, bool) {
	switch {
	case v.hasConstant:
		return v.constant, true
	case v.tool != nil:
		name, _ := lookupString(root, v.tool.name)
		input, _ := lookupAny(root, v.tool.input).(map[string]any)
		return nonEmpty(DescribeTool(name, input))
	case v.ratio != nil:
		return ratioPercent(lookupAny(root, v.ratio[0]), lookupAny(root, v.ratio[1]))
	}
	for _, path := range v.paths {
		if value, ok := nonEmpty(lookupAny(root, path)); ok {
			return value, true
		}
	}
	return nil, false
}

func lookupAny(root any, path Path) any {
	value, _ := path.lookup(root)
	return value
}

func lookupString(root any, path Path) (string, bool) {
	text, ok := lookupAny(root, path).(string)
	return text, ok && text != ""
}

// nonEmpty is false for a missing value, null and "". An empty list is a
// value: its count is 0.
func nonEmpty(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		return typed, typed != ""
	}
	return value, true
}

// ratioPercent is a divided by b, as a whole percent from 0 to 100.
func ratioPercent(a, b any) (any, bool) {
	numerator, okA := a.(float64)
	denominator, okB := b.(float64)
	if !okA || !okB || denominator <= 0 {
		return nil, false
	}
	return math.Round(min(max(numerator/denominator*100, 0), 100)), true
}

// Condition is a test on one field, with exactly one operator.
type Condition struct {
	path     Path
	operator string
	operand  any
	list     []any
	present  bool
}

func (c *Condition) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("a condition must be an object: %w", err)
	}
	rawPath, ok := fields["path"]
	if !ok {
		return errors.New("a condition needs a path")
	}
	if err := json.Unmarshal(rawPath, &c.path); err != nil {
		return err
	}
	delete(fields, "path")
	if len(fields) != 1 {
		return fmt.Errorf("a condition needs exactly one operator (equals, in, contains, present), it has %d", len(fields))
	}
	for operator, raw := range fields {
		c.operator = operator
		return c.setOperand(raw)
	}
	return nil
}

func (c *Condition) setOperand(raw json.RawMessage) error {
	switch c.operator {
	case "equals":
		if err := json.Unmarshal(raw, &c.operand); err != nil {
			return err
		}
		switch c.operand.(type) {
		case nil, string, float64, bool:
			return nil
		}
		return errors.New("equals takes a string, number, boolean or null")
	case "in":
		return json.Unmarshal(raw, &c.list)
	case "contains":
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return fmt.Errorf("contains takes a string: %w", err)
		}
		c.operand = strings.ToLower(text)
		return nil
	case "present":
		return json.Unmarshal(raw, &c.present)
	}
	return fmt.Errorf("unknown operator %q", c.operator)
}

// holds tests the condition. session replaces "{session}" in the operand
// when it is not empty (details reads).
func (c Condition) holds(root any, session string) bool {
	value, exists := c.path.lookup(root)
	switch c.operator {
	case "equals":
		return exists && equalValues(value, replaceSession(c.operand, session))
	case "in":
		return exists && slices.ContainsFunc(c.list, func(item any) bool { return equalValues(value, replaceSession(item, session)) })
	case "contains":
		text, ok := value.(string)
		return ok && strings.Contains(strings.ToLower(text), replaceSession(c.operand, session).(string))
	}
	return (exists && value != nil) == c.present
}

func equalValues(a, b any) bool {
	switch typed := b.(type) {
	case nil:
		return a == nil
	case string, float64, bool:
		return a == typed
	}
	return false
}

func replaceSession(operand any, session string) any {
	text, ok := operand.(string)
	if !ok || session == "" {
		return operand
	}
	return strings.ReplaceAll(text, "{session}", session)
}

// Conditions is one condition or a list; all must hold.
type Conditions []Condition

func (c *Conditions) UnmarshalJSON(data []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		var many []Condition
		if err := json.Unmarshal(data, &many); err != nil {
			return err
		}
		*c = many
		return nil
	}
	var one Condition
	if err := json.Unmarshal(data, &one); err != nil {
		return err
	}
	*c = Conditions{one}
	return nil
}

// Hold reports whether every condition holds. session fills "{session}".
func (c Conditions) Hold(root any, session string) bool {
	for _, condition := range c {
		if !condition.holds(root, session) {
			return false
		}
	}
	return true
}

// names is one event name or a list of names.
type names []string

func (n *names) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		*n = names{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return errors.New("on must be an event name or a list of names")
	}
	*n = many
	return nil
}

// Rule maps the payloads it matches to one event kind.
type Rule struct {
	On   names      `json:"on,omitempty"`
	If   Conditions `json:"if,omitempty"`
	Kind event.Kind `json:"kind"`
	Set  fieldSet   `json:"set,omitempty"`
}

func (r Rule) matches(eventName string, root any) bool {
	return (len(r.On) == 0 || slices.Contains(r.On, eventName)) && r.If.Hold(root, "")
}

// fieldSet holds the event fields a rule or "common" sets.
type fieldSet struct {
	values map[string]Value
	limits []limitSpec
}

// limitSpec is one limits reading. window is a fixed name.
type limitSpec struct {
	Window      event.Window `json:"window"`
	UsedPercent Value        `json:"used_percent"`
	ResetsAt    *Value       `json:"resets_at,omitempty"`
}

var (
	stringFields = []string{"project", "title", "name", "model", "model_id", "cwd", "transcript"}
	numberFields = []string{"context_percent", "background", "context_window"}
	knownWindows = []event.Window{event.FiveHour, event.Week, event.Plan}
)

func (f *fieldSet) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("set must be an object: %w", err)
	}
	f.values = map[string]Value{}
	for name, value := range raw {
		if name == "limits" {
			if err := decodeLimits(value, &f.limits); err != nil {
				return fmt.Errorf("limits: %w", err)
			}
			continue
		}
		if !slices.Contains(stringFields, name) && !slices.Contains(numberFields, name) {
			return fmt.Errorf("unknown event field %q", name)
		}
		var parsed Value
		if err := json.Unmarshal(value, &parsed); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		f.values[name] = parsed
	}
	return nil
}

func decodeLimits(raw json.RawMessage, target *[]limitSpec) error {
	var specs []json.RawMessage
	if err := json.Unmarshal(raw, &specs); err != nil {
		return err
	}
	for _, spec := range specs {
		var parsed limitSpec
		if err := decodeStrict(spec, &parsed); err != nil {
			return err
		}
		if !slices.Contains(knownWindows, parsed.Window) {
			return fmt.Errorf("unknown window %q", parsed.Window)
		}
		*target = append(*target, parsed)
	}
	return nil
}

func (f fieldSet) usesRatio() bool {
	for _, value := range f.values {
		if value.ratio != nil {
			return true
		}
	}
	for _, spec := range f.limits {
		if spec.UsedPercent.ratio != nil || spec.ResetsAt != nil && spec.ResetsAt.ratio != nil {
			return true
		}
	}
	return false
}

// apply sets the fields that give a value. A field without one keeps what
// an earlier set (common) gave it.
func (f fieldSet) apply(target *event.Event, root any) {
	for name, value := range f.values {
		result, ok := value.Evaluate(root)
		if !ok {
			continue
		}
		if slices.Contains(stringFields, name) {
			if text, ok := result.(string); ok {
				setTextField(target, name, text)
			}
			continue
		}
		if number, ok := result.(float64); ok {
			setNumberField(target, name, number)
		}
	}
	if f.limits != nil {
		target.Limits = nil
		for _, spec := range f.limits {
			if reading, ok := spec.reading(root); ok {
				target.Limits = append(target.Limits, reading)
			}
		}
	}
}

func setTextField(target *event.Event, name, text string) {
	switch name {
	case "project":
		target.Project = text
	case "title":
		target.Title = text
	case "name":
		target.Name = text
	case "model":
		target.Model = text
	case "model_id":
		target.ModelID = text
	case "cwd":
		target.Cwd = text
	case "transcript":
		target.Transcript = text
	}
}

// setNumberField sets a number field. A count must be a whole number; a value
// of the wrong kind gives no value.
func setNumberField(target *event.Event, name string, number float64) {
	switch name {
	case "context_percent":
		target.ContextPercent = &number
	case "background":
		if number >= 0 && number == math.Trunc(number) {
			count := int(number)
			target.Background = &count
		}
	case "context_window":
		if number > 0 && number == math.Trunc(number) {
			target.ContextWindow = int(number)
		}
	}
}

// reading builds a limits reading. One without a used percent is dropped.
func (s limitSpec) reading(root any) (event.LimitReading, bool) {
	used, ok := s.UsedPercent.Evaluate(root)
	percent, isNumber := used.(float64)
	if !ok || !isNumber {
		return event.LimitReading{}, false
	}
	reading := event.LimitReading{Window: s.Window, UsedPercent: percent}
	if s.ResetsAt != nil {
		if resets, ok := s.ResetsAt.Evaluate(root); ok {
			if seconds, ok := resets.(float64); ok && seconds == math.Trunc(seconds) {
				whole := int64(seconds)
				reading.ResetsAt = &whole
			}
		}
	}
	return reading, true
}

// eventSource says where the event name comes from: a payload path, or the
// --event argument of our hook command.
type eventSource struct {
	path  Path
	isArg bool
}

func (s *eventSource) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &s.path); err == nil {
		return nil
	}
	var form struct {
		Arg string `json:"arg"`
	}
	if err := decodeStrict(data, &form); err != nil || form.Arg != "event" {
		return errors.New(`event must be a path or {"arg": "event"}`)
	}
	s.isArg = true
	return nil
}

// The replies our hook may print. Nothing else is accepted, so a manifest
// can never make our hook allow or deny an action.
const (
	ReplyNone        = "none"
	ReplyEmptyObject = "empty_object"
	ReplyAsk         = "ask"
)

var replyText = map[string]string{
	"":               "",
	ReplyNone:        "",
	ReplyEmptyObject: "{}\n",
	ReplyAsk:         `{"permission":"ask"}` + "\n",
}

// Reply is what our hook prints on stdout: Default for every event, or the
// event's own reply.
type Reply struct {
	Default string
	Events  map[string]string
}

func (r *Reply) UnmarshalJSON(data []byte) error {
	var one string
	if json.Unmarshal(data, &one) == nil {
		r.Default = one
	} else if err := decodeStrict(data, &struct {
		Default *string            `json:"default"`
		Events  *map[string]string `json:"events"`
	}{&r.Default, &r.Events}); err != nil {
		return fmt.Errorf("reply: %w", err)
	}
	if r.Default != "" && !validReply(r.Default) {
		return fmt.Errorf("reply %q is not one of none, empty_object, ask", r.Default)
	}
	for eventName, reply := range r.Events {
		if !validReply(reply) {
			return fmt.Errorf("reply %q of %s is not one of none, empty_object, ask", reply, eventName)
		}
	}
	return nil
}

func validReply(reply string) bool {
	return reply == ReplyNone || reply == ReplyEmptyObject || reply == ReplyAsk
}

// Text is what our hook prints for an event named by --event ("" when the
// command has no --event).
func (r Reply) Text(eventArg string) string {
	if reply, ok := r.Events[eventArg]; ok && eventArg != "" {
		return replyText[reply]
	}
	return replyText[r.Default]
}

// HasOwn reports whether an event has a reply of its own, so its command
// must carry --event.
func (r Reply) HasOwn(eventName string) bool {
	_, ok := r.Events[eventName]
	return ok
}

// Mapping turns a hook or status line payload into events.
type Mapping struct {
	event   *eventSource
	session Value
	common  fieldSet
	rules   []Rule
	Reply   Reply
	// Converter, when set, replaces the rules: a program maps the payload.
	Converter *Converter

	source string
	// everyRule is true for a status line mapping: each rule that matches
	// makes one event. A hook mapping stops at the first match.
	everyRule bool
}

type mappingKind int

const (
	hookMappingKind mappingKind = iota
	statuslineMappingKind
)

type mappingManifest struct {
	Event     *eventSource       `json:"event,omitempty"`
	Session   *Value             `json:"session,omitempty"`
	Common    *fieldSet          `json:"common,omitempty"`
	Rules     []Rule             `json:"rules,omitempty"`
	Reply     *Reply             `json:"reply,omitempty"`
	Converter *converterManifest `json:"converter,omitempty"`
}

func parseMapping(raw []byte, source, dir string, kind mappingKind) (Mapping, error) {
	if len(raw) == 0 {
		return Mapping{}, errors.New("a mapping is required")
	}
	var form mappingManifest
	if err := decodeStrict(raw, &form); err != nil {
		return Mapping{}, err
	}
	mapping := Mapping{event: form.Event, rules: form.Rules, source: source, everyRule: kind == statuslineMappingKind}
	if form.Reply != nil {
		if kind == statuslineMappingKind {
			return Mapping{}, errors.New("a status line has no reply")
		}
		mapping.Reply = *form.Reply
	}
	if form.Converter != nil {
		if form.Session != nil || form.Common != nil || form.Rules != nil {
			return Mapping{}, errors.New("a converter replaces session, common and rules; use one or the other")
		}
		converter, err := parseConverter(*form.Converter, dir, MaxConverterTimeout)
		mapping.Converter = &converter
		return mapping, err
	}
	if err := checkRulesForm(form, kind); err != nil {
		return Mapping{}, err
	}
	mapping.session = *form.Session
	if form.Common != nil {
		mapping.common = *form.Common
	}
	return mapping, nil
}

func checkRulesForm(form mappingManifest, kind mappingKind) error {
	switch {
	case kind == hookMappingKind && form.Event == nil:
		return errors.New("event is required")
	case kind == statuslineMappingKind && form.Event != nil:
		return errors.New("a status line mapping has no event")
	case form.Session == nil:
		return errors.New("session is required")
	case len(form.Rules) == 0:
		return errors.New("rules must hold at least one rule")
	case form.Session.ratio != nil || form.Common != nil && form.Common.usesRatio():
		return errors.New("ratio is only for details")
	}
	for index, rule := range form.Rules {
		switch {
		case !knownKinds[rule.Kind]:
			return fmt.Errorf("rule %d: unknown kind %q", index, rule.Kind)
		case kind == statuslineMappingKind && len(rule.On) > 0:
			return fmt.Errorf("rule %d: a status line rule has no on", index)
		case rule.Set.usesRatio():
			return fmt.Errorf("rule %d: ratio is only for details", index)
		}
	}
	return nil
}

// UsesEventArg reports whether the event name comes from --event.
func (m Mapping) UsesEventArg() bool { return m.event != nil && m.event.isArg }

// Map applies the rules to a payload. It reports false when the payload is
// ignored: invalid JSON, no session, no rule, an ignore rule, or nothing left
// after the event checks.
func (m Mapping) Map(payload []byte, eventArg string, nowMs int64) ([]event.Event, bool) {
	var root any
	if json.Unmarshal(payload, &root) != nil {
		return nil, false
	}
	eventName := eventArg
	if m.event != nil && !m.event.isArg {
		eventName, _ = lookupString(root, m.event.path)
	}
	session, _ := m.session.Evaluate(root)
	sessionText, _ := session.(string)
	var events []event.Event
	for _, rule := range m.rules {
		if !rule.matches(eventName, root) {
			continue
		}
		if rule.Kind != kindIgnore {
			if mapped, ok := m.build(rule, root, sessionText, nowMs); ok {
				events = append(events, mapped)
			}
		}
		if !m.everyRule {
			break
		}
	}
	return events, len(events) > 0
}

func (m Mapping) build(rule Rule, root any, session string, nowMs int64) (event.Event, bool) {
	if session == "" && rule.Kind != event.KindLimits {
		return event.Event{}, false
	}
	mapped := event.Event{Session: session, Kind: rule.Kind}
	m.common.apply(&mapped, root)
	rule.Set.apply(&mapped, root)
	normalized, err := Normalize(mapped, m.source, nowMs)
	return normalized, err == nil
}

// Events maps a payload with the rules or the converter. Converter
// problems come back as an error for the log; the events that passed the
// checks come back either way.
func (m Mapping) Events(ctx context.Context, payload []byte, eventArg string, env []string, nowMs int64) ([]event.Event, error) {
	if m.Converter == nil {
		events, _ := m.Map(payload, eventArg, nowMs)
		return events, nil
	}
	converted, err := RunConverter(ctx, *m.Converter, payload, env)
	var problems []error
	if err != nil {
		problems = append(problems, err)
	}
	var events []event.Event
	for _, candidate := range converted {
		normalized, err := Normalize(candidate, m.source, nowMs)
		if err != nil {
			problems = append(problems, fmt.Errorf("converter event dropped: %w", err))
			continue
		}
		events = append(events, normalized)
	}
	return events, errors.Join(problems...)
}
