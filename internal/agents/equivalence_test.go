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
	"agent-screensaver/internal/statusline"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite testdata/golden from the Go adapters")

// goldenEvents is what a fixture maps to, as written to testdata/golden.
type goldenEvents struct {
	Events []event.Event `json:"events"`
}

// TestBuiltinManifestsMatchGoAdapters is the equivalence gate of Task 6.4:
// every fixture gives the same events through the old Go adapter and the
// built-in manifest.
func TestBuiltinManifestsMatchGoAdapters(t *testing.T) {
	loaded, _ := Load(builtinDir)
	for _, source := range []string{"claude", "codex"} {
		agent, ok := BySource(loaded, source)
		if !ok {
			t.Fatalf("no built-in agent for %s", source)
		}
		adapter, _ := ByName(source)
		fixtures, err := filepath.Glob(filepath.Join("testdata", source, "*.json"))
		if err != nil || len(fixtures) == 0 {
			t.Fatalf("no fixtures for %s (%v)", source, err)
		}
		for _, fixture := range fixtures {
			t.Run(source+"/"+filepath.Base(fixture), func(t *testing.T) {
				input := readTestFile(t, fixture)
				var old []event.Event
				if mapped, ok := adapter.Map(input, testNowMs); ok {
					old = append(old, mapped)
				}
				got, _ := agent.Hooks.Map(input, "", testNowMs)
				compareEvents(t, got, old)
				writeGolden(t, source, fixture, old)
			})
		}
	}
	claude, _ := BySource(loaded, "claude")
	fixtures, _ := filepath.Glob(filepath.Join("..", "statusline", "testdata", "*.json"))
	if len(fixtures) == 0 {
		t.Fatal("no status line fixtures")
	}
	for _, fixture := range fixtures {
		t.Run("statusline/"+filepath.Base(fixture), func(t *testing.T) {
			input := readTestFile(t, fixture)
			var old []event.Event
			if limits, ok := statusline.LimitsEvent(input, testNowMs); ok {
				old = append(old, limits)
			}
			if info, ok := statusline.InfoEvent(input, testNowMs); ok {
				old = append(old, info)
			}
			got, _ := claude.Statusline.Map(input, "", testNowMs)
			compareEvents(t, got, old)
			writeGolden(t, "statusline", fixture, old)
		})
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

func writeGolden(t *testing.T, group, fixture string, events []event.Event) {
	t.Helper()
	if !*updateGolden {
		return
	}
	path := filepath.Join("testdata", "golden", group, strings.TrimSuffix(filepath.Base(fixture), ".json")+".events.json")
	writeTestFile(t, path, encode(t, goldenEvents{Events: events})+"\n", 0o644)
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
