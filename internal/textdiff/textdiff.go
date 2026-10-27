// Package textdiff writes a git-style unified diff of two texts. It is
// small on purpose: Myers' line diff with a bound on the work, and hunks
// with three lines of context.
package textdiff

import (
	"fmt"
	"strings"
)

// Context is the number of unchanged lines shown around each change.
const Context = 3

// maxEdits bounds the Myers search. Past it the diff stops looking for
// common lines and shows the rest as all removed, then all added: still a
// correct diff, only a longer one. It keeps the time and the memory small
// for any input.
const maxEdits = 1000

type kind byte

const (
	keep   kind = ' '
	remove kind = '-'
	insert kind = '+'
)

type edit struct {
	kind kind
	text string // one line, with its "\n" unless it is the last line without one
}

// Unified returns the diff from old to new with "--- oldName" and
// "+++ newName" headers, or "" when the texts are the same. It holds at
// most maxLines lines; truncated says some were cut.
func Unified(oldName, newName string, old, new []byte, maxLines int) (diff string, truncated bool) {
	edits := diffLines(splitLines(string(old)), splitLines(string(new)))
	hunks := hunksOf(edits)
	if len(hunks) == 0 {
		return "", false
	}
	lines := []string{"--- " + oldName, "+++ " + newName}
	for _, h := range hunks {
		lines = append(lines, h.header())
		for _, e := range edits[h.from:h.to] {
			lines = append(lines, string(e.kind)+strings.TrimSuffix(e.text, "\n"))
			if !strings.HasSuffix(e.text, "\n") {
				lines = append(lines, `\ No newline at end of file`)
			}
		}
	}
	if len(lines) > maxLines {
		lines, truncated = lines[:maxLines], true
	}
	return strings.Join(lines, "\n") + "\n", truncated
}

// splitLines cuts text after each "\n". The last line has no "\n" when the
// text does not end with one.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// diffLines is the edit script from a to b. The common start and end are
// cut off first, so a small change in a long file costs little.
func diffLines(a, b []string) []edit {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	edits := make([]edit, 0, len(a)+len(b))
	for _, line := range a[:prefix] {
		edits = append(edits, edit{keep, line})
	}
	edits = append(edits, myers(a[prefix:len(a)-suffix], b[prefix:len(b)-suffix])...)
	for _, line := range a[len(a)-suffix:] {
		edits = append(edits, edit{keep, line})
	}
	return edits
}

// myers finds a shortest edit script (Myers 1986, the greedy forward
// search with a saved trace for the way back). When more than maxEdits
// edits are needed it removes all of a and inserts all of b.
func myers(a, b []string) []edit {
	n, m := len(a), len(b)
	limit := min(n+m, maxEdits)
	offset := limit + 1
	v := make([]int, 2*limit+3)
	var trace [][]int
	for d := 0; d <= limit; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1] // a step down: insert
			} else {
				x = v[offset+k-1] + 1 // a step right: remove
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrack(trace, offset, a, b)
			}
		}
	}
	edits := make([]edit, 0, n+m)
	for _, line := range a {
		edits = append(edits, edit{remove, line})
	}
	for _, line := range b {
		edits = append(edits, edit{insert, line})
	}
	return edits
}

func backtrack(trace [][]int, offset int, a, b []string) []edit {
	x, y := len(a), len(b)
	var reversed []edit
	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		k := x - y
		var previousK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			previousK = k + 1
		} else {
			previousK = k - 1
		}
		previousX := v[offset+previousK]
		previousY := previousX - previousK
		for x > previousX && y > previousY {
			reversed = append(reversed, edit{keep, a[x-1]})
			x, y = x-1, y-1
		}
		if d > 0 {
			if x == previousX {
				reversed = append(reversed, edit{insert, b[y-1]})
			} else {
				reversed = append(reversed, edit{remove, a[x-1]})
			}
		}
		x, y = previousX, previousY
	}
	edits := make([]edit, len(reversed))
	for index, e := range reversed {
		edits[len(reversed)-1-index] = e
	}
	return edits
}

// hunk is edits[from:to], which starts at line oldStart of the old text and
// newStart of the new one (both counted from 0).
type hunk struct {
	from, to           int
	oldStart, newStart int
	oldCount, newCount int
}

func (h hunk) header() string {
	return fmt.Sprintf("@@ -%s +%s @@", rangeText(h.oldStart, h.oldCount), rangeText(h.newStart, h.newCount))
}

// rangeText is git's "start,count": the start counts from 1, an empty range
// names the line before it, and a count of 1 is left out.
func rangeText(start, count int) string {
	switch count {
	case 0:
		return fmt.Sprintf("%d,0", start)
	case 1:
		return fmt.Sprintf("%d", start+1)
	default:
		return fmt.Sprintf("%d,%d", start+1, count)
	}
}

// hunksOf groups the changes with Context unchanged lines around each.
// Two changes closer than 2*Context unchanged lines share a hunk.
func hunksOf(edits []edit) []hunk {
	var changes []int
	for index, e := range edits {
		if e.kind != keep {
			changes = append(changes, index)
		}
	}
	var hunks []hunk
	for start := 0; start < len(changes); {
		end := start
		for end+1 < len(changes) && changes[end+1]-changes[end]-1 <= 2*Context {
			end++
		}
		hunks = append(hunks, makeHunk(edits, max(0, changes[start]-Context), min(len(edits), changes[end]+Context+1)))
		start = end + 1
	}
	return hunks
}

func makeHunk(edits []edit, from, to int) hunk {
	h := hunk{from: from, to: to}
	for _, e := range edits[:from] {
		if e.kind != insert {
			h.oldStart++
		}
		if e.kind != remove {
			h.newStart++
		}
	}
	for _, e := range edits[from:to] {
		if e.kind != insert {
			h.oldCount++
		}
		if e.kind != remove {
			h.newCount++
		}
	}
	return h
}
