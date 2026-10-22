// Package text turns prompts and commands into short, screen-safe titles.
package text

import (
	"regexp"
	"strings"
)

// TitleMax is the longest title in runes, including the ellipsis.
const TitleMax = 40

const (
	mask     = "•••"
	ellipsis = "…"
)

// maskRule rewrites matches with a replacement that keeps the context
// (capture groups) and hides only the secret.
type maskRule struct {
	pattern     *regexp.Regexp
	replacement string
}

// Rules are compiled once and applied in order. Go's RE2 has no lookarounds,
// so context to keep is captured and written back with ${n}. Over-masking is
// the accepted failure: a secret on screen is not.
var maskRules = []maskRule{
	// Authorization: Bearer <token>
	{regexp.MustCompile(`(?i)(bearer\s+)\S+`), "${1}" + mask},
	// Authorization: Basic <base64 credentials>
	{regexp.MustCompile(`(?i)(basic\s+)\S+`), "${1}" + mask},
	// scheme://user:password@host
	{regexp.MustCompile(`(://[^\s:/@]+:)[^\s@/]+@`), "${1}" + mask + "@"},
	// curl -u user:password
	{regexp.MustCompile(`(\s-u\s+[^\s:]+:)\S+`), "${1}" + mask},
	// AWS access key ids
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), mask},
	// name=value, name: value, --name value, "name": "value", where the name
	// contains a secret-like word (DB_PASSWORD, access_token, secrets, ...)
	{regexp.MustCompile(`(?i)([A-Za-z0-9_\-]*(?:token|key|secret|password|passwd|passphrase|pwd)[A-Za-z0-9_\-]*)(["']?\s*[:=]\s*["']?|\s+)\S+`), "${1}${2}" + mask},
}

// longRun finds any long unbroken token-like run (API keys, hashes).
var longRun = regexp.MustCompile(`[A-Za-z0-9+/_\-]{32,}`)

// identifier is a lowercase snake_case name such as the tool name
// mcp__engram__mem_session_summary: words that start with a letter, joined by
// one or two underscores. Random keys do not look like this.
var identifier = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_{1,2}[a-z][a-z0-9]*)+$`)

// MaskSecrets replaces likely secrets with a mask, keeping the surrounding
// context (the "Bearer" word, the key name and separator) readable.
func MaskSecrets(s string) string {
	for _, rule := range maskRules {
		s = rule.pattern.ReplaceAllString(s, rule.replacement)
	}
	return longRun.ReplaceAllStringFunc(s, func(run string) string {
		if identifier.MatchString(run) {
			return run
		}
		return mask
	})
}

// ShortTitle returns the first non-empty line of s, whitespace collapsed,
// secrets masked, and cut to TitleMax runes at a word end.
func ShortTitle(s string) string {
	line := strings.Join(strings.Fields(MaskSecrets(firstNonEmptyLine(s))), " ")
	return truncateAtWord([]rune(line), TitleMax)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

// truncateAtWord keeps the result, ellipsis included, within limit runes.
// Working on runes means a multi-byte character is never split.
func truncateAtWord(runes []rune, limit int) string {
	if len(runes) <= limit {
		return string(runes)
	}
	keep := limit - 1 // room for the ellipsis
	cut := keep
	if runes[keep] != ' ' { // the next rune continues a word: drop that word
		if space := lastSpace(runes[:keep]); space > 0 {
			cut = space
		}
	}
	return strings.TrimRight(string(runes[:cut]), " ") + ellipsis
}

func lastSpace(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			return i
		}
	}
	return -1
}
