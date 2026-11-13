package notifications

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

const historyLimit = 240

var priorities = map[string]int{"finding": 1, "engineering": 1, "task": 1, "carrier": 2, "fuel": 3}

type Rule struct {
	Enabled  bool `json:"enabled"`
	Visual   bool `json:"visual"`
	Priority int  `json:"priority"`
}

type Preferences struct {
	Enabled bool            `json:"enabled"`
	Rules   map[string]Rule `json:"rules"`
}

type Event struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Scope         string     `json:"scope,omitempty"`
	Activity      string     `json:"activity,omitempty"`
	Title         string     `json:"title"`
	EnglishTitle  string     `json:"englishTitle"`
	Detail        string     `json:"detail,omitempty"`
	EnglishDetail string     `json:"englishDetail,omitempty"`
	System        string     `json:"system,omitempty"`
	SystemAddress string     `json:"systemAddress,omitempty"`
	Body          string     `json:"body,omitempty"`
	BodyID        string     `json:"bodyId,omitempty"`
	Link          string     `json:"link,omitempty"`
	ObservedAt    time.Time  `json:"observedAt"`
	Priority      int        `json:"priority"`
	DisplayUntil  *time.Time `json:"displayUntil,omitempty"`
	visualUntil   time.Time
}

type savedState struct {
	Preferences Preferences `json:"preferences"`
	History     []Event     `json:"history"`
}

type Service struct {
	mu              sync.Mutex
	prefs           Preferences
	history         []Event
	seen            map[string]bool
	owner, activity string
	system          string
}

func New(raw any) *Service {
	defaults := Preferences{Rules: map[string]Rule{}}
	for kind, priority := range priorities {
		defaults.Rules[kind] = Rule{Enabled: true, Visual: true, Priority: priority}
	}
	s := &Service{prefs: defaults, seen: map[string]bool{}}
	if raw == nil {
		return s
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return s
	}
	saved := savedState{Preferences: clonePreferences(defaults)}
	if json.Unmarshal(data, &saved) != nil {
		return s
	}
	if valid(saved.Preferences) {
		s.prefs = saved.Preferences
	}
	for _, event := range saved.History[:min(len(saved.History), historyLimit)] {
		if !usable(event) || s.seen[event.key()] {
			continue
		}
		s.history = append(s.history, event)
		s.seen[event.key()] = true
	}
	return s
}

func clonePreferences(p Preferences) Preferences {
	out := map[string]Rule{}
	for kind, rule := range p.Rules {
		out[kind] = rule
	}
	p.Rules = out
	return p
}

func valid(p Preferences) bool {
	if len(p.Rules) != len(priorities) {
		return false
	}
	for kind, rule := range p.Rules {
		if priorities[kind] == 0 || rule.Priority < 1 || rule.Priority > 3 {
			return false
		}
	}
	return true
}

func (s *Service) Configure(raw any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clonePreferences(s.prefs)
	if json.Unmarshal(data, &next) != nil || !valid(next) {
		return errors.New("Проверьте приоритеты уведомлений.")
	}
	s.prefs = next
	return nil
}

func (e Event) key() string { return e.Scope + "\x00" + e.Kind + "\x00" + e.ID }

func usable(e Event) bool {
	return priorities[e.Kind] > 0 && e.Scope != "" && len(e.Scope) <= 100 && e.ID != "" && len(e.ID) <= 512 && !e.ObservedAt.IsZero()
}

func (s *Service) Enabled(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prefs.Enabled && s.prefs.Rules[kind].Enabled
}

func (s *Service) Context(owner, activity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != s.owner {
		s.system = ""
	}
	s.owner, s.activity = owner, activity
}

func (s *Service) SetSystem(system string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = system
}

func (s *Service) ClearNonurgent(kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := func(event Event) bool { return event.Scope == s.owner && event.Kind == kind && event.Priority < 3 }
	for i := range s.history {
		if active(s.history[i]) {
			s.history[i].visualUntil = time.Time{}
		}
	}
}

func (s *Service) Emit(event Event) bool {
	if !usable(event) {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rule := s.prefs.Rules[event.Kind]
	if !s.prefs.Enabled || !rule.Enabled || event.Scope != s.owner || s.seen[event.key()] {
		return false
	}
	now := time.Now()
	event.Priority = rule.Priority
	if rule.Visual {
		event.visualUntil = now.Add(18 * time.Second)
	}
	s.seen[event.key()] = true
	s.history = append([]Event{event}, s.history...)
	if len(s.history) > historyLimit {
		delete(s.seen, s.history[historyLimit].key())
		s.history = s.history[:historyLimit]
	}
	return true
}

func (s *Service) relevantLocked(e Event) bool {
	if e.Priority == 3 {
		return true
	}
	if e.Activity != "" && e.Activity != s.activity {
		return false
	}
	return e.Kind != "finding" || e.System == "" || s.system == "" || strings.EqualFold(e.System, s.system)
}

func (s *Service) Snapshot(owner string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := []Event{}
	for _, event := range s.history {
		if event.Scope != owner {
			continue
		}
		event.Scope = ""
		history = append(history, event)
	}
	return map[string]any{"preferences": clonePreferences(s.prefs), "events": history}
}

func (s *Service) HUD(now time.Time) *Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.prefs.Enabled {
		return nil
	}
	var best *Event
	for _, e := range s.history {
		rule := s.prefs.Rules[e.Kind]
		if e.Scope != s.owner || !now.Before(e.visualUntil) || !rule.Enabled || !rule.Visual || !s.relevantLocked(e) {
			continue
		}
		if best == nil || e.Priority > best.Priority {
			copy := e
			copy.Scope = ""
			copy.DisplayUntil = &copy.visualUntil
			best = &copy
		}
	}
	return best
}

func (s *Service) Export() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return savedState{Preferences: clonePreferences(s.prefs), History: append([]Event(nil), s.history...)}
}
