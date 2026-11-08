package inara

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestApplicationAccessStatusIsRussianWithoutChangingRetryOrSecrets(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	retryAt := now.Add(time.Hour).UnixMilli()
	const raw = "This application has no access allowed."
	const key = "synthetic-key-only-not-a-real-account"
	m := &Manager{opts: Options{Now: func() time.Time { return now }}, available: true, config: Config{Enabled: true, APIKey: key}, service: Service{LastError: raw, RetryAt: retryAt}, queue: []QueueRow{{Key: "synthetic-event", NextAttempt: retryAt}}}
	status := m.Status()
	want := "Inara пока не разрешила доступ для ED Copilot. Ключ сохранён; отправка возобновится после разрешения сервиса."
	if status["lastError"] != want || status["apiKeySet"] != true || status["enabled"] != true || v.Int(status["queued"]) != 1 || v.Num(status["retryAt"]) != float64(retryAt) || status["nextRetryAt"] != v.ISO(now.Add(time.Hour)) {
		t.Fatal("incorrect public access-denied status")
	}
	if m.service.LastError != raw || m.service.RetryAt != retryAt || m.queue[0].NextAttempt != retryAt || m.config.APIKey != key {
		t.Fatal("display localization changed protected state or retry decisions")
	}
	encoded, err := json.Marshal(status)
	if err != nil || bytes.Contains(encoded, []byte(key)) || bytes.Contains(encoded, []byte(raw)) {
		t.Fatal("public status contains secret or untranslated service message")
	}
}

func TestOtherInaraStatusErrorsKeepMeaningAndRedactSavedKey(t *testing.T) {
	if got := displayError("HTTP 503: synthetic-secret", "synthetic-secret"); got != "HTTP 503: [скрыто]" {
		t.Fatal("existing safe-message behavior lost")
	}
	if displayError("", "") != "" {
		t.Fatal("empty status acquired an error")
	}
	if got := displayError("A different application access error", ""); got != "A different application access error" {
		t.Fatal("unrecognized error was reclassified")
	}
}
