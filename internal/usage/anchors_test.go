package usage

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"agent-screensaver/internal/event"
)

// Times of the fixtures, 23/10/2026 in UTC.
var (
	at10 = time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC).UnixMilli()
	at11 = time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC).UnixMilli()
	at12 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC).UnixMilli()
	at13 = time.Date(2026, 10, 23, 13, 0, 0, 0, time.UTC).UnixMilli()
)

func seconds(ms int64) *int64 {
	value := ms / 1000
	return &value
}

func fixtureFS(t *testing.T, names ...string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	for _, name := range names {
		data, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		fsys[name] = &fstest.MapFile{Data: data, ModTime: time.UnixMilli(at10)}
	}
	return fsys
}

func TestDesktopHistoryAnchors(t *testing.T) {
	tests := []struct {
		name    string
		fsys    fstest.MapFS
		want    []Anchor
		wantErr bool
	}{
		{
			name: "every sample with a 5h or week percent, oldest first; samples without them are skipped",
			fsys: fixtureFS(t, "plan-usage-history.json"),
			want: []Anchor{
				{Window: event.FiveHour, Percent: 10, AtMs: at10},
				{Window: event.Week, Percent: 40, AtMs: at10},
				{Window: event.FiveHour, Percent: 25, AtMs: at11},
				{Window: event.Week, Percent: 43, AtMs: at11},
			},
		},
		{
			name: "only the org of the newest sample counts",
			fsys: fstest.MapFS{"plan-usage-history.json": {Data: []byte(`{"samples":[` +
				`{"t":1,"org":"org-a","u":{"fh":10}},` +
				`{"t":2,"org":"org-b","u":{"fh":90,"sd":90}},` +
				`{"t":3,"org":"org-a","u":{"sd":40}},` +
				`{"t":4,"org":null,"u":{"fh":70}},` +
				`{"t":5,"org":"org-a","u":{"xu":0}}]}`)}},
			want: []Anchor{
				{Window: event.FiveHour, Percent: 10, AtMs: 1},
				{Window: event.Week, Percent: 40, AtMs: 3},
			},
		},
		{name: "no desktop app means no anchors", fsys: fstest.MapFS{}},
		{
			name:    "a broken file is an error",
			fsys:    fstest.MapFS{"plan-usage-history.json": {Data: []byte(`{"samples":[`)}},
			wantErr: true,
		},
		{
			name:    "a file over the size bound is not read",
			fsys:    fstest.MapFS{"plan-usage-history.json": {Data: []byte(strings.Repeat(" ", maxHistoryBytes+1))}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &DesktopHistory{FS: tt.fsys, Path: "plan-usage-history.json"}
			got, err := history.Anchors()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Anchors error = %v, want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Anchors =\n %+v\nwant\n %+v", got, tt.want)
			}
		})
	}
}

func TestCLICacheAnchors(t *testing.T) {
	tests := []struct {
		name string
		file string
		want []Anchor
	}{
		{
			name: "both windows with their reset times, at the fetch time",
			file: "claude.json",
			want: []Anchor{
				{Window: event.FiveHour, Percent: 30, ResetsAt: seconds(at13), AtMs: at12},
				{Window: event.Week, Percent: 45, ResetsAt: seconds(time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC).UnixMilli()), AtMs: at12},
			},
		},
		{name: "no cached usage", file: "claude_no_cache.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fixtureFS(t, tt.file)
			cache := &CLICache{FS: fsys, Path: tt.file}
			got, err := cache.Anchors()
			if err != nil {
				t.Fatalf("Anchors: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Anchors =\n %+v\nwant\n %+v", got, tt.want)
			}
		})
	}
}

// A file is parsed again only when its modification time or size changes.
func TestAnchorFilesAreParsedOncePerChange(t *testing.T) {
	fsys := fstest.MapFS{"h.json": {Data: []byte(`{"samples":[{"t":1,"u":{"fh":10}}]}`), ModTime: time.UnixMilli(at10)}}
	history := &DesktopHistory{FS: fsys, Path: "h.json"}
	first, err := history.Anchors()
	if err != nil || len(first) != 1 || first[0].Percent != 10 {
		t.Fatalf("first read = %+v, %v", first, err)
	}
	// Same size and time: the cached anchors stay.
	fsys["h.json"].Data = []byte(`{"samples":[{"t":1,"u":{"fh":20}}]}`)
	if again, _ := history.Anchors(); again[0].Percent != 10 {
		t.Errorf("unchanged stamp re-parsed: %+v", again)
	}
	fsys["h.json"].ModTime = time.UnixMilli(at11)
	if changed, _ := history.Anchors(); changed[0].Percent != 20 {
		t.Errorf("new stamp not parsed: %+v", changed)
	}
}

func TestNewForMacReadsTheProjectsFolder(t *testing.T) {
	estimator := NewForMac(fstest.MapFS{}, "/data/claude-usage.json")
	if estimator.Tally.Root != ".claude/projects" || estimator.StoreFile != "/data/claude-usage.json" || len(estimator.Sources) != 2 {
		t.Errorf("estimator = %+v", estimator)
	}
}

func TestMacSourcesUseClaudeFiles(t *testing.T) {
	sources := MacSources(fstest.MapFS{})
	var paths []string
	for _, source := range sources {
		switch reader := source.(type) {
		case *DesktopHistory:
			paths = append(paths, reader.Path)
		case *CLICache:
			paths = append(paths, reader.Path)
		}
	}
	want := []string{"Library/Application Support/Claude/plan-usage-history.json", ".claude.json"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}
