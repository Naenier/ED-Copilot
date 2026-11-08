// Package updater stages verified releases from the application's fixed public
// repository. It never closes windows or stops the game. Applying is explicitly
// deferred until the host has saved its state and is fully exiting.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Repository    = "Naenier/ED-Copilot"
	LatestURL     = "https://api.github.com/repos/" + Repository + "/releases/latest"
	DesktopMutex  = `Local\EDCopilot.Desktop.Session`
	maxMetadata   = 1 << 20
	maxChecksums  = 64 << 10
	maxInstaller  = 256 << 20
	checkInterval = 6 * time.Hour
)

type Options struct {
	CurrentVersion string
	DataDir        string
	Executable     string
	HTTPClient     *http.Client
	OnChange       func()
}

type State struct {
	Enabled          bool   `json:"enabled"`
	Status           string `json:"status"`
	Version          string `json:"version"`
	AvailableVersion string `json:"availableVersion,omitempty"`
	ReleaseURL       string `json:"releaseURL,omitempty"`
	Error            string `json:"error,omitempty"`
	LastChecked      string `json:"lastChecked,omitempty"`
	Staged           bool   `json:"staged"`
}

type Notice struct {
	Version     string `json:"version"`
	FromVersion string `json:"fromVersion"`
	Body        string `json:"body"`
	ReleaseURL  string `json:"releaseURL"`
}

type Manager struct {
	mu           sync.Mutex
	checking     sync.Mutex
	opts         Options
	client       *http.Client
	root         string
	state        State
	installed    func(string) bool
	launch       func(string, []string) error
	activeCancel context.CancelFunc
	wake         chan struct{}
}

type release struct {
	Tag        string  `json:"tag_name"`
	URL        string  `json:"html_url"`
	Body       string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}
type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	State  string `json:"state"`
}

