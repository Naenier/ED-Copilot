package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type accountFixtureTransport struct {
	mu           sync.Mutex
	inaraEvents  []v.M
	profileCalls int
}

func (f *accountFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var data any
	switch r.URL.Hostname() {
	case "auth.frontierstore.net":
		switch r.URL.Path {
		case "/token":
			body, _ := io.ReadAll(r.Body)
			params, _ := url.ParseQuery(string(body))
			if params.Get("client_id") != "0a4c1d43-e433-411e-bcf5-54527fe5a34f" || params.Get("code_verifier") == "" {
				return nil, fmt.Errorf("invalid synthetic PKCE exchange")
			}
			data = v.M{"access_token": "synthetic-access", "refresh_token": "synthetic-refresh", "expires_in": 3600, "token_type": "Bearer"}
		case "/decode":
			data = v.M{"usr": v.M{"customer_id": "123"}}
		default:
			return nil, fmt.Errorf("unexpected auth path")
		}
	case "companion.orerve.net":
		if r.Header.Get("Authorization") != "Bearer synthetic-access" {
			return nil, fmt.Errorf("invalid synthetic bearer")
		}
		switch r.URL.Path {
		case "/profile":
			f.profileCalls++
			data = v.M{"commander": v.M{"name": "Synthetic Pilot", "credits": 987654, "debt": 12, "docked": true}, "lastSystem": v.M{"name": "Sol"}, "lastStarport": v.M{"id": 100, "name": "Galileo"}, "ship": v.M{"name": "panthermkii", "id": 7, "shipName": "Синтетический грузовик", "shipID": "SYN-07"}}
		case "/market":
			data = v.M{"id": 100, "name": "Galileo", "commodities": []v.M{{"name": "gold", "locName": "Золото", "buyPrice": 10, "sellPrice": 20, "stock": 100, "demand": 100, "stockBracket": 2, "demandBracket": 2}}}
		case "/shipyard":
			data = v.M{"id": 100}
		case "/fleetcarrier":
			return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		default:
			return nil, fmt.Errorf("unexpected CAPI path")
		}
	case "inara.cz":
		var body v.M
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, err
		}
		if v.Str(v.At(body, "header", "APIkey")) != "synthetic-inara-key" {
			return nil, fmt.Errorf("unexpected synthetic Inara key")
		}
		acks := []v.M{}
		for _, event := range v.Rows(body["events"]) {
			f.inaraEvents = append(f.inaraEvents, v.Copy(event))
			acks = append(acks, v.M{"eventStatus": 200, "eventCustomID": event["eventCustomID"]})
		}
		data = v.M{"header": v.M{"eventStatus": 200}, "events": acks}
	default:
		return nil, fmt.Errorf("test forbids nonfixture destination")
	}
	raw, _ := json.Marshal(data)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(raw)), Request: r}, nil
}
func accountFixture(t *testing.T) (*Engine, *accountFixtureTransport, string) {
	t.Helper()
	dir, journals := t.TempDir(), t.TempDir()
	stamp := v.ISO(time.Now().Add(-time.Second))
	events := []v.M{{"event": "Fileheader", "gameversion": "4.4.2", "timestamp": stamp}, {"event": "LoadGame", "Commander": "Synthetic Pilot", "FID": "F123", "Ship": "panthermkii", "ShipID": 7, "Credits": 1000, "Loan": 12, "timestamp": stamp}, {"event": "Location", "StarSystem": "Sol", "StarPos": []float64{0, 0, 0}, "SystemAddress": 1234, "Docked": true, "StationName": "Galileo", "MarketID": 100, "timestamp": stamp}, {"event": "Cargo", "Vessel": "Ship", "Inventory": []v.M{{"Name": "gold", "Count": 3}}, "timestamp": stamp}}
	file := filepath.Join(journals, "Journal.synthetic.log")
	var b bytes.Buffer
	for _, event := range events {
		raw, _ := json.Marshal(event)
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(file, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	transport := &accountFixtureTransport{}
	engine, err := New(Options{DataDir: dir, JournalDir: journals, CallbackPort: port, ManualPolling: true, HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine, transport, file
}
func callbackRequest(t *testing.T, engine *Engine, query url.Values, host, path, method string) int {
	t.Helper()
	address := engine.listener.Addr().String()
	request, err := http.NewRequest(method, "http://"+address+path+"?"+query.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if host == "" {
		host = "localhost:" + strconv.Itoa(engine.listener.Addr().(*net.TCPAddr).Port)
	}
	request.Host = host
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	return response.StatusCode
}
func TestAccountWorkflowThroughEngineAndNativeCallback(t *testing.T) {
	engine, transport, _ := accountFixture(t)
	result, err := engine.Do("/api/frontier/login", nil)
	if err != nil {
		t.Fatal(err)
	}
	login, _ := url.Parse(v.String(result, "url"))
	query := url.Values{"state": {login.Query().Get("state")}, "code": {"synthetic-code"}}
	if code := callbackRequest(t, engine, query, "foreign.example", "/api/frontier/callback", "GET"); code != 403 {
		t.Fatal("foreign callback host accepted", code)
	}
	if code := callbackRequest(t, engine, query, "", "/", "GET"); code != 403 {
		t.Fatal("local listener exposed other routes", code)
	}
	if code := callbackRequest(t, engine, query, "", "/api/frontier/callback", "POST"); code != 403 {
		t.Fatal("callback method accepted", code)
	}
	if code := callbackRequest(t, engine, query, "", "/api/frontier/callback", "GET"); code != 200 {
		t.Fatal("native callback", code)
	}
	if code := callbackRequest(t, engine, query, "", "/api/frontier/callback", "GET"); code != 400 {
		t.Fatal("callback replay", code)
	}
	if _, err = engine.Do("/api/integrations/configure", v.M{"apiKey": "synthetic-inara-key", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Do("/api/frontier/sync", v.M{"carrier": true}); err != nil {
		t.Fatal(err)
	}
	state := engine.Snapshot()
	if v.Str(v.At(state, "frontier", "summary", "profile", "ship")) != "Panther Clipper Mk II" || v.Str(v.At(state, "frontier", "summary", "profile", "shipName")) != "Синтетический грузовик" {
		t.Fatal("ship model and personal name conflated", v.At(state, "frontier", "summary"))
	}
	if v.Num(v.At(state, "frontier", "summary", "profile", "credits")) != 987654 {
		t.Fatal("CAPI profile not visible")
	}
	if _, err = engine.Do("/api/integrations/sync", nil); err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	received := append([]v.M(nil), transport.inaraEvents...)
	transport.mu.Unlock()
	foundCredit := false
	for _, event := range received {
		if event["eventName"] == "setCommanderCredits" && v.Num(v.At(event, "eventData", "commanderCredits")) == 987654 {
			foundCredit = true
		}
	}
	if !foundCredit {
		t.Fatal("verified CAPI credits were not submitted to Inara", received)
	}
	encoded, _ := json.Marshal(state)
	for _, secret := range []string{"synthetic-inara-key", "synthetic-access", "synthetic-refresh"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatal("credential leaked into application state", secret)
		}
	}
	for _, name := range []string{"frontier-go-tokens.enc", "integration-go-state.bin"} {
		blob, err := os.ReadFile(filepath.Join(engine.opts.DataDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"synthetic-inara-key", "synthetic-access", "synthetic-refresh"} {
			if bytes.Contains(blob, []byte(secret)) {
				t.Fatal("plaintext credential persisted", name)
			}
		}
	}
	if _, err = engine.Do("/api/frontier/logout", nil); err != nil {
		t.Fatal(err)
	}
	if v.Bool(v.At(engine.Snapshot(), "frontier", "connected")) {
		t.Fatal("logout did not update state")
	}
}
func TestInaraAppendAndRescanBoundaryThroughEngine(t *testing.T) {
	engine, _, file := accountFixture(t)
	if _, err := engine.Do("/api/integrations/configure", v.M{"apiKey": "synthetic-inara-key", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	appendEvent := func(station string) {
		t.Helper()
		event := v.M{"event": "Docked", "timestamp": v.ISO(time.Now()), "StarSystem": "Sol", "StationName": station, "MarketID": 100}
		raw, _ := json.Marshal(event)
		f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(append(raw, '\n'))
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		engine.Reader.Poll()
	}
	appendEvent("Депо Альфа")
	before := v.Int(v.At(engine.Snapshot(), "integrations", "queued"))
	if before < 1 {
		t.Fatal("new journal record not queued")
	}
	engine.Reader.Rescan()
	engine.Reader.Poll()
	after := v.Int(v.At(engine.Snapshot(), "integrations", "queued"))
	if after != before {
		t.Fatal("historical rescan resubmitted events", before, after)
	}
	appendEvent("Депо Бета")
	if got := v.Int(v.At(engine.Snapshot(), "integrations", "queued")); got <= after {
		t.Fatal("Unicode journal byte cursor skipped a new record", got)
	}
	if _, err := engine.Do("/api/integrations/configure", v.M{"enabled": false}); err != nil {
		t.Fatal(err)
	}
	paused := v.Int(v.At(engine.Snapshot(), "integrations", "queued"))
	appendEvent("Депо при выключенной отправке")
	if got := v.Int(v.At(engine.Snapshot(), "integrations", "queued")); got != paused {
		t.Fatal("opt-out did not suppress new journal upload")
	}
}
