package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestNearestRegionIsNotReplacedByCommanderSystem(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	e.opts.DisableNetwork = false
	e.opts.HTTPClient = &http.Client{Transport: economyRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "spansh.co.uk" || req.URL.Path != "/api/systems/search" {
			return nil, fmt.Errorf("unexpected request %s", req.URL)
		}
		var payload M
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if payload["reference_system"] != nil || v.At(payload, "filters", "region") == nil || v.Int(payload["page"]) != 1 {
			t.Errorf("wrong regional search payload: %v", payload)
		}
		const result = `{"count":49,"results":[{"name":"Example","id64":18446744073709551615}]}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(result)), Header: http.Header{}}, nil
	})}
	got, err := e.SearchNearest(M{"sequence": 2, "category": "systems", "region": "Inner Orion Spur", "page": 1})
	if err != nil || got["status"] != "ok" {
		t.Fatalf("nearest search failed: %v %v", got, err)
	}
	rows := v.Rows(got["rows"])
	if len(rows) != 1 || rows[0]["systemAddress"] != "18446744073709551615" {
		t.Fatalf("system address or result lost: %v", rows)
	}
	if _, err = e.CancelNearest(M{"sequence": 4}); err != nil {
		t.Fatal(err)
	}
	got, err = e.SearchNearest(M{"sequence": 3, "category": "systems", "origin": "Sol"})
	if err != nil || got["status"] != "cancelled" {
		t.Fatalf("superseded search accepted: %v %v", got, err)
	}
}
