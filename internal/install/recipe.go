package install

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"agent-screensaver/internal/agents"
	"agent-screensaver/internal/jsonedit"
	"agent-screensaver/internal/paths"
)

// hookCommand is the only command a recipe can make a tool run: our own
// binary. eventArg is set when the event name must come from the command.
// reply is the event's reply; it rides along in the command so our hook can
// still answer when the manifest cannot be read.
func hookCommand(bin, source, eventArg, reply string) string {
	command := shellQuote(bin) + " hook --tool " + source
	if eventArg != "" {
		command += " --event " + eventArg // CheckRecipe allows only safe event names
	}
	if reply != "" && reply != agents.ReplyNone {
		command += " --reply " + reply // one of the fixed reply names
	}
	return command
}

// agentHookCommand is the command of one event of an agent.
func agentHookCommand(bin string, agent agents.Agent, eventName string) string {
	return hookCommand(bin, agent.Source, eventArgFor(agent, eventName), agent.Hooks.Reply.For(eventName))
}

func statuslineCommand(bin, source string) string {
	return shellQuote(bin) + " statusline --tool " + source
}

// eventArgFor is the --event value of an event's command: every event when
// the mapping reads the name from --event, else only events with a reply of
// their own, so that reply never depends on the payload.
func eventArgFor(agent agents.Agent, eventName string) string {
	if agent.Hooks.UsesEventArg() || agent.Hooks.Reply.HasOwn(eventName) {
		return eventName
	}
	return ""
}

// workspace holds every file of one install or uninstall in memory, so
// each file is read and written once and nothing is written when any file
// is invalid.
type workspace struct {
	home, dataDir string
	// recordBin is the binary of the record whose entries are removed:
	// only commands of exactly our forms with it are ever removed.
	recordBin string
	files     map[string]*fileState
	order     []string
	copies    []fileWrite
	removals  []string // copied files to delete
}

type fileState struct {
	path     string
	format   configFormat
	config   *jsonedit.Object
	original []byte // nil when the file does not exist
	// ours are our commands, old and new: a file that holds one is no
	// longer the user's original and gets no new backup.
	ours   []string
	delete bool // install created it and only its new_file keys are left
}

type fileWrite struct {
	path    string
	content []byte
}

func newWorkspace(home, recordBin string) *workspace {
	return &workspace{home: home, dataDir: paths.ForHome(home).DataDir, recordBin: recordBin, files: map[string]*fileState{}}
}

// ownCommand matches the commands install writes: the hook command with an
// optional --event and --reply, and the status line command with an optional
// --tool. A record that lists anything else (an edited record) cannot make
// uninstall remove a command of the user's. "--reply ask" is no longer
// written, but uninstall still knows it so older installs come out clean.
func ownCommand(bin, source, command string) bool {
	quoted := regexp.QuoteMeta(shellQuote(bin))
	hook := `^` + quoted + ` hook --tool ` + regexp.QuoteMeta(source) +
		`( --event [A-Za-z0-9_.:-]{1,64})?( --reply (empty_object|ask))?$`
	statusline := `^` + quoted + ` statusline( --tool ` + regexp.QuoteMeta(source) + `)?$`
	return regexp.MustCompile(hook).MatchString(command) || regexp.MustCompile(statusline).MatchString(command)
}

// ownEntry is a record entry with only the commands of our own forms.
func (w *workspace) ownEntry(entry AgentRecord) AgentRecord {
	entry.Commands = slices.DeleteFunc(slices.Clone(entry.Commands), func(command string) bool {
		return !ownCommand(w.recordBin, entry.Source, command)
	})
	if !ownCommand(w.recordBin, entry.Source, entry.StatuslineCommand) {
		entry.StatuslineKey, entry.StatuslineCommand = "", ""
	}
	return entry
}

// open reads a file once. path is a recipe path ("~/...").
func (w *workspace) open(recipePath, format string) (*fileState, error) {
	full, err := writablePath(w.home, w.dataDir, recipePath)
	if err != nil {
		return nil, err
	}
	if state, ok := w.files[full]; ok {
		return state, nil
	}
	reader, ok := configFormats[format]
	if !ok {
		return nil, fmt.Errorf("unknown format %q", format)
	}
	config, original, err := readConfig(reader, full)
	if err != nil {
		return nil, err
	}
	state := &fileState{path: full, format: reader, config: config, original: original}
	w.files[full] = state
	w.order = append(w.order, full)
	return state, nil
}

