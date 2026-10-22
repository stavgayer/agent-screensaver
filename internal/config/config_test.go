package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		contents *string // nil means no file
		want     Config
	}{
		{"default when file missing", nil, Default()},
		{"broken file falls back to default", ptr("{not json"), Default()},
		{"an empty ignore list stays empty", ptr(`{"ignored_projects":[]}`), func() Config {
			cfg := Default()
			cfg.IgnoredProjects = []string{}
			return cfg
		}()},
		{"missing fields keep defaults", ptr(`{"privacy":true}`), func() Config {
			cfg := Default()
			cfg.Privacy = true
			return cfg
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tt.contents != nil {
				if err := os.WriteFile(path, []byte(*tt.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := Load(path); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSaveThenLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Default()
	want.Theme = "acme.night"
	want.Options = map[string]map[string]any{"builtin.clock": {"format": "12h"}}
	want.MaxFPS = 30
	want.PreviousStatusline = "echo hi"

	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := Load(path); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp*"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestDefault(t *testing.T) {
	got := Default()
	if got.Theme != "builtin.default" || !got.Sound || got.SoundFile != "/System/Library/Sounds/Glass.aiff" || got.Privacy || got.MaxFPS != 0 ||
		!reflect.DeepEqual(got.IgnoredProjects, []string{"observer-sessions"}) {
		t.Errorf("unexpected defaults: %+v", got)
	}
}

func ptr(text string) *string { return &text }

func TestLoadStrict(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(dir, "partial.json")
	if err := os.WriteFile(partial, []byte(`{"privacy": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		path    string
		wantErr bool
		privacy bool
	}{
		{"missing file gives defaults", filepath.Join(dir, "absent.json"), false, false},
		{"invalid JSON is an error", broken, true, false},
		{"unreadable path is an error", dir, true, false},
		{"partial file keeps other defaults", partial, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadStrict(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && (cfg.Privacy != tt.privacy || cfg.Theme != Default().Theme) {
				t.Errorf("cfg = %+v", cfg)
			}
		})
	}
}

func TestIgnoresProject(t *testing.T) {
	tests := []struct {
		name    string
		ignored []string
		project string
		want    bool
	}{
		{"default ignores the claude-mem observer", Default().IgnoredProjects, "observer-sessions", true},
		{"default keeps other projects", Default().IgnoredProjects, "web-app", false},
		{"folder name matches exactly", []string{"scratch"}, "scratch", true},
		{"a path matches by its base name", []string{"/Users/me/tmp/scratch/"}, "scratch", true},
		{"matching is case sensitive", []string{"Scratch"}, "scratch", false},
		{"a prefix is not a match", []string{"scratch"}, "scratch-2", false},
		{"no project never matches", []string{"scratch"}, "", false},
		{"empty list ignores nothing", nil, "observer-sessions", false},
		{"blank entry matches nothing", []string{"  "}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{IgnoredProjects: tt.ignored}
			if got := cfg.IgnoresProject(tt.project); got != tt.want {
				t.Errorf("IgnoresProject(%q) = %v, want %v", tt.project, got, tt.want)
			}
		})
	}
}

func TestProjectName(t *testing.T) {
	tests := []struct{ entry, want string }{
		{"scratch", "scratch"},
		{"  scratch  ", "scratch"},
		{"/Users/me/scratch/", "scratch"},
		{"~/work/api", "api"},
		{"", ""},
		{"   ", ""},
		{"/", ""},
		{".", ""},
	}
	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			if got := ProjectName(tt.entry); got != tt.want {
				t.Errorf("ProjectName(%q) = %q, want %q", tt.entry, got, tt.want)
			}
		})
	}
}
