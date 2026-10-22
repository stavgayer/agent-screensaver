// Package text turns prompts and commands into short, screen-safe titles.
package text

import (
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
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

// PromptTitle makes a card title from what the user typed. Tools add text
// to prompts that the user never wrote: tag blocks such as <system-reminder>
// or <task-notification>, and banner lines such as "--- MODE SWITCH ---".
// Those are skipped and the first human text is used. A file mention
// (@"/a/b/photo.png") shows only the file name. Secrets are masked before
// the cut, like in ShortTitle. The result is empty when nothing human is left.
func PromptTitle(prompt string) string {
	line := firstNonEmptyLine(firstHumanText(prompt))
	return ShortTitle(replaceFileMentions(cutRunes(line, maxTitleLineBytes)))
}

const (
	// Hooks must finish fast, so the work on a prompt is bounded: no more
	// than maxSkipSteps machine-made parts are skipped, a closing tag is
	// looked for only in the next maxTagScanBytes, and only the first
	// maxTitleLineBytes of the title line are read.
	maxSkipSteps      = 32
	maxTagScanBytes   = 4096
	maxTitleLineBytes = 4096
)

// cutRunes returns at most limit bytes of s, never ending inside a character.
func cutRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

var (
	// An opening tag at the start of the text, with attributes.
	openTag  = regexp.MustCompile(`^<([A-Za-z][A-Za-z0-9_:.-]*)(?:\s[^<>]*)?(/?)>`)
	closeTag = regexp.MustCompile(`^</[A-Za-z][A-Za-z0-9_:.-]*>`)
	banner   = regexp.MustCompile(`^(?:-{3,}|-{3,}.*-{3,})$`)
	// @"path with spaces", @'path', or @path that has a slash in it.
	quotedMention = regexp.MustCompile(`@(?:"([^"]+)"|'([^']+)')`)
	// Only at the start of the text or after a space, so an address such as
	// me@host.com/path is not a file mention.
	plainMention = regexp.MustCompile(`(?:^|\s)@(\S*/\S+)`)
)

// firstHumanText drops leading machine-made parts and returns the rest.
func firstHumanText(s string) string {
	for range maxSkipSteps {
		s = strings.TrimLeft(s, " \t\r\n")
		if rest, ok := skipTagBlock(s); ok {
			s = rest
		} else if rest, ok := skipBanner(s); ok {
			s = rest
		} else {
			return s
		}
	}
	return strings.TrimLeft(s, " \t\r\n")
}

// skipTagBlock removes <tag ...>...</tag>, a self-closing tag, a stray
// closing tag, or a lone opening tag on its own line. An opening tag with
// text after it and no closing tag (<Button> does not render) is the
// user's own words and is kept.
func skipTagBlock(s string) (string, bool) {
	if closing := closeTag.FindString(s); closing != "" {
		return s[len(closing):], true
	}
	match := openTag.FindStringSubmatch(s)
	if match == nil {
		return "", false
	}
	afterTag := s[len(match[0]):]
	if match[2] == "/" {
		return afterTag, true
	}
	closing := "</" + match[1] + ">"
	if end := strings.Index(cutRunes(afterTag, maxTagScanBytes), closing); end >= 0 {
		return afterTag[end+len(closing):], true
	}
	lineEnd := strings.IndexByte(afterTag, '\n')
	if lineEnd < 0 {
		lineEnd = len(afterTag)
	}
	if strings.TrimSpace(afterTag[:lineEnd]) == "" {
		return afterTag[lineEnd:], true
	}
	return "", false
}

// skipBanner removes a first line made of --- (alone or around a label).
func skipBanner(s string) (string, bool) {
	line, rest, _ := strings.Cut(s, "\n")
	if !banner.MatchString(strings.TrimSpace(line)) {
		return "", false
	}
	return rest, true
}

// replaceFileMentions turns @path into the file name. Real paths can be long
// and the secret mask would turn them into •••.
func replaceFileMentions(line string) string {
	line = quotedMention.ReplaceAllStringFunc(line, func(mention string) string {
		return path.Base(strings.Trim(mention[1:], `"'`))
	})
	return plainMention.ReplaceAllStringFunc(line, func(mention string) string {
		at := strings.IndexByte(mention, '@')
		return mention[:at] + path.Base(mention[at+1:])
	})
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
