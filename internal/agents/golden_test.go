package agents

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agent-screensaver/internal/event"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden from the manifests")

// goldenEvents is what a fixture maps to, as written to testdata/golden.
type goldenEvents struct {
	Events []event.Event `json:"events"`
}

// TestBuiltinManifestsMatchGolden maps every fixture with the built-in
// manifests and compares the events with testdata/golden. The golden files
// were written by the Go adapters before they were removed (Task 6.4, the
// equivalence gate), so this keeps Claude and Codex exactly as before.
// Cursor never had a Go adapter: its golden files were written from its
// manifest (Task 6.5) and checked by hand, and now guard it the same way.
func TestBuiltinManifestsMatchGolden(t *testing.T) {
	cases := []struct {
		group, fixtures string
		mapping         func(t *testing.T) *Mapping
	}{
		{"claude", "claude", func(t *testing.T) *Mapping { agent := builtinAgent(t, "claude"); return &agent.Hooks }},
		{"codex", "codex", func(t *testing.T) *Mapping { agent := builtinAgent(t, "codex"); return &agent.Hooks }},
		{"statusline", "statusline", func(t *testing.T) *Mapping { return builtinAgent(t, "claude").Statusline }},
		{"cursor", "cursor", func(t *testing.T) *Mapping { agent := builtinAgent(t, "cursor"); return &agent.Hooks }},
	}
	for _, group := range cases {
		fixtures, err := filepath.Glob(filepath.Join("testdata", group.fixtures, "*.json"))
		if err != nil || len(fixtures) == 0 {
			t.Fatalf("no fixtures for %s (%v)", group.group, err)
		}
		for _, fixture := range fixtures {
			t.Run(group.group+"/"+filepath.Base(fixture), func(t *testing.T) {
				got, _ := group.mapping(t).Map(readTestFile(t, fixture), "", testNowMs)
				path := filepath.Join("testdata", "golden", group.group, strings.TrimSuffix(filepath.Base(fixture), ".json")+".events.json")
				if *updateGolden {
					writeTestFile(t, path, encode(t, goldenEvents{Events: got})+"\n", 0o644)
					return
				}
				var want goldenEvents
				if err := json.Unmarshal(readTestFile(t, path), &want); err != nil {
					t.Fatal(err)
				}
				compareEvents(t, got, want.Events)
			})
		}
	}
}

func compareEvents(t *testing.T, got, want []event.Event) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events differ:\n manifest %s\n go       %s", encode(t, got), encode(t, want))
	}
}

func encode(t *testing.T, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
