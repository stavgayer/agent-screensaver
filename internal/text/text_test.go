package text

import (
	"strings"
	"testing"
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
