package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type updateFixture struct {
	t          *testing.T
	m          *Manager
	r          release
	setup      []byte
	sums       []byte
	requests   []string
	setupReply []byte
}

func fixture(t *testing.T) *updateFixture {
	t.Helper()
	root := t.TempDir()
	exe := filepath.Join(root, "install", "ED Copilot.exe")
	if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("synthetic original executable"), 0600); err != nil {
		t.Fatal(err)
	}
	f := &updateFixture{t: t, setup: []byte("synthetic installer; never run"), r: release{Tag: "v2.7.0", URL: "https://github.com/" + Repository + "/releases/tag/v2.7.0", Body: "Changes for the verified release."}}
	setupName := "ED-Copilot-2.7.0-Setup.exe"
	f.sums = []byte(hashBytes(f.setup) + "  " + setupName + "\r\n")
	f.r.Assets = []asset{
		{Name: setupName, URL: "https://github.com/" + Repository + "/releases/download/v2.7.0/" + setupName, Size: int64(len(f.setup)), Digest: "sha256:" + hashBytes(f.setup), State: "uploaded"},
		{Name: "SHA256SUMS.txt", URL: "https://github.com/" + Repository + "/releases/download/v2.7.0/SHA256SUMS.txt", Size: int64(len(f.sums)), Digest: "sha256:" + hashBytes(f.sums), State: "uploaded"},
	}
	client := &http.Client{Transport: roundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
			t.Error("public updater sent credentials")
		}
		f.requests = append(f.requests, req.URL.String())
		var body []byte
		switch req.URL.String() {
		case LatestURL:
			body, _ = json.Marshal(f.r)
		case f.r.Assets[1].URL:
			body = f.sums
		case f.r.Assets[0].URL:
			body = f.setup
			if f.setupReply != nil {
				body = f.setupReply
			}
		default:
			t.Errorf("unexpected download: %s", req.URL)
			return nil, errors.New("unexpected network request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), ContentLength: int64(len(body)), Header: make(http.Header)}, nil
	})}
	var err error
	f.m, err = New(Options{CurrentVersion: "2.6.0", DataDir: filepath.Join(root, "profile"), Executable: exe, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	f.m.installed = func(string) bool { return true }
	return f
}

func TestStableVersionOrdering(t *testing.T) {
	for _, test := range []struct {
		candidate, current string
		want               bool
	}{
		{"2.7.0", "2.6.9", true}, {"3.0.0", "2.99.99", true}, {"2.6.0", "2.6.0", false}, {"2.5.99", "2.6.0", false},
		{"02.7.0", "2.6.0", false}, {"2.7.0-beta.1", "2.6.0", false}, {"2.7.0+build", "2.6.0", false}, {"2.7", "2.6.0", false}, {"v2.7.0", "2.6.0", false},
	} {
		if got := newer(test.candidate, test.current); got != test.want {
			t.Fatalf("%s > %s: %v", test.candidate, test.current, got)
		}
	}
}

func TestCheckStagesThenApplyOnlyAfterExplicitCleanExit(t *testing.T) {
	f := fixture(t)
	launches := 0
	f.m.launch = func(path string, args []string) error {
		launches++
		if filepath.Base(path) != "update-helper.exe" || !reflect.DeepEqual(args, []string{"--apply-update", filepath.Join(f.m.root, "pending.json")}) {
			t.Fatal(path, args)
		}
		return nil
	}
	if err := f.m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatal("checking launched an installer")
	}
	state := f.m.Snapshot()
	if !state.Staged || state.Status != "staged" || state.AvailableVersion != "2.7.0" {
		t.Fatal(state)
	}
	if len(f.requests) != 3 {
		t.Fatal(f.requests)
	}
	if notice, err := f.m.ConsumeNotice(); err != nil || notice != nil {
		t.Fatal("staging displayed release notes", notice, err)
	}
	if err := f.m.ApplyOnExit(false); err != nil {
		t.Fatal(err)
	}
	if launches != 1 {
		t.Fatal(launches)
	}
	p, err := f.m.readPending()
	if err != nil {
		t.Fatal(err)
	}
	if p.ParentPID != os.Getpid() || p.Restart || p.Applied {
		t.Fatal(p)
	}
	copy, err := os.ReadFile(filepath.Join(f.m.root, p.Version, "update-helper.exe"))
	if err != nil || string(copy) != "synthetic original executable" {
		t.Fatal(string(copy), err)
	}
}

