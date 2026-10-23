package install

import (
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
)

// deniedPaths are the places no recipe may write, below home: shell start
// files, keys and credentials, and start-at-login items (spec §7d "Safety
// rules for recipes"). A trailing slash marks a whole folder.
var deniedPaths = []string{
	"Library/LaunchAgents/", "Library/Keychains/", ".ssh/", ".gnupg/", ".aws/", ".config/gh/",
	".gitconfig", ".zshrc", ".zprofile", ".zshenv", ".bashrc", ".bash_profile", ".profile",
	".config/fish/",
}

// eventNamePattern keeps an event name safe to write unquoted after
// --event in a shell command.
var eventNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// allowedKeys are the only keys a recipe may add to our handler or group,
// so a manifest can never pick the command or make our hook block the tool.
const (
	keyMatcher = "matcher"
	keyTimeout = "timeout"
	maxTimeout = 30
)

// CheckRecipe applies the safety rules of spec §7d to a recipe: every path
// it names stays in the user's home, outside the deny list and our data
// folder, the hook file is JSON, and our handler gets only harmless keys.
// It runs when an agent extension is installed and before every apply.
func CheckRecipe(home, dataDir string, r agents.Recipe) error {
	if r.Hooks == nil && len(r.Files) == 0 {
		return errors.New("the recipe has no hooks and no files")
	}
	if r.Detect != "" {
		if _, err := expandHome(home, r.Detect); err != nil {
			return fmt.Errorf("detect: %w", err)
		}
	}
	if r.Hooks != nil {
		if err := checkHookRecipe(home, dataDir, *r.Hooks); err != nil {
			return fmt.Errorf("hooks: %w", err)
		}
	}
	if r.Statusline != nil && (r.Statusline.Key == "" || r.Statusline.Key == hooksKey) {
		return fmt.Errorf("statusline key %q is not allowed", r.Statusline.Key)
	}
	for _, file := range r.Files {
		if _, err := writablePath(home, dataDir, file.To); err != nil {
			return fmt.Errorf("files: %w", err)
		}
	}
	return nil
}

func checkHookRecipe(home, dataDir string, hooks agents.HookRecipe) error {
	switch hooks.Format {
	case formatJSON:
	case "toml":
		return errors.New("toml is not supported yet")
	default:
		return fmt.Errorf("unknown format %q", hooks.Format)
	}
	if !strings.HasSuffix(hooks.File, ".json") {
		return fmt.Errorf("file %q must end in .json", hooks.File)
	}
	if _, err := writablePath(home, dataDir, hooks.File); err != nil {
		return err
	}
	if _, ok := hookShapes[hooks.Shape]; !ok {
		return fmt.Errorf("unknown shape %q", hooks.Shape)
	}
	if len(hooks.Group) > 0 && hooks.Shape != shapeGrouped {
		return errors.New("group keys need the grouped shape")
	}
	for _, eventName := range hooks.Events {
		if !eventNamePattern.MatchString(eventName) {
			return fmt.Errorf("event name %q may hold only letters, digits and . _ : -", eventName)
		}
	}
	for name, raw := range map[string]json.RawMessage{"group": hooks.Group, "handler": hooks.Handler} {
		if err := checkExtraKeys(raw); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return checkNewFile(hooks.NewFile)
}

// checkExtraKeys allows only matcher (a string) and timeout (1 to 30).
func checkExtraKeys(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	object, err := jsonedit.Parse(raw)
	if err != nil {
		return err
	}
	for _, key := range object.Keys() {
		value, _ := object.Get(key)
		switch key {
		case keyMatcher:
			var matcher string
			if json.Unmarshal(value, &matcher) != nil {
				return errors.New("matcher must be a string")
			}
		case keyTimeout:
			var seconds float64
			if json.Unmarshal(value, &seconds) != nil || seconds < 1 || seconds > maxTimeout {
				return fmt.Errorf("timeout must be a number from 1 to %d", maxTimeout)
			}
		default:
			return fmt.Errorf("key %q is not allowed (only matcher and timeout)", key)
		}
	}
	return nil
}

func checkNewFile(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	object, err := jsonedit.Parse(raw)
	if err != nil {
		return fmt.Errorf("new_file: %w", err)
	}
	if slices.Contains(object.Keys(), hooksKey) {
		return errors.New("new_file may not set hooks")
	}
	return nil
}

// expandHome turns a "~/" path into a path under home. Other forms and
// paths with ".." are refused.
func expandHome(home, path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok || rest == "" {
		return "", fmt.Errorf("path %q must start with ~/", path)
	}
	if slices.Contains(strings.Split(rest, "/"), "..") {
		return "", fmt.Errorf("path %q must not hold ..", path)
	}
	return filepath.Join(home, filepath.FromSlash(rest)), nil
}

// writablePath expands a recipe path and checks that writing it stays in
// home after symlinks are resolved, outside our data folder and the deny
// list.
func writablePath(home, dataDir, path string) (string, error) {
	full, err := expandHome(home, path)
	if err != nil {
		return "", err
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	resolved, err := resolveExisting(full)
	if err != nil {
		return "", err
	}
	relative, ok := within(realHome, resolved)
	if !ok {
		return "", fmt.Errorf("path %q leads outside the home folder", path)
	}
	if realData, err := resolveExisting(dataDir); err == nil {
		if _, inside := within(realData, resolved); inside {
			return "", fmt.Errorf("path %q is inside the agent-screensaver data folder", path)
		}
	}
	plain, _ := within(home, full)
	for _, candidate := range []string{relative, plain} {
		if denied(candidate) {
			return "", fmt.Errorf("path %q is a protected place", path)
		}
	}
	return full, nil
}

func denied(relative string) bool {
	slashed := filepath.ToSlash(relative)
	return slices.ContainsFunc(deniedPaths, func(entry string) bool {
		if folder, isFolder := strings.CutSuffix(entry, "/"); isFolder {
			return slashed == folder || strings.HasPrefix(slashed, entry)
		}
		return slashed == entry
	})
}

// resolveExisting resolves the symlinks of the longest part of path that
// exists and joins the rest, so a link anywhere on the way is followed.
func resolveExisting(path string) (string, error) {
	missing := ""
	current := filepath.Clean(path)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, missing), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve %s: %w", path, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path, nil
		}
		missing = filepath.Join(filepath.Base(current), missing)
		current = parent
	}
}

// within reports whether path is inside dir, and the part below dir.
func within(dir, path string) (string, bool) {
	relative, err := filepath.Rel(dir, path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relative, true
}
