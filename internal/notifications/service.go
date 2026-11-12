package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	historyLimit = 240
	queueLimit   = 20
)

var priorities = map[string]int{"finding": 1, "engineering": 1, "task": 1, "carrier": 2, "fuel": 3}

type Rule struct {
	Enabled  bool `json:"enabled"`
	Visual   bool `json:"visual"`
	Voice    bool `json:"voice"`
	Priority int  `json:"priority"`
}

type Preferences struct {
	Enabled  bool            `json:"enabled"`
	Language string          `json:"language"`
	Rate     int             `json:"rate"`
	Volume   int             `json:"volume"`
	Rules    map[string]Rule `json:"rules"`
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

type utterance struct {
	event           Event
	expires         time.Time
	previewLanguage string
	preview         *Preferences
}

type Speaker func(context.Context, string, string, int, int) error

type Service struct {
	mu                        sync.Mutex
	prefs                     Preferences
	history                   []Event
	seen                      map[string]bool
	pending                   []utterance
	current                   utterance
	cancel                    context.CancelFunc
	owner, activity, language string
	system                    string
	voiceError                bool
	wake                      chan struct{}
}

func New(raw any) *Service {
	defaults := Preferences{Language: "auto", Volume: 75, Rules: map[string]Rule{}}
	for kind, priority := range priorities {
		defaults.Rules[kind] = Rule{Enabled: true, Visual: true, Priority: priority}
	}
	s := &Service{prefs: defaults, seen: map[string]bool{}, language: "en", wake: make(chan struct{}, 1)}
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
	if p.Language != "auto" && p.Language != "ru" && p.Language != "en" || p.Volume < 0 || p.Volume > 100 || p.Rate < -10 || p.Rate > 10 || len(p.Rules) != len(priorities) {
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
		return errors.New("Проверьте язык, скорость, громкость и приоритеты уведомлений.")
	}
	s.prefs = next
	s.pruneLocked(time.Now())
	if s.cancel != nil && !s.validUtteranceLocked(s.current, time.Now()) {
		s.cancel()
	}
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

func (s *Service) Context(owner, activity, language string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != s.owner {
		s.system = ""
	}
	s.owner, s.activity, s.language = owner, activity, language
	s.refreshQueueLocked()
}

func (s *Service) SetSystem(system string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.system == system {
		return
	}
	s.system = system
	s.refreshQueueLocked()
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
	kept := s.pending[:0]
	for _, u := range s.pending {
		if !active(u.event) {
			kept = append(kept, u)
		}
	}
	s.pending = kept
	if s.cancel != nil && active(s.current.event) {
		s.cancel()
	}
}

func (s *Service) refreshQueueLocked() {
	now := time.Now()
	s.pruneLocked(now)
	if s.cancel != nil && !s.validUtteranceLocked(s.current, now) {
		s.cancel()
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
	if rule.Voice {
		s.enqueueLocked(utterance{event: event, expires: now.Add(30 * time.Second)})
	}
	return true
}

func (s *Service) Preview(language string, draft ...any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if language != "en" && language != "ru" {
		language = s.language
	}
	s.voiceError = false
	p := clonePreferences(s.prefs)
	if len(draft) > 0 && draft[0] != nil {
		data, err := json.Marshal(draft[0])
		if err != nil || json.Unmarshal(data, &p) != nil || !valid(p) {
			return errors.New("Проверьте язык, скорость, громкость и приоритеты уведомлений.")
		}
	}
	s.enqueueLocked(utterance{event: Event{Kind: "preview", Priority: 1, Title: "Проверка голоса ED Copilot.", EnglishTitle: "ED Copilot voice check."}, expires: time.Now().Add(20 * time.Second), previewLanguage: language, preview: &p})
	return nil
}

func (s *Service) enqueueLocked(u utterance) {
	s.pruneLocked(time.Now())
	s.pending = append(s.pending, u)
	sort.SliceStable(s.pending, func(i, j int) bool { return s.pending[i].event.Priority > s.pending[j].event.Priority })
	if len(s.pending) > queueLimit {
		s.pending = s.pending[:queueLimit]
	}
	if u.event.Priority == 3 && s.cancel != nil && s.current.event.Priority < u.event.Priority {
		s.cancel()
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) validUtteranceLocked(u utterance, now time.Time) bool {
	if !now.Before(u.expires) {
		return false
	}
	e := u.event
	if e.Kind == "preview" {
		return true
	}
	rule := s.prefs.Rules[e.Kind]
	return s.prefs.Enabled && rule.Enabled && rule.Voice && e.Scope == s.owner && s.relevantLocked(e)
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

func (s *Service) pruneLocked(now time.Time) {
	kept := s.pending[:0]
	for _, u := range s.pending {
		if s.validUtteranceLocked(u, now) {
			kept = append(kept, u)
		}
	}
	s.pending = kept
}

func (s *Service) Run(ctx context.Context, speak Speaker) {
	for ctx.Err() == nil {
		s.mu.Lock()
		s.pruneLocked(time.Now())
		if len(s.pending) == 0 {
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				continue
			}
		}
		u := s.pending[0]
		s.pending = s.pending[1:]
		p := s.prefs
		if u.preview != nil {
			p = *u.preview
		}
		display := s.language
		if u.previewLanguage != "" {
			display = u.previewLanguage
		}
		locale := p.Language
		if locale == "auto" {
			locale = display
		}
		voice, stop := context.WithTimeout(ctx, 15*time.Second)
		s.current, s.cancel = u, stop
		s.mu.Unlock()
		title, detail := u.event.Title, u.event.Detail
		if locale == "en" {
			title, detail = u.event.EnglishTitle, u.event.EnglishDetail
			if title == "" {
				title, detail = u.event.Title, u.event.Detail
			}
		}
		err := speak(voice, strings.TrimSpace(title+" "+detail), locale, p.Rate, p.Volume)
		cancelled := voice.Err()
		stop()
		s.mu.Lock()
		s.current, s.cancel = utterance{}, nil
		if cancelled == nil || errors.Is(cancelled, context.DeadlineExceeded) {
			s.voiceError = err != nil
		}
		s.mu.Unlock()
	}
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
	return map[string]any{"preferences": clonePreferences(s.prefs), "events": history, "voiceError": s.voiceError}
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
