package app

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/systems"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func drainLocalizationUpdates(e *Engine) {
	for {
		select {
		case <-e.Updates():
		default:
			return
		}
	}
}
func TestInterfaceLanguagePersistsAcrossProfileReloadWithoutChangingIdentities(t *testing.T) {
	e, journal, dir := fixtureEngine(t, nil)
	if e.Language() != "ru" {
		t.Fatal("a profile without a language did not default to Russian")
	}
	before := e.Commander()
	doOK(t, e, "/api/settings", M{"language": "en"})
	if e.Language() != "en" || v.At(e.SnapshotForUI("overview"), "settings", "language") != "en" {
		t.Fatal("English preference did not reach UI state")
	}
	saved, err := profile.Read(filepath.Join(dir, "settings-go-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if v.At(saved, "settings", "language") != "en" {
		t.Fatal("English preference was not saved")
	}
	e.Close()
	reloaded, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(journal), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if reloaded.Language() != "en" {
		t.Fatal("English preference lost on restart")
	}
	for _, key := range []string{"name", "fid", "system", "station", "shipType", "shipName", "shipIdent"} {
		if reloaded.Commander()[key] != before[key] {
			t.Fatalf("language change altered observation identity %s", key)
		}
	}
	doOK(t, reloaded, "/api/settings", M{"language": "ru"})
	if reloaded.Language() != "ru" {
		t.Fatal("switching back to Russian did not apply")
	}
	saved, err = profile.Read(filepath.Join(dir, "settings-go-v2.json"))
	if err != nil || v.At(saved, "settings", "language") != "ru" {
		t.Fatal("Russian preference was not saved", err)
	}
}
func TestInvalidInterfaceLanguageIsRejectedAtomically(t *testing.T) {
	e, _, dir := fixtureEngine(t, M{"settings": M{"language": "en", "minTier": "medium"}})
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "settings-go-v2.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	e.mu.RLock()
	settings, _ := json.Marshal(e.settings)
	revision := e.revision
	e.mu.RUnlock()
	drainLocalizationUpdates(e)
	for _, language := range []any{"de", "EN", "", nil, 42, M{"language": "ru"}} {
		if _, err = e.Do("/api/settings", M{"language": language, "minTier": "high", "overlayEnabled": false}); err == nil {
			t.Fatalf("invalid language accepted: %v", language)
		}
		e.mu.RLock()
		currentSettings, _ := json.Marshal(e.settings)
		changed := !bytes.Equal(currentSettings, settings) || e.revision != revision
		e.mu.RUnlock()
		if changed {
			t.Fatal("invalid language partially changed other settings")
		}
		body, readErr := os.ReadFile(file)
		if readErr != nil || !bytes.Equal(before, body) {
			t.Fatal("invalid language rewrote saved settings", readErr)
		}
		select {
		case <-e.Updates():
			t.Fatal("invalid settings triggered a UI update")
		default:
		}
	}
}
func TestSystemQueryAndCancellationValidateJavaScriptSafeSequenceNumbers(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	for _, sequence := range []any{nil, -1, 1.5, math.NaN(), math.Inf(1), float64(9007199254740992), "not-a-number", M{"n": 1}} {
		input := M{"query": "Synthetic System", "fieldId": "synthetic-input", "sequence": sequence}
		if _, err := e.QuerySystems(input); err == nil {
			t.Errorf("query accepted malformed sequence %v", sequence)
		}
		if _, err := e.CancelSystems(input); err == nil {
			t.Errorf("cancellation accepted malformed sequence %v", sequence)
		}
	}
	for _, sequence := range []any{0, json.Number("9007199254740991")} {
		input := M{"query": "Synthetic System", "fieldId": "synthetic-input", "sequence": sequence, "limit": 20}
		r, err := e.QuerySystems(input)
		if err != nil || r["status"] != "disabled" || v.Num(r["sequence"]) != v.Num(sequence) || r["fieldId"] != "synthetic-input" || len(v.Rows(r["suggestions"])) != 0 {
			t.Fatal("disabled query did not preserve safe request identity", r, err)
		}
		r, err = e.CancelSystems(input)
		if err != nil || r["ok"] != true || v.Num(r["sequence"]) != v.Num(sequence) {
			t.Fatal("valid disabled cancellation failed", r, err)
		}
	}
}
func TestSystemAutocompleteDoesNotSaveNotifyOrWaitForDomainSnapshots(t *testing.T) {
	e, _, dir := fixtureEngine(t, nil)
	if err := e.Save(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "settings-go-v2.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	marker := time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)
	if err = os.Chtimes(file, marker, marker); err != nil {
		t.Fatal(err)
	}
	var networkCalls atomic.Int32
	for _, online := range []bool{false, true} {
		if online {
			e.Systems.Close()
			e.Systems = systems.NewClient(systems.Options{MinInterval: -1, HTTPClient: &http.Client{Transport: economyRoundTripper(func(r *http.Request) (*http.Response, error) {
				networkCalls.Add(1)
				if r.URL.Host != "spansh.co.uk" || r.URL.Query().Get("q") != "Synthetic System" {
					t.Error("unexpected system endpoint/query")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"values":["Synthetic System"],"min_max":[{"name":"Synthetic System","id64":18446744073709551615,"x":0,"y":1,"z":2}]}`))}, nil
			})}})
			e.opts.DisableNetwork = false
		}
		drainLocalizationUpdates(e)
		e.mu.RLock()
		revision := e.revision
		e.mu.RUnlock()
		// Holding both locks proves this read-only request never enters a domain
		// snapshot or a settings save, rather than merely observing equal output.
		e.domainMu.Lock()
		e.saveMu.Lock()
		type outcome struct {
			result M
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			r, err := e.QuerySystems(M{"query": "Synthetic System", "fieldId": "synthetic-keyboard", "sequence": 1, "limit": 20})
			if err == nil {
				_, err = e.CancelSystems(M{"fieldId": "synthetic-keyboard", "sequence": 1})
			}
			done <- outcome{r, err}
		}()
		var output outcome
		timedOut := false
		select {
		case output = <-done:
		case <-time.After(2 * time.Second):
			timedOut = true
		}
		e.saveMu.Unlock()
		e.domainMu.Unlock()
		if timedOut {
			t.Fatal("autocomplete waited for a full-history snapshot or profile save")
		}
		if output.err != nil {
			t.Fatal(output.err)
		}
		want := "disabled"
		if online {
			want = "ready"
		}
		if output.result["status"] != want {
			t.Fatal("wrong read-only query status", output.result)
		}
		if online && v.Rows(output.result["suggestions"])[0]["id64"] != "18446744073709551615" {
			t.Fatal("app bridge rounded canonical ID64")
		}
		e.mu.RLock()
		changed := e.revision != revision
		e.mu.RUnlock()
		if changed {
			t.Fatal("autocomplete changed application revision")
		}
		select {
		case <-e.Updates():
			t.Fatal("autocomplete notified a full UI refresh")
		default:
		}
		body, readErr := os.ReadFile(file)
		info, statErr := os.Stat(file)
		if readErr != nil || statErr != nil || !bytes.Equal(before, body) || !info.ModTime().Equal(marker) {
			t.Fatal("autocomplete rewrote profile data", readErr, statErr)
		}
	}
	if networkCalls.Load() != 1 {
		t.Fatal("disabled search made a network request or live search was duplicated", networkCalls.Load())
	}
}
