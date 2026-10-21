package jsonedit

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, input string) *Object {
	t.Helper()
	object, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	return object
}

func mustGet(t *testing.T, object *Object, key string) json.RawMessage {
	t.Helper()
	raw, ok := object.Get(key)
	if !ok {
		t.Fatalf("key %q missing", key)
	}
	return raw
}

func compact(t *testing.T, raw []byte) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		t.Fatalf("compact %q: %v", raw, err)
	}
	return buffer.String()
}

func TestKeyOrderAndUntouchedBytesKept(t *testing.T) {
	data, err := os.ReadFile("testdata/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, key := range []string{"z", "a", "m"} {
		before[key] = compact(t, mustGet(t, object, key))
	}

	if err := object.Set("hooks", json.RawMessage(`{"Stop":[]}`)); err != nil {
		t.Fatal(err)
	}
	out, err := object.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	reparsed, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"z", "a", "hooks", "m"}; !reflect.DeepEqual(reparsed.Keys(), want) {
		t.Fatalf("keys = %v, want %v", reparsed.Keys(), want)
	}
	for key, want := range before {
		if got := compact(t, mustGet(t, reparsed, key)); got != want {
			t.Errorf("%s changed: %s, want %s", key, got, want)
		}
	}
	if got := compact(t, mustGet(t, reparsed, "hooks")); got != `{"Stop":[]}` {
		t.Errorf("hooks = %s", got)
	}
	// Untouched literals must not be normalised (1.50 stays 1.50) or HTML-escaped.
	for _, fragment := range []string{`1.50`, `<html> & unicode é`} {
		if !strings.Contains(string(out), fragment) {
			t.Errorf("output lost %q:\n%s", fragment, out)
		}
	}
}

func TestMarshalFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty object", `{}`, "{}\n"},
		{"two space indent", `{"a":{"b":[1,2]},"c":null}`, "{\n  \"a\": {\n    \"b\": [\n      1,\n      2\n    ]\n  },\n  \"c\": null\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := mustParse(t, tt.input).Marshal()
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.want {
				t.Errorf("got %q, want %q", out, tt.want)
			}
		})
	}
}

func TestEmptyInputIsEmptyObject(t *testing.T) {
	for _, input := range []string{"", "  \n\t "} {
		object := mustParse(t, input)
		if len(object.Keys()) != 0 {
			t.Errorf("Parse(%q) keys = %v", input, object.Keys())
		}
		out, err := object.Marshal()
		if err != nil || string(out) != "{}\n" {
			t.Errorf("Marshal = %q, %v", out, err)
		}
	}
}