type pending struct {
	Version     string `json:"version"`
	FromVersion string `json:"fromVersion"`
	ReleaseURL  string `json:"releaseURL"`
	Body        string `json:"body"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Executable  string `json:"executable"`
	ParentPID   int    `json:"parentPID"`
	Restart     bool   `json:"restart"`
	Applied     bool   `json:"applied"`
}

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// version accepts only canonical stable SemVer; prerelease/build suffixes and
// leading zeroes are deliberately ineligible for automatic installation.
func version(raw string) ([3]uint64, error) {
	var out [3]uint64
	parts := stableVersion.FindStringSubmatch(raw)
	if parts == nil {
		return out, errors.New("unsupported stable version")
	}
	for i := range out {
		n, err := strconv.ParseUint(parts[i+1], 10, 64)
		if err != nil {
			return out, errors.New("version component is too large")
		}
		out[i] = n
	}
	return out, nil
}

func newer(candidate, current string) bool {
	a, errA := version(candidate)
	b, errB := version(current)
	if errA != nil || errB != nil {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

func New(o Options) (*Manager, error) {
	if _, err := version(o.CurrentVersion); err != nil {
		return nil, err
	}
	if o.DataDir == "" {
		return nil, errors.New("update profile directory is required")
	}
	root, err := filepath.Abs(filepath.Join(o.DataDir, "updates"))
	if err != nil {
		return nil, err
	}
	if o.Executable == "" {
		o.Executable, err = os.Executable()
		if err != nil {
			return nil, err
		}
	}
	o.Executable, err = filepath.Abs(o.Executable)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	if o.HTTPClient != nil {
		*client = *o.HTTPClient
	}
	client.Timeout = 5 * time.Minute
	client.Jar = nil // public releases need neither cookies nor account credentials
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 4 || !trustedRedirect(req.URL) {
			return errors.New("untrusted update redirect")
		}
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		return nil
	}
	m := &Manager{opts: o, client: client, root: root, state: State{Enabled: true, Status: "idle", Version: o.CurrentVersion}, installed: installedExecutable, launch: launchDetached, wake: make(chan struct{}, 1)}
	if p, err := m.readPending(); err == nil && newer(p.Version, o.CurrentVersion) && m.verifyPending(p) == nil {
		m.state.Staged, m.state.Status, m.state.AvailableVersion, m.state.ReleaseURL = true, "staged", p.Version, p.ReleaseURL
	}
	return m, nil
}

func trustedRedirect(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch u.Hostname() {
	case "api.github.com":
		return u.Path == "/repos/"+Repository+"/releases/latest"
	case "github.com":
		return strings.HasPrefix(u.Path, "/"+Repository+"/releases/download/v")
	case "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	}
	return false
}

func (m *Manager) notify() {
	if m.opts.OnChange != nil {
		m.opts.OnChange()
	}
}
func (m *Manager) Snapshot() State { m.mu.Lock(); defer m.mu.Unlock(); return m.state }
func (m *Manager) SetEnabled(enabled bool) {
	m.mu.Lock()
	changed := m.state.Enabled != enabled
	m.state.Enabled = enabled
	cancel := m.activeCancel
	if !enabled {
		m.state.Status = "disabled"
		m.state.Error = ""
	} else if changed {
		if m.state.Staged {
			m.state.Status = "staged"
		} else {
			m.state.Status = "idle"
		}
	}
	m.mu.Unlock()
	if !enabled && cancel != nil {
		cancel()
	}
	if enabled && changed {
		select {
		case m.wake <- struct{}{}:
		default:
		}
	}
	m.notify()
}
func (m *Manager) change(fn func(*State)) { m.mu.Lock(); fn(&m.state); m.mu.Unlock(); m.notify() }

// Run is a blocking, cancellable loop. The host starts it only for real online
// sessions, never demo, capture or manual-polling tests.
func (m *Manager) Run(ctx context.Context) {
	for {
		if m.Snapshot().Enabled {
			_ = m.Check(ctx)
		}
		timer := time.NewTimer(checkInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (m *Manager) get(ctx context.Context, rawURL string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("invalid update request")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ED-Copilot-Updater/"+m.opts.CurrentVersion)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := m.client.Do(req)
	if err != nil {
		return nil, errors.New("update download unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update server status %d", response.StatusCode)
	}
	if response.ContentLength > max {
		return nil, errors.New("update response exceeds size limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, max+1))
	if err != nil {
		return nil, errors.New("update response incomplete")
	}
	if int64(len(body)) > max {
		return nil, errors.New("update response exceeds size limit")
	}
	return body, nil
}

func validateAsset(a asset, tag string, max int64) (string, error) {
	expected := "https://github.com/" + Repository + "/releases/download/" + tag + "/" + url.PathEscape(a.Name)
	if a.State != "uploaded" || a.URL != expected || a.Size <= 0 || a.Size > max {
		return "", errors.New("release asset identity or size is invalid")
	}
	digest := strings.TrimPrefix(a.Digest, "sha256:")
	if a.Digest != "sha256:"+digest || !digestPattern.MatchString(digest) {
		return "", errors.New("release asset has no SHA256 digest")
	}
	return digest, nil
}

func selectAssets(r release, current string) (string, asset, asset, error) {
	v := strings.TrimPrefix(r.Tag, "v")
	if r.Draft || r.Prerelease || r.Tag != "v"+v || !newer(v, current) || r.URL != "https://github.com/"+Repository+"/releases/tag/"+r.Tag {
		return "", asset{}, asset{}, errors.New("release is not a newer supported stable version")
	}
	setupName := "ED-Copilot-" + v + "-Setup.exe"
	var setup, sums asset
	for _, a := range r.Assets {
		switch a.Name {
		case setupName:
			if setup.Name != "" {
				return "", setup, sums, errors.New("duplicate setup asset")
			}
			setup = a
		case "SHA256SUMS.txt":
			if sums.Name != "" {
				return "", setup, sums, errors.New("duplicate checksum asset")
			}
			sums = a
		}
	}
	if _, err := validateAsset(setup, r.Tag, maxInstaller); err != nil {
		return "", setup, sums, err
	}
	if _, err := validateAsset(sums, r.Tag, maxChecksums); err != nil {
		return "", setup, sums, err
	}
	return v, setup, sums, nil
}

func checksumFor(body []byte, name string) (string, error) {
	var found string
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 67 || !digestPattern.MatchString(line[:64]) || (line[64:66] != "  " && line[64:66] != " *") {
			return "", errors.New("malformed release checksums")
		}
		file := line[66:]
		if file == name {
			if found != "" {
				return "", errors.New("duplicate setup checksum")
			}
			found = line[:64]
		}
	}
	if scanner.Err() != nil || found == "" {
		return "", errors.New("setup checksum is missing")
	}
	return found, nil
}

// Check stages only; it cannot run an installer or interrupt an active session.
func (m *Manager) Check(ctx context.Context) (result error) {
	m.checking.Lock()
	defer m.checking.Unlock()
	if !m.Snapshot().Enabled {
		return nil
	}
	ctx, cancelCheck := context.WithCancel(ctx)
	m.mu.Lock()
	m.activeCancel = cancelCheck
	enabled := m.state.Enabled
	m.mu.Unlock()
	defer func() { cancelCheck(); m.mu.Lock(); m.activeCancel = nil; m.mu.Unlock() }()
	if !enabled {
		return nil
	}
	m.change(func(s *State) { s.Status = "checking"; s.Error = "" })
	defer func() {
		if result != nil && ctx.Err() == nil {
			m.change(func(s *State) { s.Status = "error"; s.Error = result.Error() })
		}
	}()
	metaCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	body, err := m.get(metaCtx, LatestURL, maxMetadata)
	cancel()
	if err != nil {
		return err
	}
	var r release
	if err = json.Unmarshal(body, &r); err != nil {
		return errors.New("invalid update release metadata")
	}
	if !r.Draft && !r.Prerelease && r.Tag == "v"+m.opts.CurrentVersion {
		m.change(func(s *State) { s.Status = "current"; s.LastChecked = time.Now().UTC().Format(time.RFC3339) })
		return nil
	}
	v, setup, sums, err := selectAssets(r, m.opts.CurrentVersion)
	if err != nil {
		return err
	}
	m.change(func(s *State) {
		s.AvailableVersion = v
		s.ReleaseURL = r.URL
		s.LastChecked = time.Now().UTC().Format(time.RFC3339)
	})
	if !m.installed(m.opts.Executable) {
		m.change(func(s *State) { s.Status = "portable" })
		return nil
	}
	if p, err := m.readPending(); err == nil && p.Version == v && m.verifyPending(p) == nil {
		m.change(func(s *State) { s.Status = "staged"; s.Staged = true })
		return nil
	}
	checksumCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	checksums, err := m.get(checksumCtx, sums.URL, maxChecksums)
	cancel()
	if err != nil {
		return err
	}
	sumDigest, _ := validateAsset(sums, r.Tag, maxChecksums)
	if hashBytes(checksums) != sumDigest || int64(len(checksums)) != sums.Size {
		return errors.New("checksum asset failed digest verification")
	}
	expected, err := checksumFor(checksums, setup.Name)
	if err != nil {
		return err
	}
	assetDigest, _ := validateAsset(setup, r.Tag, maxInstaller)
	if expected != assetDigest {
		return errors.New("GitHub and release setup checksums disagree")
	}
	stage := filepath.Join(m.root, v)
	if err = os.MkdirAll(stage, 0700); err != nil {
		return errors.New("cannot create update staging directory")
	}
	m.change(func(s *State) { s.Status = "downloading" })
	if err = m.downloadSetup(ctx, setup, filepath.Join(stage, "setup.exe"), expected); err != nil {
		return err
	}
	if !m.Snapshot().Enabled {
		return nil
	}
	p := pending{Version: v, FromVersion: m.opts.CurrentVersion, ReleaseURL: r.URL, Body: truncate(r.Body, 32768), SHA256: expected, Size: setup.Size, Executable: m.opts.Executable}
	if err = writeJSON(filepath.Join(m.root, "pending.json"), p); err != nil {
		return errors.New("cannot persist verified update")
	}
	m.change(func(s *State) { s.Status = "staged"; s.Staged = true })
	return nil
}

func (m *Manager) downloadSetup(ctx context.Context, a asset, destination, expected string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return errors.New("invalid setup request")
	}
	req.Header.Set("User-Agent", "ED-Copilot-Updater/"+m.opts.CurrentVersion)
	response, err := m.client.Do(req)
	if err != nil {
		return errors.New("setup download unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxInstaller || (response.ContentLength >= 0 && response.ContentLength != a.Size) {
		return errors.New("setup response has unexpected size or status")
	}
	tmp := destination + ".partial"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("cannot write staged setup")
	}
	defer os.Remove(tmp)
	h := sha256.New()
	count, copyErr := io.Copy(io.MultiWriter(file, h), io.LimitReader(response.Body, a.Size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || count != a.Size || hex.EncodeToString(h.Sum(nil)) != expected {
		return errors.New("setup failed size or digest verification")
	}
	if err = os.Rename(tmp, destination); err != nil {
		return errors.New("cannot commit staged setup")
	}
	return nil
}

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
func (m *Manager) readPending() (pending, error) {
	var p pending
	f, err := os.Open(filepath.Join(m.root, "pending.json"))
	if err != nil {
		return p, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil {
		return p, err
	}
	if len(b) > 256<<10 {
		return p, errors.New("update manifest exceeds size limit")
	}
	err = json.Unmarshal(b, &p)
	return p, err
}
func (m *Manager) verifyPending(p pending) error {
	if _, err := version(p.Version); err != nil {
		return err
	}
	if _, err := version(p.FromVersion); err != nil {
		return err
	}
	if !newer(p.Version, p.FromVersion) || !digestPattern.MatchString(p.SHA256) || p.Size <= 0 || p.Size > maxInstaller || p.ReleaseURL != "https://github.com/"+Repository+"/releases/tag/v"+p.Version || !samePath(p.Executable, m.opts.Executable) {
		return errors.New("invalid staged update")
	}
	return verifyFile(filepath.Join(m.root, p.Version, "setup.exe"), p.Size, p.SHA256)
}
func verifyFile(path string, size int64, digest string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.New("staged update file unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return errors.New("staged update file size changed")
	}
	h := sha256.New()
	_, err = io.Copy(h, io.LimitReader(f, maxInstaller+1))
	if err != nil || hex.EncodeToString(h.Sum(nil)) != digest {
		return errors.New("staged update checksum changed")
	}
	return nil
}
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

// ApplyOnExit must be called only after a full, clean host shutdown. Closing to
// the tray must not call it. A user-initiated exit normally passes restart=false.
func (m *Manager) ApplyOnExit(restart bool) error {
	m.checking.Lock()
	defer m.checking.Unlock()
	if !m.Snapshot().Enabled {
		return nil
	}
	p, err := m.readPending()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !newer(p.Version, m.opts.CurrentVersion) {
		return nil
	}
	if !m.installed(m.opts.Executable) {
		return errors.New("automatic installation requires the existing installed application")
	}
	if err = m.verifyPending(p); err != nil {
		return err
	}
	p.ParentPID = os.Getpid()
	p.Restart = restart
	p.Applied = false
	if err = writeJSON(filepath.Join(m.root, "pending.json"), p); err != nil {
		return err
	}
	helper := filepath.Join(m.root, p.Version, "update-helper.exe")
	if err = copyExecutable(m.opts.Executable, helper); err != nil {
		return err
	}
	return m.launch(helper, []string{"--apply-update", filepath.Join(m.root, "pending.json")})
}

// ConsumeNotice returns release notes once, only after the helper successfully
// installed exactly the compiled version now running. Checking alone never
// creates a changelog notification.
func (m *Manager) ConsumeNotice() (*Notice, error) {
	p, err := m.readPending()
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !p.Applied || p.Version != m.opts.CurrentVersion || !newer(p.Version, p.FromVersion) {
		return nil, nil
	}
	if err = m.verifyPending(p); err != nil {
		return nil, err
	}
	marker := filepath.Join(m.root, "notified-"+p.Version+".json")
	if _, err = os.Stat(marker); err == nil {
		return nil, nil
	}
	if err = writeJSON(marker, map[string]string{"version": p.Version}); err != nil {
		return nil, err
	}
	return &Notice{Version: p.Version, FromVersion: p.FromVersion, Body: p.Body, ReleaseURL: p.ReleaseURL}, nil
}

func copyExecutable(source, destination string) error {
	from, err := os.Open(source)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, err = io.Copy(to, from)
	closeErr := to.Close()
	if err != nil {
		return err
	}
	return closeErr
}
