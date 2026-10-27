package textdiff

import (
	"fmt"
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		want     string
	}{
		{
			name: "one changed line keeps three lines of context",
			old:  "a\nb\nc\nd\ne\nf\ng\nh\n",
			new:  "a\nb\nc\nd\nE\nf\ng\nh\n",
			want: "--- a/x\n+++ b/x\n@@ -2,7 +2,7 @@\n b\n c\n d\n-e\n+E\n f\n g\n h\n",
		},
		{
			name: "an added file diffs against /dev/null",
			old:  "",
			new:  "one\ntwo\n",
			want: "--- /dev/null\n+++ b/x\n@@ -0,0 +1,2 @@\n+one\n+two\n",
		},
		{
			name: "a removed file diffs to /dev/null",
			old:  "one\n",
			new:  "",
			want: "--- a/x\n+++ /dev/null\n@@ -1 +0,0 @@\n-one\n",
		},
		{
			name: "far apart changes make two hunks",
			old:  "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n",
			new:  "X\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\nY\n",
			want: "--- a/x\n+++ b/x\n@@ -1,4 +1,4 @@\n-1\n+X\n 2\n 3\n 4\n@@ -9,4 +9,4 @@\n 9\n 10\n 11\n-12\n+Y\n",
		},
		{
			name: "an inserted line in the middle",
			old:  "a\nb\n",
			new:  "a\nnew\nb\n",
			want: "--- a/x\n+++ b/x\n@@ -1,2 +1,3 @@\n a\n+new\n b\n",
		},
		{
			name: "a missing final newline is marked",
			old:  "a\n",
			new:  "a",
			want: "--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+a\n\\ No newline at end of file\n",
		},
		{
			name: "same text has no diff",
			old:  "a\nb\n",
			new:  "a\nb\n",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldName, newName := "a/x", "b/x"
			if tt.old == "" {
				oldName = "/dev/null"
			}
			if tt.new == "" {
				newName = "/dev/null"
			}

			got, truncated := Unified(oldName, newName, []byte(tt.old), []byte(tt.new), 1000)

			if got != tt.want {
				t.Errorf("diff =\n%s\nwant\n%s", got, tt.want)
			}
			if truncated {
				t.Error("a short diff was truncated")
			}
		})
	}
}

func TestUnifiedCapsItsLength(t *testing.T) {
	// Given a 500-line rewrite
	var old, new strings.Builder
	for line := range 500 {
		fmt.Fprintf(&old, "old %d\n", line)
		fmt.Fprintf(&new, "new %d\n", line)
	}

	// When the diff is capped at 20 lines
	got, truncated := Unified("a/x", "b/x", []byte(old.String()), []byte(new.String()), 20)

	// Then it stops at 20 lines and says so
	if !truncated {
		t.Error("not marked truncated")
	}
	if lines := strings.Count(got, "\n"); lines != 20 {
		t.Errorf("%d lines, want 20", lines)
	}
}

// TestEditsAreMinimal checks the edit script against a brute-force LCS on
// small inputs: the number of kept lines must be the longest possible.
func TestEditsAreMinimal(t *testing.T) {
	cases := [][2]string{
		{"abcabba", "cbabac"},
		{"", "abc"},
		{"abc", ""},
		{"aaaa", "aa"},
		{"abcdef", "fedcba"},
		{"xaxbxc", "abc"},
	}
	for _, pair := range cases {
		oldLines, newLines := strings.Split(pair[0], ""), strings.Split(pair[1], "")
		if pair[0] == "" {
			oldLines = nil
		}
		if pair[1] == "" {
			newLines = nil
		}
		edits := diffLines(oldLines, newLines)
		kept, rebuiltOld, rebuiltNew := 0, "", ""
		for _, e := range edits {
			switch e.kind {
			case keep:
				kept++
				rebuiltOld += e.text
				rebuiltNew += e.text
			case remove:
				rebuiltOld += e.text
			case insert:
				rebuiltNew += e.text
			}
		}
		if rebuiltOld != pair[0] || rebuiltNew != pair[1] {
			t.Errorf("%q -> %q: edits rebuild %q -> %q", pair[0], pair[1], rebuiltOld, rebuiltNew)
		}
		if want := lcsLength(oldLines, newLines); kept != want {
			t.Errorf("%q -> %q: kept %d lines, the longest common part is %d", pair[0], pair[1], kept, want)
		}
	}
}

func TestDiffLinesGivesUpOnHugeRewritesButStaysCorrect(t *testing.T) {
	// Given two long texts with nothing in common
	oldLines, newLines := make([]string, 3000), make([]string, 3000)
	for index := range oldLines {
		oldLines[index] = fmt.Sprintf("o%d", index)
		newLines[index] = fmt.Sprintf("n%d", index)
	}

	// When they are compared
	edits := diffLines(oldLines, newLines)

	// Then every old line is removed and every new one inserted
	removed, inserted := 0, 0
	for _, e := range edits {
		switch e.kind {
		case remove:
			removed++
		case insert:
			inserted++
		case keep:
			t.Fatal("kept a line that is not common")
		}
	}
	if removed != 3000 || inserted != 3000 {
		t.Errorf("removed %d, inserted %d", removed, inserted)
	}
}

func lcsLength(a, b []string) int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	return table[0][0]
}
