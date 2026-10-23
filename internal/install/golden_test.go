package install

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// updateGolden rewrites testdata/golden from the current code. The files in
// git were written by the Go adapters before Task 6.4; the agent recipe
// engine must write the same bytes.
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden")

// homeMarker stands for the temp home in the golden files.
const homeMarker = "$HOME"

func TestInstallGoldenFiles(t *testing.T) {
	tests := []struct {
		name    string
		claude  string // fixture written before install; "" means no file
		codex   string
		outputs map[string]func(string) string
	}{
		{"missing files", "", "", map[string]func(string) string{
			"missing_claude_settings.json": claudePath, "missing_codex_hooks.json": codexPath}},
		{"user files", "claude_settings.json", "codex_hooks.json", map[string]func(string) string{
			"user_claude_settings.json": claudePath, "user_codex_hooks.json": codexPath,
			"user_claude_settings.json.bak": func(home string) string { return claudePath(home) + backupSuffix },
			"user_codex_hooks.json.bak":     func(home string) string { return codexPath(home) + backupSuffix }}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.claude != "" {
				writeFile(t, claudePath(home), readFixture(t, tt.claude))
			}
			if tt.codex != "" {
				writeFile(t, codexPath(home), readFixture(t, tt.codex))
			}
			apply(t, home, &fakeServices{})
			for golden, path := range tt.outputs {
				got, err := os.ReadFile(path(home))
				if err != nil {
					t.Fatal(err)
				}
				got = bytes.ReplaceAll(got, []byte(home), []byte(homeMarker))
				goldenPath := filepath.Join("testdata", "golden", golden)
				if *updateGolden {
					writeFile(t, goldenPath, got)
					continue
				}
				want, err := os.ReadFile(goldenPath)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s differs from %s:\n%s", path(home), goldenPath, got)
				}
			}
		})
	}
}
