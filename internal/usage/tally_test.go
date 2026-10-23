package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	dayMs     = 24 * 3_600_000
	bigBudget = 1 << 30
)

// Weighted tokens of the fixture calls (input 1, output 5, cache write
// 1.25, cache read 0.1):
const (
	msg1 = 10 + 100*5 + 1000*1.25 + 10000*0.1 // final copy of a streamed answer
	msg2 = 2 + 200*5 + 400*1.25 + 20000*0.1
	msg3 = 100 + 40*5 // the subagent's answer
)

func fixtureTally() *Tally {
	return &Tally{FS: os.DirFS("testdata"), Root: "projects", Weights: DefaultWeights}
}

func TestTallyBetween(t *testing.T) {
	tally := fixtureTally()
	caughtUp, err := tally.Scan(at10-dayMs, bigBudget)
	if err != nil || !caughtUp {
		t.Fatalf("Scan = %v, %v; want caught up", caughtUp, err)
	}
	tests := []struct {
		name     string
		from, to int64
		want     float64
	}{
		{"each call once: streamed copies and a resumed copy in another file count once", at10, at11, msg1 + msg2 + msg3},
		{"only calls after the start", at10 + 15*60_000, at11, msg2 + msg3},
		{"calls at the end are in, calls at the start are out", at10 + 20*60_000, at10 + 40*60_000, msg2},
		{"calls before the scan horizon were never kept", at10 - 20*dayMs, at10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tally.Between(tt.from, tt.to); got != tt.want {
				t.Errorf("Between = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWeightsTable(t *testing.T) {
	tests := []struct {
		name  string
		usage tokenUsage
		want  float64
	}{
		{"input", tokenUsage{Input: 10}, 10},
		{"output costs five inputs", tokenUsage{Output: 10}, 50},
		{"cache write costs a quarter more", tokenUsage{CacheCreation: 100}, 125},
		{"cache read costs a tenth", tokenUsage{CacheRead: 100}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultWeights.weigh(tt.usage); got != tt.want {
				t.Errorf("weigh = %v, want %v", got, tt.want)
			}
		})
	}
}

func assistantLine(id string, at time.Time, output int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"requestId":"req_%s","message":{"id":"msg_%s","usage":{"input_tokens":0,"output_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`+"\n",
		at.UTC().Format(time.RFC3339Nano), id, id, output)
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// Each scan reads only what was added; a line still being written waits
// for its newline; a rewritten (shorter) file is read again from the start.
func TestTallyReadsIncrementally(t *testing.T) {
	root := t.TempDir()
	transcript := filepath.Join(root, "projects", "p", "s.jsonl")
	tally := &Tally{FS: os.DirFS(root), Root: "projects", Weights: DefaultWeights}
	since := at10 - dayMs
	first := assistantLine("a", time.UnixMilli(at10), 1)
	appendTo(t, transcript, first)
	scan := func(step string) {
		t.Helper()
		if caughtUp, err := tally.Scan(since, bigBudget); err != nil || !caughtUp {
			t.Fatalf("%s: Scan = %v, %v", step, caughtUp, err)
		}
	}
	scan("first")
	if tally.readBytes != int64(len(first)) {
		t.Fatalf("first scan read %d bytes, want %d", tally.readBytes, len(first))
	}

	second := assistantLine("b", time.UnixMilli(at10+60_000), 2)
	appendTo(t, transcript, second[:20])
	scan("half a line")
	if got := tally.Between(since, at11); got != 5 {
		t.Errorf("after half a line: %v, want 5 (only the first call)", got)
	}
	appendTo(t, transcript, second[20:])
	scan("rest of the line")
	if got := tally.Between(since, at11); got != 15 {
		t.Errorf("after the whole line: %v, want 15", got)
	}
	// Only the unfinished half line was read twice.
	if want := int64(len(first) + 20 + len(second)); tally.readBytes != want {
		t.Errorf("read %d bytes in all, want %d", tally.readBytes, want)
	}

	// A shorter file was rewritten: read from the start again. Calls seen
	// before keep counting once.
	rewritten := assistantLine("c", time.UnixMilli(at10+120_000), 3)
	if err := os.WriteFile(transcript, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}
	scan("rewritten")
	if got := tally.Between(since, at11); got != 30 {
		t.Errorf("after the rewrite: %v, want 30", got)
	}
}

func TestTallyScanKeepsToItsBudget(t *testing.T) {
	root := t.TempDir()
	var lines []string
	for index := range 40 {
		line := assistantLine(fmt.Sprint(index), time.UnixMilli(at10+int64(index)*1000), 1)
		appendTo(t, filepath.Join(root, "projects", "p", fmt.Sprintf("s%d.jsonl", index%4)), line)
		lines = append(lines, line)
	}
	budget := int64(len(lines[0]) * 3)
	tally := &Tally{FS: os.DirFS(root), Root: "projects", Weights: DefaultWeights}
	scans := 0
	for {
		before := tally.readBytes
		caughtUp, err := tally.Scan(at10-dayMs, budget)
		if err != nil {
			t.Fatal(err)
		}
		scans++
		if read := tally.readBytes - before; read > budget {
			t.Fatalf("scan %d read %d bytes, budget %d", scans, read, budget)
		}
		if caughtUp {
			break
		}
		if scans > 100 {
			t.Fatal("never caught up")
		}
	}
	if scans < 10 {
		t.Errorf("caught up in %d scans; the budget was not applied", scans)
	}
	if got := tally.Between(at10-1, at11); got != 40*5 {
		t.Errorf("Between = %v, want %v", got, 40*5)
	}
}

func TestTallyForgetsCallsBeforeTheHorizon(t *testing.T) {
	tally := fixtureTally()
	if _, err := tally.Scan(at10-dayMs, bigBudget); err != nil {
		t.Fatal(err)
	}
	// The next scan moves the horizon past the first answers.
	if _, err := tally.Scan(at10+30*60_000, bigBudget); err != nil {
		t.Fatal(err)
	}
	if got := tally.Between(at10-dayMs, at11); got != msg2 {
		t.Errorf("Between = %v, want only the call after the horizon (%v)", got, msg2)
	}
}

func TestTallyWithoutTranscripts(t *testing.T) {
	tally := &Tally{FS: os.DirFS(t.TempDir()), Root: "projects", Weights: DefaultWeights}
	caughtUp, err := tally.Scan(at10, bigBudget)
	if err != nil || !caughtUp || tally.Between(0, at11) != 0 {
		t.Errorf("Scan = %v, %v; want caught up with nothing counted", caughtUp, err)
	}
}

// After the first read, a poll of a 50 MiB transcript set where one answer
// was added costs well under 50 ms.
func TestTallyPollAfterFirstReadIsCheap(t *testing.T) {
	if testing.Short() {
		t.Skip("writes 50 MiB")
	}
	root := t.TempDir()
	filler := strings.Repeat("x", 1000)
	userLine := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":%q},"timestamp":"2026-10-23T10:00:00.000Z"}`+"\n", filler)
	var chunk strings.Builder
	for chunk.Len() < 1<<20 {
		chunk.WriteString(userLine)
		chunk.WriteString(assistantLine(fmt.Sprint(chunk.Len()), time.UnixMilli(at10), 1))
	}
	for index := range 50 {
		appendTo(t, filepath.Join(root, "projects", fmt.Sprintf("p%d", index%5), fmt.Sprintf("s%d.jsonl", index)), chunk.String())
	}
	tally := &Tally{FS: os.DirFS(root), Root: "projects", Weights: DefaultWeights}
	for {
		caughtUp, err := tally.Scan(at10-dayMs, 64<<20)
		if err != nil {
			t.Fatal(err)
		}
		if caughtUp {
			break
		}
	}
	appendTo(t, filepath.Join(root, "projects", "p0", "s0.jsonl"), assistantLine("new", time.UnixMilli(at11), 7))
	start := time.Now()
	if _, err := tally.Scan(at10-dayMs, 64<<20); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 50*time.Millisecond {
		t.Errorf("poll took %v, want under 50ms", took)
	}
	if got := tally.Between(at10, at11); got != 35 {
		t.Errorf("new answer counted as %v, want 35", got)
	}
}
