package event

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantKind Kind
		wantErr  bool
	}{
		{
			name:     "minimal started event",
			input:    `{"v":1,"source":"claude","session":"s1","kind":"started","at_ms":1}`,
			wantKind: KindStarted,
		},
		{
			name:    "unknown version",
			input:   `{"v":2,"source":"claude","session":"s1","kind":"started","at_ms":1}`,
			wantErr: true,
		},
		{
			name:    "empty source",
			input:   `{"v":1,"source":"","session":"s1","kind":"started","at_ms":1}`,
			wantErr: true,
		},
		{
			name:    "empty session",
			input:   `{"v":1,"source":"claude","session":"","kind":"started","at_ms":1}`,
			wantErr: true,
		},
		{
			name:    "source over 128 chars",
			input:   `{"v":1,"source":"` + strings.Repeat("a", 129) + `","session":"s1","kind":"started","at_ms":1}`,
			wantErr: true,
		},
		{
			name:     "source of 100 Hebrew characters (200 bytes)",
			input:    `{"v":1,"source":"` + strings.Repeat("א", 100) + `","session":"s1","kind":"started","at_ms":1}`,
			wantKind: KindStarted,
		},
		{
			name:     "session of 100 Hebrew characters (200 bytes)",
			input:    `{"v":1,"source":"claude","session":"` + strings.Repeat("א", 100) + `","kind":"started","at_ms":1}`,
			wantKind: KindStarted,
		},
		{
			name:    "source of 129 Hebrew characters",
			input:   `{"v":1,"source":"` + strings.Repeat("א", 129) + `","session":"s1","kind":"started","at_ms":1}`,
			wantErr: true,
		},
		{
			name:    "session over 128 chars",
			input:   `{"v":1,"source":"claude","session":"` + strings.Repeat("a", 129) + `","kind":"started","at_ms":1}`,
			wantErr: true,
		},
		{
			name:    "unknown kind",
			input:   `{"v":1,"source":"claude","session":"s1","kind":"exploded","at_ms":1}`,
			wantErr: true,
		},
		{
			name:    "input over 64 KiB",
			input:   `{"v":1,"source":"claude","session":"s1","kind":"started","at_ms":1,"title":"` + strings.Repeat("a", MaxInputBytes) + `"}`,
			wantErr: true,
		},
		{
			name:    "not json",
			input:   `nope`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantKind)
			}
			if len(got.Limits) != 0 {
				t.Errorf("Limits = %v, want none", got.Limits)
			}
		})
	}
}

func TestLimitsRoundTrip(t *testing.T) {
	resetsAt := int64(1800000000)
	original := Event{
		V:       Version,
		Source:  "claude",
		Session: "s1",
		Kind:    KindLimits,
		AtMs:    1,
		Limits:  []LimitReading{{Window: FiveHour, UsedPercent: 62.0, ResetsAt: &resetsAt}},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(got.Limits) != 1 {
		t.Fatalf("Limits length = %d, want 1", len(got.Limits))
	}
	reading := got.Limits[0]
	if reading.Window != FiveHour || reading.UsedPercent != 62.0 {
		t.Errorf("reading = %+v, want 5h at 62", reading)
	}
	if reading.ResetsAt == nil || *reading.ResetsAt != resetsAt {
		t.Errorf("ResetsAt = %v, want %d", reading.ResetsAt, resetsAt)
	}
}
