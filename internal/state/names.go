package state

import (
	"cmp"
	"slices"
)

const (
	// SessionNameTTLMs is how long a session's name is remembered after it
	// was last seen.
	SessionNameTTLMs = 7 * 24 * 60 * 60 * 1000
	// MaxSessionNames bounds the remembered names; the newest stay.
	MaxSessionNames = 200
)

// SessionName is the last conversation name known for one session. The
// service writes these to state.json and reads them back when it starts,
// because the files it reads the name from may no longer hold it near
// their end (a long Claude transcript).
type SessionName struct {
	Source   string `json:"source"`
	Session  string `json:"session"`
	Name     string `json:"name"`
	SeenAtMs int64  `json:"seen_at_ms"`
}

type sessionKey struct{ source, session string }

// RememberNames adds names from an earlier run. A name already known
// for the session and seen later wins.
func (s *State) RememberNames(names []SessionName) {
	for _, name := range names {
		if name.Name != "" {
			s.remember(name)
		}
	}
}

func (s *State) remember(name SessionName) {
	if s.remembered == nil {
		s.remembered = map[sessionKey]SessionName{}
	}
	key := sessionKey{name.Source, name.Session}
	if known, ok := s.remembered[key]; !ok || name.SeenAtMs >= known.SeenAtMs {
		s.remembered[key] = name
	}
}

// rememberedName is the remembered name of a session, or "".
func (s *State) rememberedName(source, session string) string {
	return s.remembered[sessionKey{source, session}].Name
}

// SessionNames returns the name of each session that has one: the shown
// name of every card, and the remembered names seen in the last
// SessionNameTTLMs, newest first and at most MaxSessionNames. Older names
// are forgotten.
func (s *State) SessionNames(nowMs int64) []SessionName {
	for _, card := range s.Cards {
		if card.Name != "" {
			s.remember(SessionName{Source: card.Source, Session: card.Session, Name: card.Name, SeenAtMs: card.UpdatedAtMs})
		}
	}
	names := make([]SessionName, 0, len(s.remembered))
	for key, name := range s.remembered {
		if nowMs-name.SeenAtMs > SessionNameTTLMs {
			delete(s.remembered, key)
			continue
		}
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b SessionName) int {
		return cmp.Or(cmp.Compare(b.SeenAtMs, a.SeenAtMs), cmp.Compare(a.Source, b.Source), cmp.Compare(a.Session, b.Session))
	})
	if len(names) > MaxSessionNames {
		for _, dropped := range names[MaxSessionNames:] {
			delete(s.remembered, sessionKey{dropped.Source, dropped.Session})
		}
		names = names[:MaxSessionNames]
	}
	return names
}