func TestNonObjectIsError(t *testing.T) {
	for _, input := range []string{`[]`, `"x"`, `1`, `null`, `true`} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse([]byte(input)); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestInvalidJSONIsError(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"truncated", `{"a": 1`},
		{"bad value", `{"a": }`},
		{"missing colon", `{"a" 1}`},
		{"trailing comma", `{"a": 1,}`},
		{"garbage", `not json`},
		{"trailing object", `{} {}`},
		{"trailing garbage", `{}x`},
		{"duplicate top-level key", `{"a": 1, "b": 2, "a": 3}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.input)); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestSetNewKeyAppends(t *testing.T) {
	object := mustParse(t, `{"b":1,"a":2}`)
	if err := object.Set("c", json.RawMessage(`3`)); err != nil {
		t.Fatal(err)
	}
	if err := object.Set("b", json.RawMessage(`9`)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(object.Keys(), want) {
		t.Errorf("keys = %v, want %v", object.Keys(), want)
	}
	if got := string(mustGet(t, object, "b")); got != "9" {
		t.Errorf("b = %s", got)
	}
}

func TestSetRejectsInvalidJSON(t *testing.T) {
	object := mustParse(t, `{"a":1}`)
	for _, raw := range []string{``, `{`, `nope`, `1 2`} {
		if err := object.Set("a", json.RawMessage(raw)); err == nil {
			t.Errorf("Set(%q) expected error", raw)
		}
	}
	if got := string(mustGet(t, object, "a")); got != "1" {
		t.Errorf("a changed to %s after rejected Set", got)
	}
}

func TestDelete(t *testing.T) {
	object := mustParse(t, `{"a":1,"b":2,"c":3}`)
	object.Delete("b")
	object.Delete("missing")
	if want := []string{"a", "c"}; !reflect.DeepEqual(object.Keys(), want) {
		t.Errorf("keys = %v, want %v", object.Keys(), want)
	}
	if _, ok := object.Get("b"); ok {
		t.Error("b still present")
	}
}

func TestGetAndSetCopyBytes(t *testing.T) {
	object := mustParse(t, `{"a":"xyz"}`)
	got := mustGet(t, object, "a")
	got[1] = 'Q'
	if string(mustGet(t, object, "a")) != `"xyz"` {
		t.Error("mutating Get result changed the object")
	}

	raw := json.RawMessage(`"abc"`)
	if err := object.Set("b", raw); err != nil {
		t.Fatal(err)
	}
	raw[1] = 'Q'
	if string(mustGet(t, object, "b")) != `"abc"` {
		t.Error("mutating Set input changed the object")
	}
}

func TestKeysReturnsCopy(t *testing.T) {
	object := mustParse(t, `{"a":1}`)
	object.Keys()[0] = "x"
	if want := []string{"a"}; !reflect.DeepEqual(object.Keys(), want) {
		t.Errorf("keys = %v", object.Keys())
	}
}

func TestOddKeysKeepSpelling(t *testing.T) {
	keys := []string{"", "with space", `quote"inside`, `back\slash`, "new\nline", "<&>", "unicode é 日本", "PreToolUse|Bash"}
	object := mustParse(t, `{}`)
	for index, key := range keys {
		if err := object.Set(key, json.RawMessage(`1`)); err != nil {
			t.Fatalf("Set(%q) #%d: %v", key, index, err)
		}
	}
	out, err := object.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	reparsed, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reparsed.Keys(), keys) {
		t.Errorf("keys = %q, want %q", reparsed.Keys(), keys)
	}
	if !strings.Contains(string(out), `"<&>"`) {
		t.Errorf("key was HTML-escaped:\n%s", out)
	}
}

func TestNestedObjectEditKeepsOrder(t *testing.T) {
	outer := mustParse(t, `{"z":1,"hooks":{"Stop":[],"Alpha":{"k":1},"Mid":2},"a":3}`)

	innerRaw := mustGet(t, outer, "hooks")
	inner, err := Parse(innerRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := inner.Set("Alpha", json.RawMessage(`{"k":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := inner.Set("Zeta", json.RawMessage(`[]`)); err != nil {
		t.Fatal(err)
	}
	innerBytes, err := inner.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := outer.Set("hooks", innerBytes); err != nil {
		t.Fatal(err)
	}
	out, err := outer.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	reparsed := mustParse(t, string(out))
	if want := []string{"z", "hooks", "a"}; !reflect.DeepEqual(reparsed.Keys(), want) {
		t.Errorf("outer keys = %v, want %v", reparsed.Keys(), want)
	}
	reparsedInner := mustParse(t, string(mustGet(t, reparsed, "hooks")))
	if want := []string{"Stop", "Alpha", "Mid", "Zeta"}; !reflect.DeepEqual(reparsedInner.Keys(), want) {
		t.Errorf("inner keys = %v, want %v", reparsedInner.Keys(), want)
	}
	if got := compact(t, mustGet(t, reparsedInner, "Alpha")); got != `{"k":2}` {
		t.Errorf("Alpha = %s", got)
	}
	want := "{\n  \"z\": 1,\n  \"hooks\": {\n    \"Stop\": [],\n    \"Alpha\": {\n      \"k\": 2\n    },\n    \"Mid\": 2,\n    \"Zeta\": []\n  },\n  \"a\": 3\n}\n"
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
