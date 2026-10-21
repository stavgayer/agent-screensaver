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
	if got.Theme != "builtin.default" || !got.Sound || got.SoundFile != "/System/Library/Sounds/Glass.aiff" || got.Privacy || got.MaxFPS != 0 {
		t.Errorf("unexpected defaults: %+v", got)
	}
}

func ptr(text string) *string { return &text }
