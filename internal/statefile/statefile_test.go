package statefile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agent-screensaver/internal/nowplaying"
	"agent-screensaver/internal/state"
	"agent-screensaver/internal/view"
)

func sampleState() state.State {
	return state.State{Cards: []state.Card{
		{Source: "claude", Session: "s1", Kind: state.KindRunning, Title: "Fix login", Request: "run rm", Project: "app",
			Name: "Login fix", Branch: "fix/login", Model: "Opus 5.5"},
	}}
}

func TestBuildPrivacyStripsTitles(t *testing.T) {
	tests := []struct {
		name      string
		privacy   bool
		wantTitle string
	}{
		{"privacy on", true, ""},
		{"privacy off", false, "Fix login"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sampleState()
			got := Build(input, view.View{Privacy: tt.privacy}, nil, 42)

			if got.Version != 1 || !got.ServiceOK || got.GeneratedAtMs != 42 {
				t.Fatalf("header = %+v", got)
			}
			if got.Cards[0].Title != tt.wantTitle {
				t.Fatalf("Title = %q, want %q", got.Cards[0].Title, tt.wantTitle)
			}
			// The conversation name and the branch name say what the work
			// is, as the title does.
			if card := got.Cards[0]; tt.privacy && (card.Request != "" || card.Name != "" || card.Branch != "") {
				t.Fatalf("request, name and branch = %q, %q, %q, want empty", card.Request, card.Name, card.Branch)
			}
			if got.Cards[0].Project != "app" || got.Cards[0].Model != "Opus 5.5" {
				t.Fatalf("project and model = %q, %q, want kept", got.Cards[0].Project, got.Cards[0].Model)
			}
			if input.Cards[0].Title != "Fix login" || input.Cards[0].Request != "run rm" {
				t.Fatalf("caller state was mutated: %+v", input.Cards[0])
			}
		})
	}
}

func TestBuildNeverEmitsNullLists(t *testing.T) {
	data, err := json.Marshal(Build(state.State{}, view.View{}, nil, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"cards":[]`) || !strings.Contains(string(data), `"limits":[]`) {
		t.Fatalf("json = %s, want empty arrays for cards and limits", data)
	}
}

func TestWriteAtomicLeavesNoTmp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	path := filepath.Join(dir, "state.json")
	want := Build(sampleState(), view.View{}, nil, 7)

	for range 2 { // second write replaces the first
		if err := WriteAtomic(path, want); err != nil {
			t.Fatalf("WriteAtomic: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got StateFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("state file does not parse: %v", err)
	}
	if got.GeneratedAtMs != 7 || len(got.Cards) != 1 {
		t.Fatalf("round trip = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("file mode = %v, err = %v, want 0644", info.Mode().Perm(), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("dir has %d entries, want only state.json", len(entries))
	}
}

func TestReadLimits(t *testing.T) {
	resets := int64(1_792_760_400)
	written := []state.Limit{
		{Source: "claude", Window: "5h", UsedPercent: 42, ResetsAt: &resets, UpdatedAtMs: 1000, Estimated: true},
		{Source: "codex", Window: "week", UsedPercent: 7, UpdatedAtMs: 2000},
	}
	tests := []struct {
		name    string
		content string // "" means no file
		want    []state.Limit
		wantErr bool
	}{
		{"limits of the last run come back", "write", written, false},
		{"no file yet is not an error", "", nil, false},
		{"a broken file is an error", "{", nil, true},
		{"a demo file gives back no limits", "demo", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.json")
			switch tt.content {
			case "":
			case "write", "demo":
				file := Build(state.State{Limits: written}, view.View{}, nil, 5000)
				file.Demo = tt.content == "demo"
				if err := WriteAtomic(path, file); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ReadLimits(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReadLimits error = %v, want error %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadLimits = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBuildShowsTheSongUnlessPrivacyIsOn(t *testing.T) {
	track := &nowplaying.Track{App: "Spotify", Title: "Sloop John B", Artist: "The Beach Boys", PositionMs: 1, DurationMs: 2, AtMs: 3}
	for _, privacy := range []bool{false, true} {
		got := Build(state.State{}, view.View{Privacy: privacy}, track, 42).NowPlaying
		if privacy && got != nil || !privacy && (got == nil || *got != *track || got == track) {
			t.Errorf("privacy %v: now_playing = %+v, want %v", privacy, got, map[bool]string{false: "a copy of the song", true: "none"}[privacy])
		}
	}
	data, err := json.Marshal(Build(state.State{}, view.View{}, nil, 1))
	if err != nil || strings.Contains(string(data), "now_playing") {
		t.Errorf("no song, the file holds now_playing: %s (%v)", data, err)
	}
}

func TestSessionNamesAreWrittenAndReadBackUnlessPrivacyIsOn(t *testing.T) {
	for _, privacy := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := WriteAtomic(path, Build(sampleState(), view.View{Privacy: privacy}, nil, 42)); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if privacy && strings.Contains(string(data), "Login fix") {
			t.Errorf("privacy on, state.json holds the name:\n%s", data)
		}
		names, err := ReadSessionNames(path)
		want := []state.SessionName{{Source: "claude", Session: "s1", Name: "Login fix"}}
		if privacy {
			want = nil
		}
		if err != nil || !reflect.DeepEqual(names, want) {
			t.Errorf("privacy %v: names = %+v, %v, want %+v", privacy, names, err, want)
		}
	}
}
