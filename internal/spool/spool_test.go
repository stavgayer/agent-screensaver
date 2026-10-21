package spool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"agent-screensaver/internal/event"
)

func sampleEvent(session string, atMs int64) event.Event {
	return event.Event{V: 1, Source: "claude", Session: session, Kind: event.KindStarted, AtMs: atMs}
}

func fileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestWriteThenDrainOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	want := sampleEvent("s1", 1_760_000_000_000)
	if err := Write(dir, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %v, err = %v, want 0700", info.Mode().Perm(), err)
	}

	got := Drain(dir)
	if len(got) != 1 || got[0].Session != "s1" || got[0].AtMs != want.AtMs {
		t.Fatalf("first Drain = %+v, want one event for s1", got)
	}
	if again := Drain(dir); len(again) != 0 {
		t.Fatalf("second Drain = %+v, want empty", again)
	}
	if names := fileNames(t, dir); len(names) != 0 {
		t.Fatalf("files left after Drain: %v", names)
	}
}

func TestWriteFileMode(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, sampleEvent("s1", 1_760_000_000_000)); err != nil {
		t.Fatal(err)
	}
	names := fileNames(t, dir)
	if len(names) != 1 || !strings.HasSuffix(names[0], ".json") {
		t.Fatalf("files = %v, want one .json", names)
	}
	info, err := os.Stat(filepath.Join(dir, names[0]))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, err = %v, want 0600", info.Mode().Perm(), err)
	}
}

func TestDrainOrdersByTime(t *testing.T) {
	dir := t.TempDir()
	for _, tt := range []struct {
		session string
		atMs    int64
	}{{"late", 1_760_000_000_300}, {"early", 1_760_000_000_100}, {"middle", 1_760_000_000_200}} {
		if err := Write(dir, sampleEvent(tt.session, tt.atMs)); err != nil {
			t.Fatal(err)
		}
	}

	var order []string
	for _, e := range Drain(dir) {
		order = append(order, e.Session)
	}
	if strings.Join(order, ",") != "early,middle,late" {
		t.Fatalf("order = %v, want early,middle,late", order)
	}
}

func TestDrainIgnoresTmpAndDeletesGarbage(t *testing.T) {
	dir := t.TempDir()
	valid := `{"v":1,"source":"claude","session":"ok","kind":"started","at_ms":1}`
	oversize := `{"v":1,"source":"` + strings.Repeat("a", event.MaxInputBytes) + `"}`
	files := map[string]string{
		"1-1-1.json.tmp": valid,
		"2-1-1.json":     "not json",
		"3-1-1.json":     `{"v":2,"source":"claude","session":"s","kind":"started"}`,
		"4-1-1.json":     oversize,
		"5-1-1.json":     valid,
		"notes.txt":      "hello",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	got := Drain(dir)
	if len(got) != 1 || got[0].Session != "ok" {
		t.Fatalf("Drain = %+v, want only the valid event", got)
	}
	names := strings.Join(fileNames(t, dir), ",")
	if names != "1-1-1.json.tmp,notes.txt" {
		t.Fatalf("remaining files = %s, want only the .tmp and notes.txt", names)
	}
}

func TestDrainMissingDirIsEmpty(t *testing.T) {
	if got := Drain(filepath.Join(t.TempDir(), "nope")); len(got) != 0 {
		t.Fatalf("Drain = %+v, want empty", got)
	}
}

func TestConcurrentWritersAndDrainersExactlyOnce(t *testing.T) {
	const writers = 50
	dir := t.TempDir()

	var mu sync.Mutex
	seen := map[string]int{}
	record := func(events []event.Event) {
		mu.Lock()
		defer mu.Unlock()
		for _, e := range events {
			seen[e.Session]++
		}
	}

	var writing, draining sync.WaitGroup
	for i := range writers {
		writing.Add(1)
		go func() {
			defer writing.Done()
			if err := Write(dir, sampleEvent(fmt.Sprintf("s%d", i), 1_760_000_000_000+int64(i))); err != nil {
				t.Errorf("Write: %v", err)
			}
		}()
	}
	done := make(chan struct{})
	for range 2 {
		draining.Add(1)
		go func() {
			defer draining.Done()
			for {
				record(Drain(dir))
				select {
				case <-done:
					return
				default:
				}
			}
		}()
	}
	writing.Wait()
	close(done)
	draining.Wait()
	record(Drain(dir))

	if len(seen) != writers {
		t.Fatalf("got %d distinct events, want %d", len(seen), writers)
	}
	for session, count := range seen {
		if count != 1 {
			t.Errorf("event %s returned %d times, want 1", session, count)
		}
	}
}
