package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWriteBelowTheLimitAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	if err := os.WriteFile(path, []byte("old line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := Open(path, 100, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	if _, err := log.Write([]byte("new line\n")); err != nil {
		t.Fatal(err)
	}

	if got := read(t, path); got != "old line\nnew line\n" {
		t.Errorf("log = %q", got)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf("rotated below the limit: %v", err)
	}
}

func TestALineThatPassesTheLimitGoesWholeToANewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 90)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("older run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var reopened []string
	log, err := Open(path, 100, func(file *os.File) error {
		reopened = append(reopened, file.Name())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	line := "this line passes the limit\n"
	if n, err := log.Write([]byte(line)); err != nil || n != len(line) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if _, err := log.Write([]byte("next\n")); err != nil {
		t.Fatal(err)
	}

	if got := read(t, path); got != line+"next\n" {
		t.Errorf("new log = %q, want the whole line and the next one", got)
	}
	if got := read(t, path+".1"); got != strings.Repeat("a", 90)+"\n" {
		t.Errorf("old log = %q, want the full file before rotation (the older one replaced)", got)
	}
	if len(reopened) != 1 || reopened[0] != path {
		t.Errorf("reopen hook calls = %v, want one for %s", reopened, path)
	}
}

func TestOnlyOneOldFileIsKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")
	log, err := Open(path, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	for _, line := range []string{"first 789\n", "second 89\n", "third 789\n"} {
		if _, err := log.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("files = %v, want daemon.log and daemon.log.1", entries)
	}
	if got := read(t, path); got != "third 789\n" {
		t.Errorf("log = %q", got)
	}
	if got := read(t, path+".1"); got != "second 89\n" {
		t.Errorf("old log = %q", got)
	}
}

func TestALineLongerThanTheLimitIsStillWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	log, err := Open(path, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	long := strings.Repeat("x", 50) + "\n"
	if _, err := log.Write([]byte(long)); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != long {
		t.Errorf("log = %q", got)
	}
}
