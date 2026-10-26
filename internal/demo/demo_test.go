package demo

import (
	"strings"
	"testing"

	"agent-screensaver/internal/event"
	"agent-screensaver/internal/state"
)

const (
	totalMs = 60_000
	nowMs   = 1_800_000_000_000
)

func kinds(cards []state.Card) map[string]state.CardKind {
	bySource := map[string]state.CardKind{}
	for _, card := range cards {
		bySource[card.Source] = card.Kind
	}
	return bySource
}

func TestSceneKinds(t *testing.T) {
	running, finished, approval := state.KindRunning, state.KindFinished, state.KindApproval
	tests := []struct {
		name      string
		elapsedMs int64
		want      map[string]state.CardKind
	}{
		{"start", 0, map[string]state.CardKind{"claude": running, "codex": running, "cursor": running}},
		{"codex finished", 7_000, map[string]state.CardKind{"claude": running, "codex": finished, "cursor": running}},
		{"claude approval", 11_000, map[string]state.CardKind{"claude": approval, "codex": finished, "cursor": running}},
		{"claude running again", 19_000, map[string]state.CardKind{"claude": running, "codex": finished, "cursor": running}},
		{"all finished", 25_000, map[string]state.CardKind{"claude": finished, "codex": finished, "cursor": finished}},
		{"loop restarted", 31_000, map[string]state.CardKind{"claude": running, "codex": running, "cursor": running}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards, _ := Scene(tt.elapsedMs, totalMs, nowMs)
			got := kinds(cards)
			for source, want := range tt.want {
				if got[source] != want {
					t.Errorf("%s at %d ms = %q, want %q", source, tt.elapsedMs, got[source], want)
				}
			}
		})
	}
}

func TestSceneLoopRepeatsFirstSecond(t *testing.T) {
	first, _ := Scene(1_000, totalMs, nowMs)
	again, _ := Scene(31_000, totalMs, nowMs)
	for index := range first {
		if first[index].Kind != again[index].Kind || first[index].Session != again[index].Session {
			t.Errorf("card %d differs after the loop: %+v vs %+v", index, first[index], again[index])
		}
	}
}

func TestSceneApprovalCard(t *testing.T) {
	cards, _ := Scene(11_000, totalMs, nowMs)
	var claude state.Card
	for _, card := range cards {
		if card.Source == "claude" {
			claude = card
		}
	}
	if claude.Request != "Bash: npm run migrate" {
		t.Errorf("request = %q", claude.Request)
	}
	if claude.ApprovalSinceMs == nil || *claude.ApprovalSinceMs != nowMs-1_000 {
		t.Errorf("approval_since_ms = %v, want %d", claude.ApprovalSinceMs, nowMs-1_000)
	}
	if claude.Title != "refactor auth middleware" || claude.Branch != "feat/auth" ||
		claude.ContextPercent == nil || *claude.ContextPercent != 34 {
		t.Errorf("claude details wrong: %+v", claude)
	}
	for _, card := range cards {
		if card.Source != "claude" && (card.Request != "" || card.ApprovalSinceMs != nil) {
			t.Errorf("%s must not show an approval: %+v", card.Source, card)
		}
	}
}

func TestSceneStableIdentityAndStart(t *testing.T) {
	early, _ := Scene(0, totalMs, nowMs)
	late, _ := Scene(45_000, totalMs, nowMs+45_000)
	for index := range early {
		if early[index].Session != late[index].Session {
			t.Errorf("session id changed for %s", early[index].Source)
		}
		if early[index].StartedAtMs != late[index].StartedAtMs {
			t.Errorf("started_at_ms moved for %s", early[index].Source)
		}
		if early[index].StartedAtMs >= nowMs {
			t.Errorf("%s started in the future", early[index].Source)
		}
	}
}

func claudeFiveHour(limits []state.Limit) state.Limit {
	for _, limit := range limits {
		if limit.Source == "claude" && limit.Window == event.FiveHour {
			return limit
		}
	}
	return state.Limit{}
}

func TestSceneLimits(t *testing.T) {
	_, atStart := Scene(0, totalMs, nowMs)
	_, atHalf := Scene(totalMs/2, totalMs, nowMs)
	_, atEnd := Scene(totalMs, totalMs, nowMs)
	if got := claudeFiveHour(atStart).UsedPercent; got != 42 {
		t.Errorf("start = %v, want 42", got)
	}
	if got := claudeFiveHour(atEnd).UsedPercent; got != 61 {
		t.Errorf("end = %v, want 61", got)
	}
	if got := claudeFiveHour(atHalf).UsedPercent; got <= 42 || got >= 61 {
		t.Errorf("half = %v, want between 42 and 61", got)
	}
	_, beyond := Scene(2*totalMs, totalMs, nowMs)
	if got := claudeFiveHour(beyond).UsedPercent; got != 61 {
		t.Errorf("past the end = %v, want 61", got)
	}
	_, zeroTotal := Scene(10, 0, nowMs)
	if got := claudeFiveHour(zeroTotal).UsedPercent; got != 61 {
		t.Errorf("zero total = %v, want 61", got)
	}
}

func TestSceneFixedLimits(t *testing.T) {
	_, limits := Scene(5_000, totalMs, nowMs)
	want := map[string]float64{"claude/week": 28, "codex/5h": 17, "codex/week": 9}
	if len(limits) != 4 {
		t.Fatalf("got %d limits, want 4", len(limits))
	}
	for _, limit := range limits {
		if limit.ResetsAt == nil || *limit.ResetsAt*1000 <= nowMs {
			t.Errorf("%s/%s reset is not in the future", limit.Source, limit.Window)
		}
		if limit.UpdatedAtMs != nowMs {
			t.Errorf("%s/%s updated_at_ms = %d", limit.Source, limit.Window, limit.UpdatedAtMs)
		}
		if value, ok := want[limit.Source+"/"+string(limit.Window)]; ok && limit.UsedPercent != value {
			t.Errorf("%s/%s = %v, want %v", limit.Source, limit.Window, limit.UsedPercent, value)
		}
	}
}

func TestSceneResetCountdownTicks(t *testing.T) {
	_, first := Scene(0, totalMs, nowMs)
	_, later := Scene(10_000, totalMs, nowMs+10_000)
	if *claudeFiveHour(first).ResetsAt != *claudeFiveHour(later).ResetsAt {
		t.Error("the reset moment must stay fixed so the countdown ticks down")
	}
}

func TestSceneTextIsScripted(t *testing.T) {
	allowed := map[string]bool{
		"refactor auth middleware": true, "add rate limit tests": true, "update dashboard styles": true,
		"Bash: npm run migrate": true, "feat/auth": true, "": true,
	}
	for _, elapsed := range []int64{0, 7_000, 11_000, 19_000, 25_000} {
		cards, _ := Scene(elapsed, totalMs, nowMs)
		for _, card := range cards {
			for _, text := range []string{card.Title, card.Request, card.Branch} {
				if !allowed[text] {
					t.Errorf("unscripted text %q", text)
				}
			}
			for _, text := range []string{card.Project, card.Name, card.Model, card.Session, card.Cwd, card.Transcript} {
				if strings.ContainsAny(text, "/\\~") {
					t.Errorf("%q looks like a path", text)
				}
			}
		}
	}
}
