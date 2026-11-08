package frontier

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestOptionalSquadronCargoSyncAndIndependentFailure(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	failSquadron, squadronCalls := false, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/profile":
			fmt.Fprint(w, `{"commander":{"name":"Pilot","docked":false}}`)
		case "/fleetcarrier":
			fmt.Fprint(w, `{"market":{"id":42},"name":{"callsign":"SYN-001"},"cargo":[{"commodity":"steel","qty":30}]}`)
		case "/squadron":
			squadronCalls++
			if failSquadron {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			fmt.Fprint(w, `{"id":7,"squadronCarrier":{"market":{"id":43},"name":{"callsign":"SYN-002"},"cargo":[{"commodity":"steel","qty":40}]},"bank":{"commodities":{"steel":[{"qty":9999}]}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	encrypt, decrypt := testCrypt(t)
	client, err := New(Options{DataDir: t.TempDir(), HTTPClient: server.Client(), CAPIURL: server.URL, Encrypt: encrypt, Decrypt: decrypt, Now: func() time.Time { return now }, GetCommander: func() v.M { return v.M{"fid": "F1", "name": "Pilot"} }})
	if err != nil {
		t.Fatal(err)
	}
	client.tokens = v.M{"accessToken": "synthetic", "refreshToken": "synthetic", "expires": now.Add(time.Hour).UnixMilli()}
	client.identity = v.M{"fid": "F1", "name": "Pilot"}
	result, err := client.Sync(context.Background(), true)
	if err != nil || v.Str(v.At(result, "data", "squadron", "squadronCarrier", "market", "id")) != "43" || v.At(result, "metadata", "observedAt", "squadron") == nil {
		t.Fatal("squadron carrier observation not returned", result, err)
	}
	if v.At(result, "metadata", "requestStartedAt", "carrier") == nil || v.At(result, "metadata", "requestStartedAt", "squadron") == nil {
		t.Fatal("carrier endpoint request boundaries are missing", result)
	}
	if _, err = client.Sync(context.Background(), true); err != nil || squadronCalls != 1 {
		t.Fatal("squadron endpoint ignored carrier cooldown", squadronCalls, err)
	}
	now = now.Add(16 * time.Minute)
	failSquadron = true
	result, err = client.Sync(context.Background(), true)
	if err != nil || v.At(result, "data", "profile") == nil || v.At(result, "data", "carrier") == nil || v.At(result, "errors", "squadron") == nil || v.At(result, "data", "squadron") != nil {
		t.Fatal("optional squadron failure disrupted personal sync", result, err)
	}
}
