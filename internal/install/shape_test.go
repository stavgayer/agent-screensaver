package install

import (
	"encoding/json"
	"strings"
	"testing"

	"agent-screensaver/internal/jsonedit"
)

func parseObject(t *testing.T, text string) *jsonedit.Object {
	t.Helper()
	object, err := jsonedit.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func flat(t *testing.T, object *jsonedit.Object) string {
	t.Helper()
	data, err := object.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReplacer(" ", "", "\n", "").Replace(string(data))
}

func TestRemoveDropsOnlyWhatWeEmptied(t *testing.T) {
	const ours = "'/bin/x' hook --tool codex"
	entry := AgentRecord{ID: "builtin.codex", Shape: shapeGrouped, Events: []string{"Stop", "SessionEnd"}, Commands: []string{ours}}
	tests := []struct {
		name string
		in   string
		want []string // substrings of the compacted result
		not  []string
	}{
		{"empty event the user left stays",
			`{"hooks":{"Stop":[],"SessionEnd":[{"hooks":[{"type":"command","command":"` + ours + `"}]}]}}`,
			[]string{`"Stop":[]`}, []string{"SessionEnd"}},
		{"hooks key stays when it was already empty", `{"hooks":{}}`, []string{`"hooks":{}`}, nil},
		{"hooks key goes when we emptied it",
			`{"a":1,"hooks":{"Stop":[{"hooks":[{"type":"command","command":"` + ours + `"}]}]}}`,
			[]string{`"a":1`}, []string{"hooks"}},
		{"user handler in the same group stays",
			`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"` + ours + `"},{"type":"command","command":"mine"}]}]}}`,
			[]string{`"command":"mine"`}, []string{ours}},
		{"an event we did not record is left alone",
			`{"hooks":{"PreCompact":[{"hooks":[{"type":"command","command":"` + ours + `"}]}]}}`,
			[]string{"PreCompact", "hook--toolcodex"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseObject(t, tt.in)
			if err := removeFromEvents(cfg, groupedShape{}, entry, nil); err != nil {
				t.Fatal(err)
			}
			got := flat(t, cfg)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("%s lacks %s", got, want)
				}
			}
			for _, not := range tt.not {
				if strings.Contains(got, not) {
					t.Errorf("%s still has %s", got, not)
				}
			}
		})
	}
}

func TestFlatRemoveKeepsOtherHandlers(t *testing.T) {
	list := []json.RawMessage{
		json.RawMessage(`{"type":"command","command":"mine"}`),
		json.RawMessage(`{"type":"command","command":"ours","timeout":5}`),
	}

	kept, removed, err := flatShape{}.remove(list, []string{"ours"})

	if err != nil || !removed || len(kept) != 1 || string(kept[0]) != `{"type":"command","command":"mine"}` {
		t.Errorf("kept %s, removed %v, err %v", kept, removed, err)
	}
}

func TestAddRejectsWrongShapes(t *testing.T) {
	home := t.TempDir()
	for _, in := range []string{`{"hooks":[]}`, `{"hooks":{"Stop":{}}}`} {
		t.Run(in, func(t *testing.T) {
			writeFile(t, codexPath(home), []byte(in))
			w := newWorkspace(home, "/bin/x")
			agent := builtinAgents(t)["codex"]
			if _, err := w.addAgent("/bin/x", agent, AgentRecord{}); err == nil {
				t.Error("want an error, got nil")
			}
		})
	}
}

func TestSwapStatuslineRejectsNonObject(t *testing.T) {
	if _, err := swapStatusline(parseObject(t, `{"statusLine":"x"}`), "statusLine", "ours", nil); err == nil {
		t.Error("want an error, got nil")
	}
}

func TestSwapStatuslineTreatsAnEarlierCommandOfOursAsOurs(t *testing.T) {
	cfg := parseObject(t, `{"statusLine":{"type":"command","command":"'/bin/x' statusline"}}`)

	previous, err := swapStatusline(cfg, "statusLine", "'/bin/x' statusline --tool claude", []string{"'/bin/x' statusline"})

	if err != nil || previous != "" || !strings.Contains(flat(t, cfg), `statusline--toolclaude`) {
		t.Errorf("previous %q, err %v, file %s", previous, err, flat(t, cfg))
	}
}

func TestRestoreStatuslineCases(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		previous string
		want     string
	}{
		{"restores previous, keeps padding", `{"statusLine":{"type":"command","command":"ours","padding":1}}`, "old", `"command":"old"`},
		{"removes ours when nothing before", `{"statusLine":{"type":"command","command":"ours"}}`, "", `{}`},
		{"keeps user keys when nothing before", `{"statusLine":{"type":"command","command":"ours","padding":1}}`, "", `"statusLine":{"padding":1}`},
		{"keeps user's own", `{"statusLine":{"type":"command","command":"mine"}}`, "old", `"command":"mine"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseObject(t, tt.in)
			if err := restoreStatusline(cfg, "statusLine", tt.previous, "ours"); err != nil {
				t.Fatal(err)
			}
			if got := flat(t, cfg); !strings.Contains(got, tt.want) {
				t.Errorf("%s lacks %s", got, tt.want)
			}
		})
	}
}
