package usage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"time"
)

// TokenWeights says how much one token of each kind counts toward a limit,
// relative to one input token. Anthropic does not publish how its limits
// weigh tokens; these are the price ratios of the current models, which
// is the best public guess. Calibration (see Estimator) turns weighted
// tokens into percent, so only the ratios between kinds matter.
type TokenWeights struct {
	Input, Output, CacheCreation, CacheRead float64
}

// DefaultWeights: output costs 5 inputs, a cache write 1.25, a cache read 0.1.
var DefaultWeights = TokenWeights{Input: 1, Output: 5, CacheCreation: 1.25, CacheRead: 0.1}

type tokenUsage struct {
	Input         float64 `json:"input_tokens"`
	Output        float64 `json:"output_tokens"`
	CacheCreation float64 `json:"cache_creation_input_tokens"`
	CacheRead     float64 `json:"cache_read_input_tokens"`
}

func (w TokenWeights) weigh(usage tokenUsage) float64 {
	return usage.Input*w.Input + usage.Output*w.Output + usage.CacheCreation*w.CacheCreation + usage.CacheRead*w.CacheRead
}

// Tally sums the tokens of every model call in the Claude Code transcripts
// (~/.claude/projects/**/*.jsonl, subagent files included). It remembers
// how far it read each file, so a poll reads only the lines added since.
// It is not safe for use by more than one goroutine.
type Tally struct {
	FS      fs.FS
	Root    string // the projects folder inside FS
	Weights TokenWeights

	files map[string]int64 // bytes read of each file, up to a whole line
	calls map[string]call  // by message id and request id
	// readBytes counts every byte read, so tests can check nothing is
	// read twice.
	readBytes int64
}

// call is one model call. A streamed answer is written several times with
// the same ids and growing usage; a resumed session copies old calls into
// its new file. Both count once.
type call struct {
	atMs     int64
	weighted float64
}

// Scan reads what was added to the transcripts since the last scan, at
// most budget bytes, so a first scan of a large history is spread over
// several polls. Calls at or before sinceMs are dropped, and files not
// changed since then are not opened. It reports whether every file was
// read to its end.
func (t *Tally) Scan(sinceMs, budget int64) (bool, error) {
	if t.files == nil {
		t.files = map[string]int64{}
		t.calls = map[string]call{}
	}
	t.forgetBefore(sinceMs)
	seen := map[string]bool{}
	caughtUp := true
	left := budget
	err := fs.WalkDir(t.FS, t.Root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if name == t.Root && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return nil // an unreadable folder is skipped, the rest still counts
		}
		if entry.IsDir() || path.Ext(name) != ".jsonl" {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().UnixMilli() <= sinceMs {
			return nil
		}
		seen[name] = true
		offset := t.files[name]
		if info.Size() < offset {
			offset = 0 // the file was rewritten
		}
		if info.Size() == offset {
			t.files[name] = offset
			return nil
		}
		if left <= 0 {
			caughtUp = false
			return nil
		}
		// A line longer than the whole budget may be read at once, but
		// only with a fresh budget, so one scan stays near its budget.
		before := t.readBytes
		read, done, err := t.readFile(name, offset, info.Size(), left, left == budget, sinceMs)
		left -= t.readBytes - before
		if err != nil {
			return nil // a file that cannot be read now is tried next scan
		}
		t.files[name] = read
		caughtUp = caughtUp && done
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("scan transcripts: %w", err)
	}
	for name := range t.files {
		if !seen[name] {
			delete(t.files, name)
		}
	}
	return caughtUp, nil
}

// Between returns the weighted tokens of the calls after fromMs, up to and
// including toMs.
func (t *Tally) Between(fromMs, toMs int64) float64 {
	var total float64
	for _, found := range t.calls {
		if found.atMs > fromMs && found.atMs <= toMs {
			total += found.weighted
		}
	}
	return total
}

// windowStart returns when the window open at nowMs began, given that none
// was open at closedAtMs. A window opens at the first call after the last
// one closed and stays open for lengthMs, which is how Claude's limit
// windows work. It reports false when no window is open at nowMs.
func (t *Tally) windowStart(closedAtMs, lengthMs, nowMs int64) (int64, bool) {
	var times []int64
	for _, found := range t.calls {
		if found.atMs > closedAtMs && found.atMs <= nowMs {
			times = append(times, found.atMs)
		}
	}
	if len(times) == 0 {
		return 0, false
	}
	slices.Sort(times)
	start := times[0]
	for _, at := range times[1:] {
		if at >= start+lengthMs {
			start = at
		}
	}
	return start, nowMs < start+lengthMs
}

func (t *Tally) forgetBefore(sinceMs int64) {
	for key, found := range t.calls {
		if found.atMs <= sinceMs {
			delete(t.calls, key)
		}
	}
}

// readFile reads whole lines of one file from offset, at most budget bytes
// unless a single line is longer and longLine allows it. It returns the new offset and whether the
// file was read to its end. A last line without its newline is still being
// written: it is left for the next scan.
func (t *Tally) readFile(name string, offset, size, budget int64, longLine bool, sinceMs int64) (int64, bool, error) {
	file, err := t.FS.Open(name)
	if err != nil {
		return offset, false, fmt.Errorf("open transcript: %w", err)
	}
	defer file.Close()
	reader, ok := file.(io.ReaderAt)
	if !ok {
		return offset, false, fmt.Errorf("transcript %s cannot be read at an offset", name)
	}
	chunk, err := t.readAt(reader, offset, min(size-offset, budget))
	if err != nil {
		return offset, false, err
	}
	end := bytes.LastIndexByte(chunk, '\n')
	if end < 0 && offset+int64(len(chunk)) < size {
		if !longLine {
			return offset, false, nil
		}
		if chunk, err = t.readAt(reader, offset, size-offset); err != nil {
			return offset, false, err
		}
		end = bytes.LastIndexByte(chunk, '\n')
	}
	done := offset+int64(len(chunk)) >= size
	if end < 0 {
		return offset, done, nil
	}
	for _, line := range bytes.Split(chunk[:end], []byte("\n")) {
		t.addLine(line, sinceMs)
	}
	return offset + int64(end) + 1, done, nil
}

func (t *Tally) addLine(line []byte, sinceMs int64) {
	// Cheap test first: most lines are prompts and tool results.
	if !bytes.Contains(line, []byte(`"usage"`)) || !bytes.Contains(line, []byte(`"assistant"`)) {
		return
	}
	var entry struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		RequestID string `json:"requestId"`
		Message   struct {
			ID    string      `json:"id"`
			Usage *tokenUsage `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" || entry.Message.Usage == nil || entry.Message.ID == "" {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
	if err != nil || at.UnixMilli() <= sinceMs {
		return
	}
	key := entry.Message.ID + ":" + entry.RequestID
	found := call{atMs: at.UnixMilli(), weighted: t.Weights.weigh(*entry.Message.Usage)}
	if old, ok := t.calls[key]; ok {
		// Keep the fullest copy, at the time the call was first seen.
		found.atMs = min(found.atMs, old.atMs)
		found.weighted = max(found.weighted, old.weighted)
	}
	t.calls[key] = found
}

func (t *Tally) readAt(reader io.ReaderAt, offset, length int64) ([]byte, error) {
	chunk := make([]byte, length)
	read, err := reader.ReadAt(chunk, offset)
	t.readBytes += int64(read)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read transcript: %w", err)
	}
	return chunk[:read], nil
}
