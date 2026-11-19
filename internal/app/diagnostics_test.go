package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/diagnostics"
)

func TestDiagnosticsReflectsAppJournalAndCommandOutcomes(t *testing.T) {
	e, journalFile, profileDir := fixtureEngine(t, M{"settings": M{"language": "ru"}})
	const private = "DO_NOT_LOG_API_KEY"
	if _, err := e.Do("/api/integrations/configure", M{"apiKey": private}); err == nil {
		t.Fatal("expected the offline profile to reject integration configuration")
	}
	if _, err := e.Do("/api/"+private, M{"action": private}); err == nil {
		t.Fatal("expected an unknown command error")
	}
	writeEvents(t, journalFile, true, []M{testEvent("CarrierDepositFuel", M{"CarrierID": 98, "Amount": 73})})
	e.Reader.Poll()
	if _, err := os.Stat(filepath.Join(profileDir, "logs")); err != nil {
		t.Fatal("missing local log directory:", err)
	}

	logs := func(filter M) []diagnostics.Entry {
		t.Helper()
		out, err := e.Do("/api/diagnostics/logs", filter)
		if err != nil {
			t.Fatal(err)
		}
		entries, ok := out["entries"].([]diagnostics.Entry)
		if !ok {
			t.Fatalf("unexpected diagnostic response: %#v", out)
		}
		encoded, err := json.Marshal(out)
		if err != nil || strings.Contains(string(encoded), private) || strings.Contains(string(encoded), "Synthetic Commander") {
			t.Fatalf("local log included private request or journal data: %s (%v)", encoded, err)
		}
		return entries
	}
	before := logs(nil)
	after := logs(nil)
	if len(before) < 6 || len(before) != len(after) {
		t.Fatalf("expected recorded operations and a read-only viewer: %v, %v", before, after)
	}
	wants := []struct {
		subsystem, level, search, event string
	}{
		{"app", "info", "", "startup.begin"},
		{"journal", "info", "Файлы 1/1", "scan"},
		{"carrier", "debug", "", "journal.carrierdepositfuel"},
		{"inara", "error", "", "integrations.configure"},
		{"app", "warn", "", "command.unknown"},
	}
	for _, want := range wants {
		found := false
		for _, entry := range logs(M{"subsystem": want.subsystem, "level": want.level, "search": want.search}) {
			found = found || entry.Event == want.event
		}
		if !found {
			t.Errorf("expected %s in filtered results", want.event)
		}
	}
	if _, err := e.Do("/api/settings", M{"language": "en"}); err != nil {
		t.Fatal(err)
	}
	if entries := logs(M{"subsystem": "journal", "search": "Game journals read"}); len(entries) != 1 ||
		!strings.HasPrefix(entries[0].Message, "Game journals read · Files") {
		t.Fatalf("localized search and rendering differ: %v", entries)
	}
}

func TestDiagnosticTransportKeepsProviderStatusWithoutRequestData(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	const private = "sample-private-value"
	base := &http.Client{Transport: economyRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("token") != private || r.Header.Get("Authorization") != private {
			t.Fatal("diagnostics altered the request")
		}
		return &http.Response{
			StatusCode: http.StatusTooManyRequests, Body: http.NoBody, Header: make(http.Header),
		}, nil
	})}
	client := e.observedHTTP(base, nil)
	req, err := http.NewRequest(http.MethodGet, "https://www.edsm.net/"+private+"?token="+private, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", private)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatal("lost the upstream response")
	}
	page, err := e.diagnostics.Query(diagnostics.Filter{Subsystem: "edsm", Level: diagnostics.Warn})
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("expected one provider warning: %v, %v", page, err)
	}
	entry := page.Entries[0]
	if entry.Event != "http.request" || !strings.Contains(entry.Message, "HTTP 429") || strings.Contains(entry.Message, private) {
		t.Fatalf("unexpected provider diagnostics: %v", entry)
	}
}
