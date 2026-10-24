package extensions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// MaxMarkBytes caps an agent's mark file.
const MaxMarkBytes = 8 << 10

var sourcePattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// agentBasics are the parts of an agent body that this package checks: the
// event source the agent writes and the mark its cards show.
type agentBasics struct {
	Source string `json:"source"`
	Mark   string `json:"mark"`
}

func validateAgent(m Manifest, dir string) error {
	if m.Entry != "" || len(m.Options) > 0 {
		return errors.New("an agent extension has no entry and no options")
	}
	if len(m.Agent) == 0 {
		return errors.New("agent is required for an agent extension")
	}
	var basics agentBasics
	if err := json.Unmarshal(m.Agent, &basics); err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	if !sourcePattern.MatchString(basics.Source) {
		return fmt.Errorf("agent.source %q must be 1 to 32 lowercase letters, digits or dashes", basics.Source)
	}
	if basics.Mark == "" {
		return errors.New("agent.mark is required")
	}
	if !filepath.IsLocal(basics.Mark) {
		return fmt.Errorf("agent.mark %q must be a relative path inside the extension folder", basics.Mark)
	}
	return CheckMark(filepath.Join(dir, filepath.Clean(basics.Mark)))
}

// AgentSource is the event source an agent extension writes, or "" for
// another type.
func AgentSource(m Manifest) string {
	if m.Type != TypeAgent {
		return ""
	}
	var basics agentBasics
	if json.Unmarshal(m.Agent, &basics) != nil {
		return ""
	}
	return basics.Source
}

// markRules are the things a mark may not hold. The runtime shows the mark
// with <img src>, which runs no script and loads nothing; this check is the
// second line of defense.
var markRules = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`(?i)<\s*script`), "a <script> element"},
	{regexp.MustCompile(`(?i)<\s*foreignobject`), "a <foreignObject> element"},
	{regexp.MustCompile(`(?i)[\s"'/]on[a-z]+\s*=`), "an on... attribute"},
	{regexp.MustCompile(`(?i)href\s*=\s*["']?\s*[^"'\s#]`), "a link that does not start with #"},
	{regexp.MustCompile(`(?i)url\(\s*["']?\s*[^"'\s#)]`), "a url( that does not start with url(#"},
	{regexp.MustCompile(`(?i)@import`), "an @import"},
}

var svgRoot = regexp.MustCompile(`(?i)^(?:\s|<\?xml[^>]*\?>|<!--.*?-->)*<svg[\s>]`)

// CheckMark checks an agent's mark file: a regular file (not a symlink) of
// at most MaxMarkBytes with an <svg> root and none of the parts in
// markRules, also after character references such as &#40; are decoded.
func CheckMark(path string) error {
	if info, err := os.Lstat(path); err != nil {
		return fmt.Errorf("mark: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("mark %s is a symlink", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("mark: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("mark: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("mark %s is not a regular file", path)
	}
	if info.Size() > MaxMarkBytes {
		return fmt.Errorf("mark is %d bytes, limit is %d", info.Size(), MaxMarkBytes)
	}
	content, err := io.ReadAll(io.LimitReader(file, MaxMarkBytes+1))
	if err != nil {
		return fmt.Errorf("mark: %w", err)
	}
	if !svgRoot.Match(bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})) {
		return errors.New("mark must be an SVG file with an <svg> root")
	}
	decoded := []byte(html.UnescapeString(string(content)))
	for _, rule := range markRules {
		if rule.pattern.Match(content) || rule.pattern.Match(decoded) {
			return fmt.Errorf("mark holds %s", rule.reason)
		}
	}
	return nil
}

// checkUniqueSource refuses a new agent whose source another installed
// agent already writes. An upgrade of the same id may keep its source.
func checkUniqueSource(m Manifest, extDir string) error {
	source := AgentSource(m)
	if source == "" {
		return nil
	}
	for _, installed := range Scan(extDir) {
		if installed.Manifest.ID != m.ID && AgentSource(installed.Manifest) == source {
			return fmt.Errorf("agent source %q is already used by %s", source, installed.Manifest.ID)
		}
	}
	return nil
}
