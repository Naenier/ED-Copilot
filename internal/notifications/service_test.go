package notifications

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNotificationsAreOptInScopedAndDeduplicatedAfterRestart(t *testing.T) {
	now := time.Now()
	s := New(nil)
	s.Context("alpha", "flight")
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
	s.Context("alpha", "trade")
	if s.HUD(now) != nil || s.HUD(now.Add(time.Minute)) != nil {
		t.Fatal("a nonurgent notice survived activity change or expiry")
	}
	s.Context("beta", "flight")
	if len(s.Snapshot("beta")["events"].([]Event)) != 0 || s.Emit(event) {
		t.Fatal("alert crossed commander scope")
	}
	restarted := New(s.Export())
	restarted.Context("alpha", "flight")
	if restarted.Emit(event) || restarted.HUD(now) != nil || len(restarted.Snapshot("alpha")["events"].([]Event)) != 1 {
		t.Fatal("persistent history replayed or lost the alert")
	}
}

func TestLegacySpeechSettingsAreDiscardedAndVisualPreferencesSurvive(t *testing.T) {
	legacy := json.RawMessage(`{"preferences":{"enabled":true,"language":"ru","volume":75,"rate":-3,"rules":{"finding":{"enabled":true,"visual":false,"voice":true,"priority":2}}}}`)
	s := New(legacy)
	s.Context("alpha", "flight")
	now := time.Now()
	if !s.Emit(Event{ID: "old", Kind: "finding", Scope: "alpha", ObservedAt: now}) || s.HUD(now) != nil {
		t.Fatal("the existing history-only rule was lost")
	}
	if !s.Emit(Event{ID: "fuel", Kind: "fuel", Scope: "alpha", ObservedAt: now}) {
		t.Fatal("the existing visual notifications were lost")
	}
	if event := s.HUD(now); event == nil || event.ID != "fuel" {
		t.Fatal("the visual alert did not appear")
	}
	saved, err := json.Marshal(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []string{`"voice"`, `"language"`, `"volume"`, `"rate"`} {
		if strings.Contains(string(saved), obsolete) {
			t.Errorf("old notification field retained: %s", obsolete)
		}
	}
}