// edits are the changed files, in the order they were opened.
func (w *workspace) edits() ([]configEdit, error) {
	var edits []configEdit
	for _, path := range w.order {
		state := w.files[path]
		if state.delete {
			edits = append(edits, configEdit{path: path, remove: true})
			continue
		}
		updated, err := state.format.marshal(state.config, state.original)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		updated = finalNewlineLike(state.original, updated)
		if bytes.Equal(updated, state.original) {
			continue
		}
		edits = append(edits, configEdit{path: path, original: state.original, updated: updated,
			backup: state.original != nil && !holdsAny(state.original, state.ours)})
	}
	return edits, nil
}

// finalNewlineLike gives updated the final newline state of the user's
// file, so install and uninstall do not add or drop one. A file we create
// (no original) keeps the newline the format writes.
func finalNewlineLike(original, updated []byte) []byte {
	if original == nil || bytes.HasSuffix(original, []byte("\n")) {
		return updated
	}
	return bytes.TrimRight(updated, "\n")
}

func holdsAny(data []byte, commands []string) bool {
	return slices.ContainsFunc(commands, func(command string) bool {
		return command != "" && strings.Contains(string(data), jsonEscaped(command))
	})
}

// write applies everything prepared: config files, then copied files.
func (w *workspace) write() error {
	edits, err := w.edits()
	if err != nil {
		return err
	}
	for _, edit := range edits {
		if err := writeEdit(edit); err != nil {
			return err
		}
	}
	for _, file := range w.copies {
		if err := writeFileAtomic(file.path, file.content, 0o644); err != nil {
			return fmt.Errorf("copy %s: %w", file.path, err)
		}
	}
	for _, path := range w.removals {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

// addAgent prepares the hooks, status line and files of one agent. prior is
// what an earlier install wrote for it (zero when none): its entries that
// the new recipe no longer writes are removed in the same edit, and the ones
// it writes again stay where they are.
func (w *workspace) addAgent(bin string, agent agents.Agent, prior AgentRecord) (AgentRecord, error) {
	if err := CheckRecipe(w.home, w.dataDir, agent.Install); err != nil {
		return AgentRecord{}, fmt.Errorf("%s: %w", agent.ID, err)
	}
	entry := AgentRecord{ID: agent.ID, Source: agent.Source}
	if hooks := agent.Install.Hooks; hooks != nil {
		if err := w.addHooks(bin, agent, *hooks, prior, &entry); err != nil {
			return AgentRecord{}, fmt.Errorf("%s: %w", agent.ID, err)
		}
	} else if err := w.removeHooks(prior, nil); err != nil {
		return AgentRecord{}, err
	}
	files, err := w.copyFiles(agent, prior)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("%s: %w", agent.ID, err)
	}
	entry.Files = files
	return entry, nil
}

func (w *workspace) addHooks(bin string, agent agents.Agent, hooks agents.HookRecipe, prior AgentRecord, entry *AgentRecord) error {
	commands := map[string]string{} // event -> our command
	for _, eventName := range hooks.Events {
		commands[eventName] = agentHookCommand(bin, agent, eventName)
	}
	keep := func(eventName, command string) bool {
		return prior.File == hooks.File && commands[eventName] == command
	}
	if err := w.removeHooks(prior, keep); err != nil {
		return err
	}
	state, err := w.open(hooks.File, hooks.Format)
	if err != nil {
		return err
	}
	entry.File, entry.Shape, entry.Events = hooks.File, hooks.Shape, slices.Clone(hooks.Events)
	entry.CreatedFile = state.original == nil || prior.File == hooks.File && prior.CreatedFile
	if state.original == nil {
		if entry.NewFileKeys, err = addNewFileKeys(state.config, hooks.NewFile); err != nil {
			return err
		}
	} else if prior.File == hooks.File {
		entry.NewFileKeys = prior.NewFileKeys
	}
	hooksObject, err := objectOf(state.config, hooksKey)
	if err != nil {
		return err
	}
	shape := hookShapes[hooks.Shape]
	for _, eventName := range hooks.Events {
		command := commands[eventName]
		handler, err := buildHandler(command, hooks.Handler)
		if err != nil {
			return err
		}
		list, err := arrayOf(hooksObject, eventName)
		if err != nil {
			return fmt.Errorf("%s: event %s: %w", state.path, eventName, err)
		}
		if list, err = shape.add(list, handler, groupKeys(hooks.Group), command); err != nil {
			return fmt.Errorf("%s: %w", state.path, err)
		}
		if err := hooksObject.Set(eventName, joinArray(list)); err != nil {
			return err
		}
		if !slices.Contains(entry.Commands, command) {
			entry.Commands = append(entry.Commands, command)
		}
	}
	if err := setObject(state.config, hooksKey, hooksObject); err != nil {
		return err
	}
	state.ours = append(state.ours, entry.Commands...)
	state.ours = append(state.ours, prior.Commands...)
	if agent.Install.Statusline != nil {
		return w.swapAgentStatusline(state, bin, agent, prior, entry)
	}
	return nil
}

func (w *workspace) swapAgentStatusline(state *fileState, bin string, agent agents.Agent, prior AgentRecord, entry *AgentRecord) error {
	ours := statuslineCommand(bin, agent.Source)
	earlier := w.ownEntry(prior).StatuslineCommand // an edited record cannot call the user's command ours
	previous, err := swapStatusline(state.config, agent.Install.Statusline.Key, ours, []string{earlier})
	if err != nil {
		return fmt.Errorf("%s: %w", state.path, err)
	}
	if previous == "" {
		previous = prior.PreviousCommand
	}
	entry.StatuslineKey, entry.StatuslineCommand, entry.PreviousCommand = agent.Install.Statusline.Key, ours, previous
	state.ours = append(state.ours, ours, prior.StatuslineCommand)
	return nil
}

func groupKeys(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

// buildHandler is {"type": "command", "command": <ours>, <handler keys>}.
func buildHandler(command string, extra json.RawMessage) (json.RawMessage, error) {
	handler := &jsonedit.Object{}
	quoted, err := marshalPlain(command)
	if err != nil {
		return nil, err
	}
	if err := handler.Set("type", json.RawMessage(`"command"`)); err != nil {
		return nil, err
	}
	if err := handler.Set("command", quoted); err != nil {
		return nil, err
	}
	if len(extra) > 0 {
		keys, err := jsonedit.Parse(extra)
		if err != nil {
			return nil, err
		}
		for _, key := range keys.Keys() {
			value, _ := keys.Get(key)
			if err := handler.Set(key, value); err != nil {
				return nil, err
			}
		}
	}
	data, err := handler.Marshal()
	if err != nil {
		return nil, err
	}
	return compactJSON(data), nil
}

// addNewFileKeys writes the new_file keys into a file we create and
// returns their names.
func addNewFileKeys(config *jsonedit.Object, raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	keys, err := jsonedit.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("new_file: %w", err)
	}
	for _, key := range keys.Keys() {
		value, _ := keys.Get(key)
		if err := config.Set(key, value); err != nil {
			return nil, err
		}
	}
	return keys.Keys(), nil
}

// removeHooks prepares the removal of what a record entry wrote, except
// the (event, command) pairs keep says stay. A nil keep removes all.
func (w *workspace) removeHooks(entry AgentRecord, keep func(eventName, command string) bool) error {
	entry = w.ownEntry(entry)
	if entry.File == "" {
		return nil
	}
	shape, ok := hookShapes[entry.Shape]
	if !ok {
		return fmt.Errorf("%s: unknown shape %q in the install record", entry.ID, entry.Shape)
	}
	state, err := w.open(entry.File, formatJSON)
	if err != nil {
		return err
	}
	state.ours = append(state.ours, entry.Commands...)
	if state.original == nil {
		return nil // the user deleted the file; nothing of ours is left in it
	}
	if err := removeFromEvents(state.config, shape, entry, keep); err != nil {
		return fmt.Errorf("%s: %w", state.path, err)
	}
	if keep == nil && entry.StatuslineKey != "" {
		err := restoreStatusline(state.config, entry.StatuslineKey, entry.PreviousCommand, entry.StatuslineCommand)
		if err != nil {
			return fmt.Errorf("%s: %w", state.path, err)
		}
	}
	if keep == nil && entry.CreatedFile && onlyKeys(state.config, entry.NewFileKeys) {
		state.delete = true
	}
	return nil
}

// removeFromEvents removes our handlers from the recorded events. A group,
// an event or the hooks key goes only when our removal emptied it.
func removeFromEvents(config *jsonedit.Object, shape hookShape, entry AgentRecord, keep func(string, string) bool) error {
	if _, ok := config.Get(hooksKey); !ok {
		return nil
	}
	hooksObject, err := objectOf(config, hooksKey)
	if err != nil {
		return err
	}
	removedAny := false
	for _, eventName := range entry.Events {
		stale := slices.DeleteFunc(slices.Clone(entry.Commands), func(command string) bool {
			return keep != nil && keep(eventName, command)
		})
		list, err := arrayOf(hooksObject, eventName)
		if err != nil {
			return fmt.Errorf("event %s: %w", eventName, err)
		}
		kept, removed, err := shape.remove(list, stale)
		if err != nil {
			return fmt.Errorf("event %s: %w", eventName, err)
		}
		if !removed {
			continue
		}
		removedAny = true
		if len(kept) == 0 {
			hooksObject.Delete(eventName)
		} else if err := hooksObject.Set(eventName, joinArray(kept)); err != nil {
			return err
		}
	}
	if !removedAny {
		return nil
	}
	if len(hooksObject.Keys()) == 0 {
		config.Delete(hooksKey)
		return nil
	}
	return setObject(config, hooksKey, hooksObject)
}

func onlyKeys(config *jsonedit.Object, allowed []string) bool {
	return !slices.ContainsFunc(config.Keys(), func(key string) bool { return !slices.Contains(allowed, key) })
}

// copyFiles prepares the plugin files of a recipe. A target that exists
// must hold our earlier copy (its hash is in the record) or the same bytes.
func (w *workspace) copyFiles(agent agents.Agent, prior AgentRecord) ([]CopiedFile, error) {
	var copied []CopiedFile
	for _, file := range agent.Install.Files {
		target, err := writablePath(w.home, w.dataDir, file.To)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(filepath.Join(agent.Dir, filepath.Clean(file.From)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file.From, err)
		}
		hash := sha256Hex(content)
		if existing, err := os.ReadFile(target); err == nil {
			existingHash := sha256Hex(existing)
			if existingHash != hash && !slices.Contains(prior.Files, CopiedFile{Path: file.To, SHA256: existingHash}) {
				return nil, fmt.Errorf("%s exists and is not our copy; move it away and try again", target)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", target, err)
		}
		w.copies = append(w.copies, fileWrite{path: target, content: content})
		copied = append(copied, CopiedFile{Path: file.To, SHA256: hash})
	}
	for _, old := range prior.Files {
		if !slices.ContainsFunc(copied, func(file CopiedFile) bool { return file.Path == old.Path }) {
			if err := w.removeCopy(old); err != nil {
				return nil, err
			}
		}
	}
	return copied, nil
}

// removeCopy deletes a copied file, but only while it still holds our copy.
func (w *workspace) removeCopy(file CopiedFile) error {
	target, err := writablePath(w.home, w.dataDir, file.Path)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	if sha256Hex(content) == file.SHA256 {
		w.removals = append(w.removals, target)
	}
	return nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// removeAgent prepares removing everything a record entry lists. The
// record path is checked with the same safety rules as a recipe, so an
// edited record cannot point the removal anywhere else.
func (w *workspace) removeAgent(entry AgentRecord) error {
	if err := CheckRecipe(w.home, w.dataDir, recipeOf(entry)); err != nil {
		return fmt.Errorf("install record of %s: %w", entry.ID, err)
	}
	if err := w.removeHooks(entry, nil); err != nil {
		return err
	}
	for _, file := range entry.Files {
		if err := w.removeCopy(file); err != nil {
			return err
		}
	}
	return nil
}

// recipeOf rebuilds the parts of a recipe that a record entry wrote.
func recipeOf(entry AgentRecord) agents.Recipe {
	var recipe agents.Recipe
	if entry.File != "" {
		recipe.Hooks = &agents.HookRecipe{File: entry.File, Format: formatJSON, Shape: entry.Shape, Events: entry.Events}
	}
	if entry.StatuslineKey != "" {
		recipe.Statusline = &agents.StatuslineRecipe{Key: entry.StatuslineKey}
	}
	for _, file := range entry.Files {
		recipe.Files = append(recipe.Files, agents.FileCopy{From: "x", To: file.Path})
	}
	return recipe
}

// AddAgentHooks adds the hooks, status line and files of one agent and
// records them in rec, which is saved before the first user file changes.
func AddAgentHooks(home, bin string, a agents.Agent, rec *Record) error {
	if rec.Bin == "" {
		rec.Bin = bin
	}
	w := newWorkspace(home, rec.Bin)
	prior, _ := rec.agentRecord(a.ID)
	entry, err := w.addAgent(bin, a, prior)
	if err != nil {
		return err
	}
	if _, err := w.edits(); err != nil {
		return err
	}
	updated := *rec
	updated.Agents = slices.Clone(rec.Agents)
	updated.setAgent(entry)
	if err := SaveRecord(paths.ForHome(home).InstallRecord, updated); err != nil {
		return err
	}
	*rec = updated
	return w.write()
}

// RemoveAgentHooks removes what the record lists for one agent, and
// nothing else, then drops the agent from the record. The manifest is not
// used: it may have changed or be gone.
func RemoveAgentHooks(home string, a agents.Agent, rec *Record) error {
	entry, ok := rec.agentRecord(a.ID)
	if !ok {
		return nil
	}
	w := newWorkspace(home, rec.Bin)
	if err := w.removeAgent(entry); err != nil {
		return err
	}
	if err := w.write(); err != nil {
		return err
	}
	updated := *rec
	updated.Agents = slices.Clone(rec.Agents)
	updated.dropAgent(a.ID)
	if err := SaveRecord(paths.ForHome(home).InstallRecord, updated); err != nil {
		return err
	}
	*rec = updated
	return nil
}

// PlanAgent lists what AddAgentHooks will do for one agent. Lines about an
// outside agent's own programs and plugin files carry Warning: the CLI and
// the settings window must show them and ask for explicit confirmation.
func PlanAgent(home, bin string, a agents.Agent, rec Record) ([]PlannedStep, error) {
	if err := CheckRecipe(home, paths.ForHome(home).DataDir, a.Install); err != nil {
		return nil, fmt.Errorf("%s: %w", a.ID, err)
	}
	origin := "built in"
	if !a.Builtin {
		origin = "from " + a.ID + " " + a.Version
	}
	verb := "Add"
	if _, ok := rec.agentRecord(a.ID); ok {
		verb = "Update"
	}
	name := "hooks-" + a.Source
	steps := []PlannedStep{{Name: name, Description: fmt.Sprintf("%s hooks for %s (%s)", verb, a.Name, origin), Warning: !a.Builtin}}
	if hooks := a.Install.Hooks; hooks != nil {
		path, _ := expandHome(home, hooks.File)
		for _, command := range distinctCommands(bin, a, hooks.Events) {
			steps = append(steps, PlannedStep{Name: name, Description: fmt.Sprintf("Add to %s for %s: %s (backup: %s%s)",
				path, strings.Join(command.events, ", "), command.command, path, backupSuffix)})
		}
		if a.Install.Statusline != nil {
			steps = append(steps, PlannedStep{Name: "statusline-" + a.Source, Description: fmt.Sprintf(
				"Point the %s status line in %s at agent-screensaver (your command is saved and still runs)", a.Name, path)})
		}
	}
	for _, file := range a.Install.Files {
		target, _ := expandHome(home, file.To)
		steps = append(steps, PlannedStep{Name: name, Warning: !a.Builtin,
			Description: fmt.Sprintf("Copy a plugin file that %s will run: %s", a.Name, target)})
	}
	for _, program := range converterPrograms(a) {
		steps = append(steps, PlannedStep{Name: name, Warning: !a.Builtin,
			Description: fmt.Sprintf("%s runs its own program %s on each %s event", a.ID, program, a.Name)})
	}
	return steps, nil
}

type plannedCommand struct {
	command string
	events  []string
}

func distinctCommands(bin string, a agents.Agent, events []string) []plannedCommand {
	var planned []plannedCommand
	for _, eventName := range events {
		command := agentHookCommand(bin, a, eventName)
		index := slices.IndexFunc(planned, func(item plannedCommand) bool { return item.command == command })
		if index < 0 {
			planned = append(planned, plannedCommand{command: command})
			index = len(planned) - 1
		}
		planned[index].events = append(planned[index].events, eventName)
	}
	return planned
}

// converterPrograms lists the agent's own programs, relative to its folder.
func converterPrograms(a agents.Agent) []string {
	programs := Programs(a)
	for index, program := range programs {
		if relative, err := filepath.Rel(a.Dir, program); err == nil {
			programs[index] = relative
		}
	}
	return programs
}
