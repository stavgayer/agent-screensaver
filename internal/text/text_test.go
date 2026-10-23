package text

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		secret string // must be gone from the output; empty means output must equal input
	}{
		{"bearer token", "curl -H 'Authorization: Bearer abc123def456'", "abc123def456"},
		{"env assignment", "API_KEY=sk-live-XYZ npm start", "sk-live-XYZ"},
		{"flag", "--password hunter2", "hunter2"},
		{"colon", "token: ghp_abcdefghijklmnopqrstuvwxyz0123456789", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"},
		{"long random string", "deploy with AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/ now", "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/"},
		{"short colon value", "token: abc", "abc"},
		{"db password", "DB_PASSWORD=hunter2", "hunter2"},
		{"github token", "GITHUB_TOKEN=ghp_short1", "ghp_short1"},
		{"aws secret", "AWS_SECRET_ACCESS_KEY=abcd1234", "abcd1234"},
		{"openai key", "OPENAI_API_KEY=sk-short", "sk-short"},
		{"access token", "access_token=abc123", "abc123"},
		{"client secret", "client_secret=abc123", "abc123"},
		{"passphrase", "passphrase hunter2", "hunter2"},
		{"plural secrets", "secrets=abc", "abc"},
		{"plural tokens", "tokens abc", "abc"},
		{"json key", `{"password": "hunter2"}`, "hunter2"},
		{"basic auth", "Authorization: Basic dXNlcjpwYXNz", "dXNlcjpwYXNz"},
		{"url credentials", "postgres://user:hunter2@db/x", "hunter2"},
		{"curl user", "curl -u admin:hunter2 host", "hunter2"},
		{"aws access key id", "id AKIAIOSFODNN7EXAMPLE here", "AKIAIOSFODNN7EXAMPLE"},
		{"aws temp key id", "id ASIAIOSFODNN7EXAMPLE here", "ASIAIOSFODNN7EXAMPLE"},
		{"normal word kept", "implementation", ""},
		{"dash p not masked", "mysql -p hunter2", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskSecrets(tt.input)
			if tt.secret == "" {
				if got != tt.input {
					t.Fatalf("MaskSecrets(%q) = %q, want unchanged", tt.input, got)
				}
				return
			}
			if strings.Contains(got, tt.secret) {
				t.Fatalf("secret %q still present in %q", tt.secret, got)
			}
			if !strings.Contains(got, "•••") {
				t.Fatalf("mask missing in %q", got)
			}
		})
	}
}

