package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// The test folder is on the system volume, which is APFS on every supported Mac.
func TestCloneFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mov")
	if err := os.WriteFile(source, []byte("video bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing.mov")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		src     string
		dst     string
		wantErr bool
	}{
		{"clones a file", source, filepath.Join(dir, "clone.mov"), false},
		{"missing source fails", filepath.Join(dir, "missing.mov"), filepath.Join(dir, "never.mov"), true},
		{"existing destination fails", source, existing, true},
		{"missing destination folder fails", source, filepath.Join(dir, "no", "clone.mov"), true},
	}
	cloner := NewFileCloner()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cloner.Clone(tt.src, tt.dst)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Clone error = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			data, err := os.ReadFile(tt.dst)
			if err != nil || string(data) != "video bytes" {
				t.Errorf("clone holds %q (err %v), want the source bytes", data, err)
			}
		})
	}
	if data, _ := os.ReadFile(existing); string(data) != "old" {
		t.Errorf("existing file changed to %q", data)
	}
}
