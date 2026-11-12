package power

import (
	"reflect"
	"testing"

	"agent-screensaver/internal/state"
)

func ptr(value int) *int {
	return &value
}

var (
	running      = state.Card{Kind: state.KindRunning}
	done         = state.Card{Kind: state.KindFinished}
	doneWithJobs = state.Card{Kind: state.KindFinished, Background: 2}
	waiting      = state.Card{Kind: state.KindApproval}
	unknown      = state.Card{Kind: state.KindUnknown}
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name    string
		reading Reading
		cards   []state.Card
		want    *Status
	}{
		{
			name:    "no battery",
			reading: Reading{},
			cards:   []state.Card{running},
			want:    nil,
		},
		{
			name:    "plugged in",
			reading: Reading{HasBattery: true, Percent: 15, Charging: true},
			cards:   []state.Card{running},
			want:    nil,
		},
		{
			name:    "on battery, fine",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 64, MinutesLeft: 110},
			cards:   []state.Card{running},
			want:    &Status{Percent: 64, MinutesLeft: ptr(110), Working: 1},
		},
		{
			name:    "zero minutes means unknown",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 64, MinutesLeft: 0},
			cards:   nil,
			want:    &Status{Percent: 64},
		},
		{
			name:    "unknown time",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 64, MinutesLeft: -1},
			cards:   nil,
			want:    &Status{Percent: 64},
		},
		{
			name:    "low, nothing works",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 20, MinutesLeft: -1},
			cards:   []state.Card{done},
			want:    &Status{Percent: 20, Low: true},
		},
		{
			name:    "low, one works",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 18, MinutesLeft: 34},
			cards:   []state.Card{running, done},
			want:    &Status{Percent: 18, MinutesLeft: ptr(34), Low: true, Warn: true, Working: 1},
		},
		{
			name:    "low, background jobs",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 10, MinutesLeft: -1},
			cards:   []state.Card{doneWithJobs},
			want:    &Status{Percent: 10, Low: true, Warn: true, Working: 1},
		},
		{
			name:    "low, approval waits",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 10, MinutesLeft: -1},
			cards:   []state.Card{waiting},
			want:    &Status{Percent: 10, Low: true, Warn: true, Working: 1},
		},
		{
			name:    "21 percent is not low",
			reading: Reading{HasBattery: true, OnBattery: true, Percent: 21, MinutesLeft: -1},
			cards:   []state.Card{running},
			want:    &Status{Percent: 21, Working: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.reading, tt.cards)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Evaluate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWorking(t *testing.T) {
	if got := Working([]state.Card{running, done, doneWithJobs, waiting}); got != 3 {
		t.Errorf("Working() = %d, want 3", got)
	}
	if got := Working(nil); got != 0 {
		t.Errorf("Working(nil) = %d, want 0", got)
	}
	if got := Working([]state.Card{unknown}); got != 1 {
		t.Errorf("Working(unknown) = %d, want 1", got)
	}
}

func TestSame(t *testing.T) {
	tests := []struct {
		name string
		a, b *Status
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil and set", nil, &Status{}, false},
		{"same, different working", &Status{Percent: 50, Working: 1}, &Status{Percent: 50, Working: 2}, true},
		{"different percent", &Status{Percent: 50}, &Status{Percent: 49}, false},
		{"different warn", &Status{Warn: true}, &Status{Warn: false}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Same(tt.a, tt.b); got != tt.want {
				t.Errorf("Same() = %v, want %v", got, tt.want)
			}
		})
	}
}