func TestMaskSecretsKeepsIdentifiers(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"mcp tool name", "mcp__engram__mem_session_summary", "mcp__engram__mem_session_summary"},
		{"long snake_case name", "update_customer_billing_address_record_v2", "update_customer_billing_address_record_v2"},
		{"tool name in title", "Tool: mcp__engram__mem_session_summary", "Tool: mcp__engram__mem_session_summary"},
		{"random key with underscores still masked", "k_9fA3Zq8LmN2xR7vB1cD5eG0hJ4kP6sT", mask},
		{"mixed case long run still masked", "Mcp__Engram__Mem_Session_Summary_Value", mask},
		{"hex hash still masked", "0123456789abcdef0123456789abcdef", mask},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskSecrets(tt.input); got != tt.want {
				t.Fatalf("MaskSecrets(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaskSecretsKeepsContext(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"bearer keeps word", "Bearer abc123", "Bearer •••"},
		{"assignment keeps key and separator", "API_KEY=sk-live-XYZ npm start", "API_KEY=••• npm start"},
		{"flag keeps name", "--password hunter2", "--password •••"},
		{"prefixed name kept", "DB_PASSWORD=hunter2 run", "DB_PASSWORD=••• run"},
		{"url keeps user and host", "postgres://user:hunter2@db/x", "postgres://user:•••@db/x"},
		{"curl keeps user", "curl -u admin:hunter2 host", "curl -u admin:••• host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskSecrets(tt.input); got != tt.want {
				t.Fatalf("MaskSecrets(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestShortTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"cuts at word end", "Refactor the authentication module so it uses the new session store", "Refactor the authentication module so…"},
		{"first line only", "\n\n  Fix the login bug  \nand then more text", "Fix the login bug"},
		{"short input unchanged", "Fix typo", "Fix typo"},
		{"collapses whitespace", "Fix   the\ttypo", "Fix the typo"},
		{"masks before cutting", "export TOKEN=supersecretvalue", "export TOKEN=•••"},
		{"empty input", "  \n ", ""},
		{"mcp tool name kept", "mcp__engram__mem_session_summary", "mcp__engram__mem_session_summary"},
		{"long tool name cut after masking", "mcp__engram__mem_session_summary: save the whole session notes now", "mcp__engram__mem_session_summary: save…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortTitle(tt.input); got != tt.want {
				t.Fatalf("ShortTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestShortTitleHebrewDoesNotSplitRunes(t *testing.T) {
	input := strings.Repeat("שלום עולם ", 10)
	got := ShortTitle(input)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n > TitleMax {
		t.Fatalf("title has %d runes, max %d", n, TitleMax)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis in %q", got)
	}
}

func TestShortTitleLongWordIsHardCut(t *testing.T) {
	got := ShortTitle(strings.Repeat("א", 100))
	if n := utf8.RuneCountInString(got); n != TitleMax {
		t.Fatalf("title has %d runes, want %d", n, TitleMax)
	}
}

func TestPromptTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal prompt", "Fix the login bug", "Fix the login bug"},
		{"normal prompt is cut at 40", "Refactor the authentication module so it uses the new session store", "Refactor the authentication module so…"},
		{"lone agent-message tag", `<agent-message from="a005f5d42630ca89f">`, ""},
		{"agent-message block with body", "<agent-message from=\"a005f5d42630ca89f\">\nplease continue\n</agent-message>", ""},
		{"lone task-notification tag", "<task-notification>", ""},
		{"task-notification block", "<task-notification>\n<task-id>x1</task-id>\n<status>done</status>\n</task-notification>", ""},
		{"only a tag block", "<system-reminder>\nUse the skill.\n</system-reminder>", ""},
		{"tag block then human text", "<system-reminder>\nUse the skill.\n</system-reminder>\nFix the login bug", "Fix the login bug"},
		{"several tag blocks then human text", "<command-name>/clear</command-name>\n<local-command-stdout></local-command-stdout>\n\nAdd a retry", "Add a retry"},
		{"tag block and text on one line", "<system-reminder>x</system-reminder> Fix it", "Fix it"},
		{"banner only", "--- MODE SWITCH: PROGRESS SUMMARY ---", ""},
		{"banner then human text", "--- MODE SWITCH: PROGRESS SUMMARY ---\nSummarize what changed", "Summarize what changed"},
		{"plain dashes line", "-----\nFix it", "Fix it"},
		{"tag then banner then text", "<system-reminder>x</system-reminder>\n--- NOTE ---\nFix it", "Fix it"},
		{"unclosed tag with text is the user's", "<Button> does not render", "<Button> does not render"},
		{"less-than sign is not a tag", "<3 this app", "<3 this app"},
		{"dash in a sentence is kept", "--force is not working", "--force is not working"},
		{"quoted file mentions show the file name", `@"/Users/me/Desktop/IMG_7324.HEIC" @"/Users/me/Desktop/IMG_7325.HEIC"`, "IMG_7324.HEIC IMG_7325.HEIC"},
		{"quoted path with spaces", `@"/Users/me/My Photos/beach day.png" what is this`, "beach day.png what is this"},
		{"plain file mention", "look at @src/app/main.go please", "look at main.go please"},
		{"address with a path is not a file mention", "mail me@host.com/path now", "mail me@host.com/path now"},
		{"prompt wrapped in one tag gives no title", "<command-name>/clear</command-name>", ""},
		{"inline tag after human text is kept", "fix <Button> please", "fix <Button> please"},
		{"handle without slash is kept", "ask @alice about it", "ask @alice about it"},
		{"mention and secret", "@/tmp/notes.txt TOKEN=abc123", "notes.txt TOKEN=•••"},
		{"secret is masked before the cut", "export TOKEN=supersecretvalue and then do many more things after", "export TOKEN=••• and then do many more…"},
		{"empty", "  \n ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PromptTitle(tt.input); got != tt.want {
				t.Fatalf("PromptTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPromptTitleIsFastOnManyLoneTags(t *testing.T) {
	prompt := strings.Repeat("<a>\n", 256*1024) // 1 MB, no closing tag
	start := time.Now()
	PromptTitle(prompt)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("PromptTitle took %v on 1 MB of lone tags, want well under 100 ms", elapsed)
	}
}

func TestPromptTitleIsFastOnOneHugeLine(t *testing.T) {
	start := time.Now()
	got := PromptTitle(strings.Repeat("word ", 200*1024))
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("PromptTitle took %v on 1 MB single line", elapsed)
	}
	if want := "word word word word word word word word…"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestShortModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"claude id", "claude-opus-5-5", "Opus 5.5"},
		{"claude id with a window tag", "claude-opus-5-5[1m]", "Opus 5.5"},
		{"claude id with a date", "claude-sonnet-4-5-20250929", "Sonnet 4.5"},
		{"older claude id order", "claude-3-5-haiku-20241022", "Haiku 3.5"},
		{"display name with a note", "Opus 5.5 (1M context)", "Opus 5.5"},
		{"display name", "Sonnet 4.5", "Sonnet 4.5"},
		{"display name with the brand", "Claude 3.5 Sonnet", "3.5 Sonnet"},
		{"codex model stays as it is", "gpt-5.6", "gpt-5.6"},
		{"codex model with a suffix", "gpt-5.6-codex", "gpt-5.6-codex"},
		{"spaces trimmed", "  gpt-5  ", "gpt-5"},
		{"empty", "", ""},
		{"only a note", "(1M context)", ""},
		{"long name cut", strings.Repeat("m", 40), strings.Repeat("m", 23) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortModel(tt.model); got != tt.want {
				t.Errorf("ShortModel(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}
