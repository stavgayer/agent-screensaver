package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name     string
		existing string // "" means no file yet
	}{
		{"new file in a missing folder", ""},
		{"replaces an old file", "old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sub", "file.json")
			if tt.existing != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := Write(path, []byte("new"), 0o644); err != nil {
				t.Fatalf("Write: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "new" {
				t.Fatalf("content = %q, %v; want new", data, err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o644 {
				t.Errorf("mode = %v, %v; want 0644", info.Mode().Perm(), err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil || len(entries) != 1 {
				t.Errorf("folder holds %d entries (%v), want only the file", len(entries), err)
			}
		})
	}
}