func TestReleaseValidationRejectsUnsafeAssets(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*updateFixture)
	}{
		{"draft", func(f *updateFixture) { f.r.Draft = true }},
		{"prerelease", func(f *updateFixture) { f.r.Prerelease = true }},
		{"older", func(f *updateFixture) { f.r.Tag = "v2.5.0" }},
		{"noncanonical", func(f *updateFixture) { f.r.Tag = "v02.7.0" }},
		{"other repository", func(f *updateFixture) { f.r.URL = "https://github.com/other/repo/releases/tag/v2.7.0" }},
		{"other setup host", func(f *updateFixture) { f.r.Assets[0].URL = "https://example.com/setup.exe" }},
		{"query on setup", func(f *updateFixture) { f.r.Assets[0].URL += "?download=1" }},
		{"missing digest", func(f *updateFixture) { f.r.Assets[0].Digest = "" }},
		{"oversize", func(f *updateFixture) { f.r.Assets[0].Size = maxInstaller + 1 }},
		{"duplicate setup", func(f *updateFixture) { f.r.Assets = append(f.r.Assets, f.r.Assets[0]) }},
		{"wrong sums digest", func(f *updateFixture) { f.r.Assets[1].Digest = "sha256:" + strings.Repeat("a", 64) }},
		{"disagreeing setup digests", func(f *updateFixture) { f.r.Assets[0].Digest = "sha256:" + strings.Repeat("a", 64) }},
		{"damaged setup", func(f *updateFixture) { f.setupReply = []byte(strings.Repeat("x", len(f.setup))) }},
		{"setup size mismatch", func(f *updateFixture) { f.setupReply = []byte("truncated") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := fixture(t)
			test.change(f)
			if err := f.m.Check(context.Background()); err == nil {
				t.Fatal("unsafe release accepted")
			}
			if f.m.Snapshot().Staged {
				t.Fatal("unsafe release staged")
			}
			if _, err := os.Stat(filepath.Join(f.m.root, "pending.json")); !os.IsNotExist(err) {
				t.Fatal("unsafe manifest persisted", err)
			}
		})
	}
}

func TestPortableAndDisabledSessionsDoNotDownloadSetup(t *testing.T) {
	f := fixture(t)
	f.m.installed = func(string) bool { return false }
	if err := f.m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 1 || f.m.Snapshot().Status != "portable" {
		t.Fatal(f.requests, f.m.Snapshot())
	}
	f.m.SetEnabled(false)
	if err := f.m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 1 {
		t.Fatal("disabled updater made a request")
	}
}

func TestDisableCancelsActiveDownload(t *testing.T) {
	f := fixture(t)
	started := make(chan struct{})
	f.m.client.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	done := make(chan error, 1)
	go func() { done <- f.m.Check(context.Background()) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("check did not start")
	}
	f.m.SetEnabled(false)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("disabling left download active")
	}
	if f.m.Snapshot().Enabled || f.m.Snapshot().Staged {
		t.Fatal(f.m.Snapshot())
	}
}

func TestEnablingWakesDisabledBackgroundLoop(t *testing.T) {
	f := fixture(t)
	f.m.SetEnabled(false)
	staged := make(chan struct{})
	var once sync.Once
	f.m.opts.OnChange = func() {
		if f.m.Snapshot().Staged {
			once.Do(func() { close(staged) })
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { f.m.Run(ctx); close(done) }()
	f.m.SetEnabled(true)
	select {
	case <-staged:
	case <-time.After(2 * time.Second):
		t.Fatal("enabling did not wake background check")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run ignored cancellation")
	}
}

func TestChangelogOnlyAfterVerifiedInstalledVersion(t *testing.T) {
	f := fixture(t)
	if err := f.m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, err := f.m.readPending()
	if err != nil {
		t.Fatal(err)
	}
	p.Applied = true
	if err = writeJSON(filepath.Join(f.m.root, "pending.json"), p); err != nil {
		t.Fatal(err)
	}
	if n, err := f.m.ConsumeNotice(); n != nil || err != nil {
		t.Fatal("old version showed notice", n, err)
	}
	f.m.opts.CurrentVersion = "2.7.0"
	n, err := f.m.ConsumeNotice()
	if err != nil || n == nil || n.FromVersion != "2.6.0" || n.Version != "2.7.0" {
		t.Fatal(n, err)
	}
	if n, err = f.m.ConsumeNotice(); n != nil || err != nil {
		t.Fatal("duplicate notice", n, err)
	}
}

func TestTamperedStagedSetupCannotLaunch(t *testing.T) {
	f := fixture(t)
	if err := f.m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.m.root, "2.7.0", "setup.exe"), []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	f.m.launch = func(string, []string) error { t.Fatal("tampered stage launched"); return nil }
	if err := f.m.ApplyOnExit(false); err == nil {
		t.Fatal("tampering was ignored")
	}
}

func TestChecksumParserRejectsMissingAndDuplicateSetup(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, body := range []string{digest + "  other.exe\n", digest + "  setup.exe\n" + digest + "  setup.exe\n", "garbage\n"} {
		if _, err := checksumFor([]byte(body), "setup.exe"); err == nil {
			t.Fatal(body)
		}
	}
	if got, err := checksumFor([]byte(digest+" *setup.exe\r\n"), "setup.exe"); err != nil || got != digest {
		t.Fatal(got, err)
	}
}

func TestRedirectAllowlist(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want bool
	}{
		{"https://release-assets.githubusercontent.com/path?signature=temporary", true},
		{"https://objects.githubusercontent.com/path", true},
		{"https://github.com/" + Repository + "/releases/download/v2.7.0/setup.exe", true},
		{"https://github.com/attacker/repo/releases/download/v2.7.0/setup.exe", false},
		{"http://release-assets.githubusercontent.com/path", false},
		{"https://release-assets.githubusercontent.com.evil.test/path", false},
		{"https://user@release-assets.githubusercontent.com/path", false},
		{"https://release-assets.githubusercontent.com:443/path", false},
	} {
		u, err := url.Parse(test.raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := trustedRedirect(u); got != test.want {
			t.Fatal(test.raw, got)
		}
	}
}
