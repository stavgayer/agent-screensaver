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

// Patterns are compiled once. Go's RE2 has no lookarounds, so context
// to keep is captured and written back with ${n} references.
var (
	bearerPattern     = regexp.MustCompile(`(?i)(bearer\s+)\S+`)
	assignmentPattern = regexp.MustCompile(`(?i)\b(token|key|secret|password|passwd|pwd|api[_-]?key)\b(\s*[:=]\s*|\s+)\S+`)
	longRunPattern    = regexp.MustCompile(`[A-Za-z0-9+/_\-]{32,}`)
)

// MaskSecrets replaces likely secrets with a mask, keeping the surrounding
// context (the "Bearer" word, the key name and separator) readable.
func MaskSecrets(s string) string {
	s = bearerPattern.ReplaceAllString(s, "${1}"+mask)
	s = assignmentPattern.ReplaceAllString(s, "${1}${2}"+mask)
	return longRunPattern.ReplaceAllString(s, mask)
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
