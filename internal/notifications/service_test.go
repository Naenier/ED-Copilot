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

func TestSoundIsOptInAndIndependentOfHUD(t *testing.T) {
	s := New(nil)
	s.Context("alpha", "flight")
	now := time.Now()
	emit := func(kind, id, activity string) {
		t.Helper()
		if !s.Emit(Event{Kind: kind, ID: id, Scope: "alpha", Activity: activity, ObservedAt: now}) {
			t.Fatal("notification did not enter history", id)
		}
	}
	set := func(raw map[string]any) {
		t.Helper()
		if err := s.Configure(raw); err != nil {
			t.Fatal(err)
		}
	}
	set(map[string]any{"enabled": true})
	emit("task", "first", "")
	if s.TakeSound() != 0 || s.HUD(now) == nil {
		t.Fatal("sound should be opt-in, with visual messages independent")
	}
	set(map[string]any{"sound": true, "rules": map[string]any{"task": map[string]any{"visual": false}}})
	emit("task", "soundOnly", "")
	if s.HUD(now) != nil || s.TakeSound() != 1 || s.TakeSound() != 0 {
		t.Fatal("sound-only notification was not consumed once")
	}
	emit("task", "queue", "")
	set(map[string]any{"rules": map[string]any{"task": map[string]any{"sound": false}}})
	if s.TakeSound() != 0 {
		t.Fatal("an already queued cue escaped its category mute")
	}
	prefs := s.Snapshot("alpha")["preferences"].(Preferences)
	if !prefs.Enabled || !prefs.Sound || !prefs.Rules["task"].Enabled || prefs.Rules["task"].Visual || prefs.Rules["task"].Sound {
		t.Fatalf("partial update overwrote existing preferences: %+v", prefs.Rules["task"])
	}
	emit("finding", "off-context", "cartography")
	if s.TakeSound() != 0 {
		t.Fatal("an off-context finding played a cue")
	}
	emit("cartographyComplete", "cross-activity", "cartography")
	if s.TakeSound() != 2 || s.HUD(now) == nil {
		t.Fatal("route completion across activities was suppressed")
	}
	emit("carrierArrived", "simultaneous", "")
	emit("fuel", "urgent", "cartography")
	if s.TakeSound() != 3 || s.TakeSound() != 0 {
		t.Fatal("simultaneous alerts must play just the most urgent cue")
	}
	emit("carrierArrived", "old-commander", "")
	s.Context("beta", "flight")
	s.Context("alpha", "flight")
	if s.TakeSound() != 0 || s.HUD(now) != nil {
		t.Fatal("a transient alert survived a commander change")
	}
	if restarted := New(s.Export()); restarted.TakeSound() != 0 || len(restarted.Snapshot("alpha")["events"].([]Event)) != 8 {
		t.Fatal("saved history was lost or played again after restart")
	}
}
