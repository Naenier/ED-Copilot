package frontier

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testCrypt(t *testing.T) (func([]byte) ([]byte, error), func([]byte) ([]byte, error)) {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	return func(plain []byte) ([]byte, error) {
			nonce := make([]byte, gcm.NonceSize())
			rand.Read(nonce)
			return append(nonce, gcm.Seal(nil, nonce, plain, nil)...), nil
		}, func(blob []byte) ([]byte, error) {
			if len(blob) < gcm.NonceSize() {
				return nil, fmt.Errorf("short fixture")
			}
			return gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
		}
}
func TestPKCEPersistenceSyncAndCooldown(t *testing.T) {
	dir := t.TempDir()
	encrypt, decrypt := testCrypt(t)
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	var refreshes, profiles, markets, carriers atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			r.ParseForm()
			if r.Form.Get("client_id") != ClientID {
				t.Error("wrong public client")
			}
			if r.Form.Get("grant_type") == "authorization_code" && len(r.Form.Get("code_verifier")) < 43 {
				t.Error("missing PKCE verifier")
			}
			token := "access-1"
			expires := 1
			if r.Form.Get("grant_type") == "refresh_token" {
				refreshes.Add(1)
				token = "access-2"
				expires = 3600
			}
			json.NewEncoder(w).Encode(v.M{"access_token": token, "refresh_token": "synthetic-refresh", "expires_in": expires, "token_type": "Bearer"})
		case "/decode":
			fmt.Fprint(w, `{"usr":{"customer_id":"123"}}`)
		case "/profile":
			profiles.Add(1)
			if r.Header.Get("Authorization") != "Bearer access-2" {
				t.Error("expired access token reused")
			}
			fmt.Fprint(w, `{"commander":{"name":"Pilot","credits":50000,"debt":100,"docked":true},"lastSystem":{"name":"Sol"},"lastStarport":{"id":100,"name":"Galileo"},"ship":{"name":"panthermkii","id":42,"shipName":"DESTINY","shipID":"ED-42"}}`)
		case "/market":
			markets.Add(1)
			fmt.Fprint(w, `{"id":100,"name":"Galileo","commodities":[{"name":"gold","locName":"Золото","buyPrice":10,"sellPrice":20,"stock":50,"demand":100,"stockBracket":1,"demandBracket":3}]}`)
		case "/shipyard":
			fmt.Fprint(w, `{"id":100}`)
		case "/fleetcarrier":
			carriers.Add(1)
			fmt.Fprint(w, `{"name":{"callsign":"AAA-BBB","vanityName":"54455354"},"currentStarSystem":"Sol","fuel":900,"capacity":{"freeSpace":1000},"finance":{"bankBalance":100000},"cargo":[{"commodity":"gold","qty":1},{"commodity":"gold","qty":1},{"commodity":"gold","qty":1,"stolen":true}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Options{DataDir: dir, HTTPClient: server.Client(), AuthURL: server.URL, CAPIURL: server.URL, Now: func() time.Time { return now }, Encrypt: encrypt, Decrypt: decrypt, GetCommander: func() v.M { return v.M{"name": "Pilot", "fid": "F123"} }})
	if err != nil {
		t.Fatal(err)
	}
	login, err := client.LoginURL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(login)
	q := parsed.Query()
	if q.Get("scope") != "auth capi" || q.Get("client_id") != ClientID || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 {
		t.Fatal(q)
	}
	if err = client.HandleCallback(url.Values{"state": {"incorrect"}, "code": {"test"}}); err == nil {
		t.Fatal("foreign callback accepted")
	}
	callback := url.Values{"state": {q.Get("state")}, "code": {"test"}}
	if err = client.HandleCallback(callback); err != nil {
		t.Fatal(err)
	}
	if err = client.HandleCallback(callback); err == nil {
		t.Fatal("callback replay accepted")
	}
	blob, err := os.ReadFile(filepath.Join(dir, "frontier-go-tokens.enc"))
	if err != nil || bytes.Contains(blob, []byte("synthetic-refresh")) {
		t.Fatal("credential persistence", err)
	}
	state := client.Status()
	for _, field := range []string{"clientId", "redirectUri", "accessToken", "refreshToken"} {
		if state[field] != nil {
			t.Fatal("secret/developer config in UI", field)
		}
	}
	syncResult, err := client.Sync(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if refreshes.Load() != 1 || profiles.Load() != 1 || markets.Load() != 1 || carriers.Load() != 1 {
		t.Fatal("first sync counts", refreshes.Load(), profiles.Load(), markets.Load(), carriers.Load())
	}
	summary := v.Obj(client.Status()["summary"])
	if v.Str(v.At(summary, "profile", "ship")) != "Panther Clipper Mk II" || v.Str(v.At(summary, "profile", "shipName")) != "DESTINY" {
		t.Fatal(summary)
	}
	if v.Str(v.At(summary, "carrier", "name")) != "TEST" || len(v.Rows(v.At(summary, "carrier", "cargo"))) != 2 {
		t.Fatal("carrier names or cargo grouping", summary)
	}
	market := MarketSnapshot(syncResult)
	if market == nil || len(v.Rows(market["Items"])) != 1 {
		t.Fatal("market snapshot", syncResult)
	}
	if _, err = client.Sync(context.Background(), true); err != nil || profiles.Load() != 1 || carriers.Load() != 1 {
		t.Fatal("cooldown not respected", err)
	}
	now = now.Add(61 * time.Second)
	if _, err = client.Sync(context.Background(), true); err != nil || profiles.Load() != 2 || carriers.Load() != 1 {
		t.Fatal("independent carrier cooldown", err)
	}
	restored, err := New(client.opts)
	if err != nil || !v.Bool(restored.Status()["connected"]) {
		t.Fatal("saved login not restored", err)
	}
	if err = client.Logout(); err != nil {
		t.Fatal(err)
	}
	restored, err = New(client.opts)
	if err != nil || v.Bool(restored.Status()["connected"]) {
		t.Fatal("logout not persistent", err)
	}
}
func TestLegacyImportDoesNotMutateRollbackAndLogoutDoesNotResurrect(t *testing.T) {
	dir := t.TempDir()
	encrypt, decrypt := testCrypt(t)
	plain, _ := json.Marshal(v.M{"accessToken": "legacy-access", "refreshToken": "legacy-refresh", "expires": time.Now().Add(time.Hour).UnixMilli(), "clientId": ClientID, "identity": v.M{"fid": "F123", "name": "Pilot"}})
	blob, _ := encrypt(plain)
	old := filepath.Join(dir, "frontier-tokens.enc")
	os.WriteFile(old, blob, 0600)
	options := Options{DataDir: dir, Encrypt: encrypt, Decrypt: decrypt}
	client, err := New(options)
	if err != nil || !v.Bool(client.Status()["connected"]) {
		t.Fatal("legacy import", err)
	}
	if err = client.Logout(); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := os.ReadFile(old)
	if !bytes.Equal(blob, unchanged) {
		t.Fatal("rollback credential changed")
	}
	client, err = New(options)
	if err != nil || v.Bool(client.Status()["connected"]) || v.String(client.Status(), "error") != "" {
		t.Fatal("legacy credentials resurrected after logout", client.Status(), err)
	}
}
func TestRefreshIsSingleFlightAndIdentityGate(t *testing.T) {
	dir := t.TempDir()
	encrypt, decrypt := testCrypt(t)
	var refreshes, requests atomic.Int32
	var commanderMu sync.RWMutex
	commander := v.M{"fid": "F123", "name": "Pilot"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			refreshes.Add(1)
			time.Sleep(10 * time.Millisecond)
			fmt.Fprint(w, `{"access_token":"new-token","refresh_token":"rotated-refresh","expires_in":3600}`)
			return
		}
		requests.Add(1)
		fmt.Fprint(w, `{"commander":{"name":"Pilot"}}`)
	}))
	defer server.Close()
	client, _ := New(Options{DataDir: dir, HTTPClient: server.Client(), AuthURL: server.URL, CAPIURL: server.URL, Encrypt: encrypt, Decrypt: decrypt, GetCommander: func() v.M { commanderMu.RLock(); defer commanderMu.RUnlock(); return v.Copy(commander) }})
	client.tokens = v.M{"accessToken": "old-token", "refreshToken": "old-refresh", "expires": 0}
	client.identity = v.M{"fid": "F123", "name": "Pilot"}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := client.capi(context.Background(), "profile"); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if refreshes.Load() != 1 || requests.Load() != 8 {
		t.Fatal("concurrent rotating refresh", refreshes.Load(), requests.Load())
	}
	commanderMu.Lock()
	commander["fid"] = "F999"
	commanderMu.Unlock()
	if _, err := client.capi(context.Background(), "profile"); err == nil {
		t.Fatal("foreign commander data fetched")
	}
	if requests.Load() != 8 || !v.Bool(client.Status()["commanderMismatch"]) {
		t.Fatal("identity gate", client.Status())
	}
}
func TestLogoutCancelsInFlightCallback(t *testing.T) {
	encrypt, decrypt := testCrypt(t)
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			close(started)
			<-release
			fmt.Fprint(w, `{"access_token":"late-token","refresh_token":"late-refresh","expires_in":3600}`)
		} else {
			fmt.Fprint(w, `{"usr":{"customer_id":"123"}}`)
		}
	}))
	defer server.Close()
	client, _ := New(Options{DataDir: t.TempDir(), HTTPClient: server.Client(), AuthURL: server.URL, Encrypt: encrypt, Decrypt: decrypt})
	login, _ := client.LoginURL()
	parsed, _ := url.Parse(login)
	done := make(chan error, 1)
	go func() {
		done <- client.HandleCallback(url.Values{"state": {parsed.Query().Get("state")}, "code": {"test"}})
	}()
	<-started
	if err := client.Logout(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("late callback restored login")
	}
	if v.Bool(client.Status()["connected"]) {
		t.Fatal("logged in after logout race")
	}
}
func TestRedirectAndMalformedPayloadRejected(t *testing.T) {
	for _, uri := range []string{"https://localhost:7122/api/frontier/callback", "http://example.com:7122/callback", "http://localhost/api/callback", "http://user:pass@localhost:7122/callback"} {
		if _, err := New(Options{DataDir: t.TempDir(), RedirectURI: uri}); err == nil {
			t.Fatal("unsafe redirect", uri)
		}
	}
	encrypt, decrypt := testCrypt(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/credential-target", http.StatusFound)
	}))
	defer server.Close()
	client, _ := New(Options{DataDir: t.TempDir(), HTTPClient: server.Client(), AuthURL: server.URL, Encrypt: encrypt, Decrypt: decrypt})
	if _, err := client.request(context.Background(), server.URL, "GET", nil, nil, time.Second); err == nil {
		t.Fatal("redirect followed")
	}
	if _, err := client.token(context.Background(), url.Values{"grant_type": {"test"}}); err == nil {
		t.Fatal("bad token response")
	}
}
