package nowplaying

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	const now = 1_000
	tests := []struct {
		name    string
		answer  string
		want    Track
		wantOK  bool
		wantErr bool
	}{
		{"nothing plays", "", Track{}, false, false},
		{"nothing plays, newline only", "\n", Track{}, false, false},
		{"spotify", "Spotify\x1fSloop John B\x1fThe Beach Boys\x1f128358\x1f179493\n",
			Track{App: "Spotify", Title: "Sloop John B", Artist: "The Beach Boys", PositionMs: 128358, DurationMs: 179493, AtMs: now}, true, false},
		{"a file without an artist", "Music\x1fjustep3\x1f\x1f104638\x1f971154\n",
			Track{App: "Music", Title: "justep3", PositionMs: 104638, DurationMs: 971154, AtMs: now}, true, false},
		{"a streamed song has no length", "Music\x1fwaiting for me\x1fArlo Parks\x1f0\x1f0\n",
			Track{App: "Music", Title: "waiting for me", Artist: "Arlo Parks", AtMs: now}, true, false},
		{"a tab in a title stays", "Spotify\x1fA\tB\x1fC\x1f1\x1f2", Track{App: "Spotify", Title: "A\tB", Artist: "C", PositionMs: 1, DurationMs: 2, AtMs: now}, true, false},
		{"no title is nothing", "Music\x1f\x1fSomeone\x1f1\x1f2", Track{}, false, false},
		{"a negative position is zero", "Music\x1fX\x1f\x1f-5\x1f10", Track{App: "Music", Title: "X", DurationMs: 10, AtMs: now}, true, false},
		{"too few fields", "Spotify\x1fX", Track{}, false, true},
		{"a position that is not a number", "Music\x1fX\x1f\x1f1,5\x1f10", Track{}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := Parse(tt.answer, now)
			if (err != nil) != tt.wantErr || ok != tt.wantOK || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q) = %+v, %v, %v; want %+v, %v, error %v", tt.answer, got, ok, err, tt.want, tt.wantOK, tt.wantErr)
			}
		})
	}
}
