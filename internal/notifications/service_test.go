package notifications

import (
	"context"
	"testing"
	"time"
)

func TestNotificationsAreOptInScopedAndDeduplicatedAfterRestart(t *testing.T) {
	now := time.Now()
	s := New(nil)
	s.Context("alpha", "flight", "en")
	event := Event{ID: "first", Kind: "finding", Scope: "alpha", Activity: "flight", Title: "Находка", EnglishTitle: "Discovery", System: "Sol", ObservedAt: now}
	if s.Emit(event) || s.HUD(now) != nil {
		t.Fatal("alerts must be explicitly enabled")
	}
	if err := s.Configure(map[string]any{"enabled": true}); err != nil {
		t.Fatal(err)
	}
	if !s.Emit(event) || s.Emit(event) {
		t.Fatal("expected precisely one accepted event")
	}
	if shown := s.HUD(now); shown == nil || shown.DisplayUntil == nil || shown.Scope != "" {
		t.Fatalf("HUD must expose a bounded alert without its scope: %#v", shown)
	}
	s.SetSystem("Achenar")
	if s.HUD(now) != nil {
		t.Fatal("the previous system's discovery remained on the HUD")
	}
	s.Context("alpha", "trade", "en")
	if s.HUD(now) != nil || s.HUD(now.Add(time.Minute)) != nil {
		t.Fatal("a nonurgent notice survived activity change or expiry")
	}
	s.Context("beta", "flight", "en")
	if len(s.Snapshot("beta")["events"].([]Event)) != 0 || s.Emit(event) {
		t.Fatal("alert crossed commander scope")
	}
	restarted := New(s.Export())
	restarted.Context("alpha", "flight", "en")
	if restarted.Emit(event) || restarted.HUD(now) != nil || len(restarted.Snapshot("alpha")["events"].([]Event)) != 1 {
		t.Fatal("persistent history replayed or lost the alert")
	}
}

func TestVoiceDropsQueuedActivityOnSwitchAndSpeaksCurrentUrgency(t *testing.T) {
	s := New(nil)
	s.Context("alpha", "flight", "en")
	p := s.Snapshot("alpha")["preferences"].(Preferences)
	p.Enabled = true
	for _, kind := range []string{"finding", "fuel"} {
		rule := p.Rules[kind]
		rule.Voice = true
		p.Rules[kind] = rule
	}
	if err := s.Configure(p); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.Emit(Event{ID: "first", Kind: "finding", Scope: "alpha", Activity: "flight", EnglishTitle: "Discovery", ObservedAt: now})
	s.Context("alpha", "trade", "en")
	s.Emit(Event{ID: "second", Kind: "fuel", Scope: "alpha", EnglishTitle: "Low fuel", ObservedAt: now})
	s.Preview("ru")
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	spoken := make(chan string, 3)
	go s.Run(ctx, func(_ context.Context, text, language string, _, _ int) error {
		spoken <- language + " " + text
		return nil
	})
	for _, want := range []string{"en Low fuel", "ru Проверка голоса ED Copilot."} {
		select {
		case text := <-spoken:
			if text != want {
				t.Fatalf("wanted %q, got %q", want, text)
			}
		case <-ctx.Done():
			t.Fatal("voice worker did not speak")
		}
	}
}
