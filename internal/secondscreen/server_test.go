package secondscreen

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSecondScreenRequiresSameOriginPairingAndRevokesDevice(t *testing.T) {
	var projections atomic.Int32
	s := New(func(mode string) View {
		projections.Add(1)
		return View{Language: "en", Cards: []Card{{Title: mode, Rows: []Row{{Label: "System", Value: "Sol"}}}}}
	})
	if err := s.Start("0.0.0.0", []string{"navigation"}); err == nil {
		t.Fatal("wildcard listener was accepted")
	}
	if err := s.Start("127.0.0.1", []string{"navigation", "engineering"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	status := s.Status()
	if !status.Running || len(status.Fingerprint) != 64 || len(status.Code) != 8 {
		t.Fatalf("pairing not ready: %#v", status)
	}
	leaf, err := x509.ParseCertificate(s.running.server.TLSConfig.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Jar: jar, Timeout: 3 * time.Second}
	request := func(method, path, origin, body string, want int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, status.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		content, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s: got %d (%q), wanted %d", method, path, res.StatusCode, content, want)
		}
		return content
	}
	request("GET", "/api/state", "", "", 401)
	pin := `{"code":"` + status.Code + `"}`
	request("POST", "/api/pair", "https://invalid.test", pin, 403)
	request("POST", "/api/pair", "", pin, 403)
	request("POST", "/api/pair", status.URL, pin, 204)
	request("POST", "/api/pair", status.URL, pin, 429)
	raw := request("GET", "/api/state?mode=engineering", "", "", 200)
	var projected struct {
		View
		Mode string
	}
	if err := json.Unmarshal(raw, &projected); err != nil || projected.Mode != "engineering" ||
		len(projected.Cards) != 1 || projected.Cards[0].Rows[0].Value != "Sol" || strings.Contains(string(raw), status.Code) {
		t.Fatalf("unexpected projection: %s", raw)
	}
	request("GET", "/api/state", "", "", 429)
	devices := s.Status().Devices
	if len(devices) != 1 {
		t.Fatalf("unexpected devices: %#v", devices)
	}
	firstDevice := devices[0].ID
	firstClient := client
	if err := s.Rotate(); err != nil {
		t.Fatal(err)
	}
	secondJar, _ := cookiejar.New(nil)
	client = &http.Client{Transport: transport, Jar: secondJar, Timeout: 3 * time.Second}
	request("POST", "/api/pair", status.URL, `{"code":"`+s.Status().Code+`"}`, 204)
	request("GET", "/api/state?mode=engineering", "", "", 200)
	if got := projections.Load(); got != 1 {
		t.Fatalf("two devices sharing a view generated %d projections", got)
	}
	s.Invalidate()
	s.Revoke(firstDevice)
	secondClient := client
	client = firstClient
	request("GET", "/api/state", "", "", 401)
	if len(s.Status().Devices) != 1 || s.Status().Devices[0].ID == firstDevice {
		t.Fatal("revocation did not target just the selected device")
	}
	client = secondClient
	request("GET", "/api/state", "", "", 429)
}
