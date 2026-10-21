package agents

import (
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

func marshalString(t *testing.T, object *jsonedit.Object) string {
	t.Helper()
	data, err := object.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRemoveHooksDropsOnlyWhatWeEmptied(t *testing.T) {
	const ours = "'/bin/x' hook --tool codex"
	tests := []struct {
		name string
		in   string
		want []string // substrings of the compacted result
		not  []string
	}{
		{
			name: "empty event the user left stays",
			in:   `{"hooks":{"Stop":[],"SessionEnd":[{"hooks":[{"type":"command","command":"` + ours + `"}]}]}}`,
			want: []string{`"Stop":[]`},
			not:  []string{"SessionEnd"},
		},
		{
			name: "hooks key stays when it was already empty",
			in:   `{"hooks":{}}`,
			want: []string{`"hooks":{}`},
		},
		{
			name: "hooks key goes when we emptied it",
			in:   `{"a":1,"hooks":{"Stop":[{"hooks":[{"type":"command","command":"` + ours + `"}]}]}}`,
			want: []string{`"a":1`},
			not:  []string{"hooks"},
		},
		{
			name: "user handler in the same group stays",
			in:   `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"` + ours + `"},{"type":"command","command":"mine"}]}]}}`,
			want: []string{`"command":"mine"`},
			not:  []string{ours},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := parseObject(t, tt.in)
			if err := (Codex{}).RemoveHooks(cfg, ours); err != nil {
				t.Fatal(err)
			}
			got := strings.NewReplacer(" ", "", "\n", "").Replace(marshalString(t, cfg))
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

func TestAddHooksRejectsWrongShapes(t *testing.T) {
	for _, in := range []string{`{"hooks":[]}`, `{"hooks":{"Stop":{}}}`} {
		t.Run(in, func(t *testing.T) {
			if err := (Claude{}).AddHooks(parseObject(t, in), "cmd"); err == nil {
				t.Error("want an error, got nil")
			}
		})
	}
}

func TestSwapStatuslineRejectsNonObject(t *testing.T) {
	if _, err := SwapStatusline(parseObject(t, `{"statusLine":"x"}`), "ours"); err == nil {
		t.Error("want an error, got nil")
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
			if err := RestoreStatusline(cfg, tt.previous, "ours"); err != nil {
				t.Fatal(err)
			}
			got := strings.NewReplacer(" ", "", "\n", "").Replace(marshalString(t, cfg))
			if !strings.Contains(got, tt.want) {
				t.Errorf("%s lacks %s", got, tt.want)
			}
		})
	}
}
