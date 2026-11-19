package notifications

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	historyLimit = 240
	soundLimit   = 8
	soundTTL     = 8 * time.Second
)

type Rule struct {
	Enabled  bool `json:"enabled"`
	Visual   bool `json:"visual"`
	Sound    bool `json:"sound"`
	Priority int  `json:"priority"`
}

type Preferences struct {
	Enabled bool            `json:"enabled"`
	Sound   bool            `json:"sound"`
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
	sounds          []soundEvent
}

type soundEvent struct {
	event   Event
	expires time.Time
}

type rulePatch struct {
	Enabled  *bool `json:"enabled"`
	Visual   *bool `json:"visual"`
	Sound    *bool `json:"sound"`
	Priority *int  `json:"priority"`
}

type preferencesPatch struct {
	Enabled *bool                 `json:"enabled"`
	Sound   *bool                 `json:"sound"`
	Rules   map[string]*rulePatch `json:"rules"`
}

func New(raw any) *Service {
	defaults := Preferences{Rules: map[string]Rule{}}
	for _, category := range catalog {
		defaults.Rules[category.ID] = Rule{Enabled: true, Visual: true, Sound: true, Priority: category.Priority}
	}
	s := &Service{prefs: defaults, seen: map[string]bool{}}
	if raw == nil {
		return s
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return s
	}
	var saved struct {
		Preferences preferencesPatch `json:"preferences"`
		History     []Event          `json:"history"`
	}
	if json.Unmarshal(data, &saved) != nil {
		return s
	}
	s.prefs = applyPreferences(defaults, saved.Preferences)
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

func applyPreferences(p Preferences, patch preferencesPatch) Preferences {
	if patch.Enabled != nil {
		p.Enabled = *patch.Enabled
	}
	if patch.Sound != nil {
		p.Sound = *patch.Sound
	}
	for kind, update := range patch.Rules {
		rule, known := p.Rules[kind]
		if !known || update == nil {
			continue
		}
		if update.Enabled != nil {
			rule.Enabled = *update.Enabled
		}
		if update.Visual != nil {
			rule.Visual = *update.Visual
		}
		if update.Sound != nil {
			rule.Sound = *update.Sound
		}
		if update.Priority != nil && *update.Priority >= 1 && *update.Priority <= 3 {
			rule.Priority = *update.Priority
		}
		p.Rules[kind] = rule
	}
	return p
}

func (s *Service) Configure(raw any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	var patch preferencesPatch
	if raw == nil || json.Unmarshal(data, &patch) != nil {
		return errors.New("Проверьте приоритеты уведомлений.")
	}
	for kind, rule := range patch.Rules {
		if _, known := categories[kind]; !known || rule == nil || rule.Priority != nil && (*rule.Priority < 1 || *rule.Priority > 3) {
			return errors.New("Проверьте приоритеты уведомлений.")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs = applyPreferences(s.prefs, patch)
	s.pruneSoundsLocked(time.Now())
	for i := range s.history {
		rule := s.prefs.Rules[s.history[i].Kind]
		if !s.prefs.Enabled || !rule.Enabled || !rule.Visual {
			s.history[i].visualUntil = time.Time{}
		}
	}
	return nil
}

func (e Event) key() string { return e.Scope + "\x00" + e.Kind + "\x00" + e.ID }

func usable(e Event) bool {
	return categories[e.Kind].Priority > 0 && e.Scope != "" && len(e.Scope) <= 100 && e.ID != "" && len(e.ID) <= 512 && !e.ObservedAt.IsZero()
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
		s.sounds = nil
		// An alert from the previous context stays in history but cannot
		// reappear if the commander changes back before its display expires.
		for i := range s.history {
			s.history[i].visualUntil = time.Time{}
		}
	}
	activityChanged := activity != s.activity
	s.owner, s.activity = owner, activity
	if activityChanged {
		s.pruneSoundsLocked(time.Now())
	}
}

func (s *Service) SetSystem(system string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = system
}

func (s *Service) ClearNonurgent(kind string) {
	s.dismiss(kind, false)
}

func (s *Service) Dismiss(kind string) {
	s.dismiss(kind, true)
}

func (s *Service) dismiss(kind string, includeUrgent bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := func(event Event) bool {
		return event.Scope == s.owner && event.Kind == kind && (includeUrgent || event.Priority < 3)
	}
	for i := range s.history {
		if active(s.history[i]) {
			s.history[i].visualUntil = time.Time{}
		}
	}
	for i := range s.sounds {
		if active(s.sounds[i].event) {
			s.sounds[i].expires = time.Time{}
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
	s.pruneSoundsLocked(now)
	if s.soundAllowedLocked(event) {
		if len(s.sounds) == soundLimit {
			// Prefer retaining critical candidates when several events arrive
			// before the native output loop can play one short cue.
			lowest := 0
			for i := 1; i < len(s.sounds); i++ {
				if s.sounds[i].event.Priority < s.sounds[lowest].event.Priority {
					lowest = i
				}
			}
			if s.sounds[lowest].event.Priority <= event.Priority {
				s.sounds[lowest] = soundEvent{event, now.Add(soundTTL)}
			}
		} else {
			s.sounds = append(s.sounds, soundEvent{event, now.Add(soundTTL)})
		}
	}
	return true
}

func (s *Service) soundAllowedLocked(e Event) bool {
	rule := s.prefs.Rules[e.Kind]
	return e.Scope == s.owner && s.prefs.Enabled && s.prefs.Sound && rule.Enabled && rule.Sound && s.relevantLocked(e)
}

func (s *Service) pruneSoundsLocked(now time.Time) {
	valid := s.sounds[:0]
	for _, candidate := range s.sounds {
		if now.Before(candidate.expires) && s.soundAllowedLocked(candidate.event) {
			valid = append(valid, candidate)
		}
	}
	clear(s.sounds[len(valid):])
	s.sounds = valid
}

// TakeSound coalesces a burst of observations into the highest-priority cue.
// The native consumer does not receive game or profile data.
func (s *Service) TakeSound() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneSoundsLocked(time.Now())
	priority := 0
	for _, candidate := range s.sounds {
		priority = max(priority, s.prefs.Rules[candidate.event.Kind].Priority)
	}
	clear(s.sounds)
	s.sounds = s.sounds[:0]
	return priority
}

func (s *Service) relevantLocked(e Event) bool {
	if e.Kind != "finding" {
		return true
	}
	return (e.Activity == "" || e.Activity == s.activity) &&
		(e.System == "" || s.system == "" || strings.EqualFold(e.System, s.system))
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
	return map[string]any{"preferences": clonePreferences(s.prefs), "catalog": append([]Category(nil), catalog...), "events": history}
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
