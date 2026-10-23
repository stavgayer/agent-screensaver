package agents

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"agent-screensaver/internal/event"
)

func testConverter(t *testing.T, script string) Converter {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "converter", script))
	if err != nil {
		t.Fatal(err)
	}
	return Converter{Run: path, Timeout: DefaultConverterTimeout}
}

func TestConverter(t *testing.T) {
	approval := withSession(event.KindApproval, "s1")
	approval.Title = "export API_TOKEN=•••"
	finishedS2 := withSession(event.KindFinished, "s2")
	tests := []struct {
		name       string
		script     string
		payload    string
		wantTitles []string // titles of the events, in order
		wantEvents []event.Event
		wantErr    string
	}{
		{name: "prints one event", script: "one.sh", wantTitles: []string{"one"}},
		{name: "prints two lines gives two events", script: "two.sh", wantTitles: []string{"one", ""}},
		{name: "empty output means ignore", script: "empty.sh"},
		{name: "five lines keep only 4", script: "five.sh", wantTitles: []string{"", "", "", ""}},
		{name: "output over 64 KiB is dropped", script: "big.sh", wantErr: "64 KiB"},
		{name: "payload reaches standard input", script: "echo-input.sh", payload: "hello", wantTitles: []string{"hello"}},
		{name: "source in output is replaced and text masked", script: "other-source.sh", wantEvents: []event.Event{approval}},
		{name: "invalid event dropped", script: "invalid.sh", wantEvents: []event.Event{finishedS2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := Mapping{source: testSource, Converter: ptr(testConverter(t, tt.script))}

			got, err := mapping.Events(context.Background(), []byte(tt.payload), "", nil, testNowMs)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || len(got) != 0 {
					t.Fatalf("events %+v, error %v; want no events and %q", got, err, tt.wantErr)
				}
				return
			}
			if tt.wantEvents != nil {
				if !slices.EqualFunc(got, tt.wantEvents, eventsEqual) {
					t.Errorf("events:\n got  %+v\n want %+v", got, tt.wantEvents)
				}
				return
			}
			var titles []string
			for _, mapped := range got {
				titles = append(titles, mapped.Title)
				if mapped.Source != testSource || mapped.AtMs != testNowMs {
					t.Errorf("event %+v lacks the forced source or time", mapped)
				}
			}
			if !slices.Equal(titles, tt.wantTitles) {
				t.Errorf("titles = %q, want %q", titles, tt.wantTitles)
			}
		})
	}
}

func TestConverterTimeout(t *testing.T) {
	mapping := Mapping{source: testSource, Converter: ptr(testConverter(t, "sleep.sh"))}
	start := time.Now()

	got, err := mapping.Events(context.Background(), nil, "", nil, testNowMs)

	if elapsed := time.Since(start); elapsed > 600*time.Millisecond {
		t.Errorf("took %v, want under 600 ms", elapsed)
	}
	if len(got) != 0 || err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("events %+v, error %v", got, err)
	}
}

func TestConverterEnvironment(t *testing.T) {
	home := t.TempDir()
	env := ConverterEnv(home, "/usr/bin:/bin", testSource, "session.idle")

	if _, err := RunConverter(context.Background(), testConverter(t, "env.sh"), nil, env); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		name, _, _ := strings.Cut(line, "=")
		names = append(names, name)
	}
	slices.Sort(names)
	// sh adds PWD (and some shells SHLVL or _) itself; nothing else of ours may leak.
	names = slices.DeleteFunc(names, func(name string) bool { return name == "PWD" || name == "SHLVL" || name == "_" })
	if want := []string{"AGS_EVENT", "AGS_SOURCE", "HOME", "PATH"}; !slices.Equal(names, want) {
		t.Errorf("environment = %v, want %v", names, want)
	}
	if !strings.Contains(string(data), "AGS_EVENT=session.idle") || !strings.Contains(string(data), "AGS_SOURCE=acme") {
		t.Errorf("environment values:\n%s", data)
	}
}

func TestConverterMustBeExecutable(t *testing.T) {
	dir, err := filepath.Abs(filepath.Join("testdata", "converter"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		run     string
		wantErr string
	}{
		{"executable file", "one.sh", ""},
		{"non-executable file refused", "not-executable.sh", "execute"},
		{"path outside the folder refused", "../converter/one.sh", "inside"},
		{"missing file refused", "nope.sh", "no such file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConverter(converterManifest{Run: tt.run}, dir, MaxConverterTimeout)
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func ptr[T any](value T) *T { return &value }

// eventsEqual compares the fields a converter can set.
func eventsEqual(a, b event.Event) bool {
	return a.V == b.V && a.Source == b.Source && a.Session == b.Session && a.Kind == b.Kind &&
		a.AtMs == b.AtMs && a.Title == b.Title && a.Project == b.Project && a.Name == b.Name
}
