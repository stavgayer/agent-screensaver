package tray

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-screensaver/internal/statefile"
)

func writeState(t *testing.T, path string, file statefile.StateFile, modTime time.Time) {
	t.Helper()
	if err := statefile.WriteAtomic(path, file); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func TestStateReader(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name string
		// each step changes the folder, then reads once
		steps []func(t *testing.T, path string)
		want  []int64 // GeneratedAtMs seen after each step
	}{
		{
			name: "missing file reads as empty",
			steps: []func(*testing.T, string){
				func(*testing.T, string) {},
			},
			want: []int64{0},
		},
		{
			name: "same mtime is not read again",
			steps: []func(*testing.T, string){
				func(t *testing.T, path string) { writeState(t, path, statefile.StateFile{GeneratedAtMs: 1}, start) },
				// new content, same mtime: a re-read would show 2
				func(t *testing.T, path string) { writeState(t, path, statefile.StateFile{GeneratedAtMs: 2}, start) },
			},
			want: []int64{1, 1},
		},
		{
			name: "newer mtime is read",
			steps: []func(*testing.T, string){
				func(t *testing.T, path string) { writeState(t, path, statefile.StateFile{GeneratedAtMs: 1}, start) },
				func(t *testing.T, path string) {
					writeState(t, path, statefile.StateFile{GeneratedAtMs: 2}, start.Add(time.Second))
				},
			},
			want: []int64{1, 2},
		},
		{
			name: "bad JSON keeps the last good file",
			steps: []func(*testing.T, string){
				func(t *testing.T, path string) { writeState(t, path, statefile.StateFile{GeneratedAtMs: 1}, start) },
				func(t *testing.T, path string) {
					if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(path, start.Add(time.Second), start.Add(time.Second)); err != nil {
						t.Fatal(err)
					}
				},
			},
			want: []int64{1, 1},
		},
		{
			name: "removal resets to empty",
			steps: []func(*testing.T, string){
				func(t *testing.T, path string) { writeState(t, path, statefile.StateFile{GeneratedAtMs: 1}, start) },
				func(t *testing.T, path string) {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				},
			},
			want: []int64{1, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			reader := stateReader{path: path}
			for i, step := range tt.steps {
				step(t, path)
				if got := reader.read().GeneratedAtMs; got != tt.want[i] {
					t.Errorf("step %d: GeneratedAtMs = %d, want %d", i, got, tt.want[i])
				}
			}
		})
	}
}
