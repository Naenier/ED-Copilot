package secondscreen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	cookieName     = "__Host-edcopilot-screen"
	maxDevices     = 4
	pairWindow     = 4 * time.Minute
	deviceLifetime = 12 * time.Hour
)

var allowedModes = []string{"navigation", "expedition", "engineering", "colonization"}

type Row struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type Card struct {
	Title string       `json:"title"`
	Rows  []Row        `json:"rows"`
	Map   [][2]float64 `json:"map,omitempty"`
}
type View struct {
	Language string `json:"language"`
	Cards    []Card `json:"cards"`
}
type Screen struct {
	ID       string    `json:"id"`
	Address  string    `json:"address"`
	PairedAt time.Time `json:"pairedAt"`
	LastSeen time.Time `json:"lastSeen"`
}
type Status struct {
	Running     bool      `json:"running"`
	URL         string    `json:"url,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	Code        string    `json:"code,omitempty"`
	CodeExpires time.Time `json:"codeExpires,omitempty"`
	Modes       []string  `json:"modes"`
	Devices     []Screen  `json:"devices"`
	Addresses   []string  `json:"addresses"`
}

type cachedView struct {
	view       View
	until      time.Time
	generation uint64
}

type session struct {
	mu                     sync.Mutex
	cacheMu                sync.Mutex
	server                 *http.Server
	url, host, fingerprint string
	code                   string
	codeExpires            time.Time
	attempts               int
	modes                  []string
	devices                map[string]Screen
	cache                  map[string]cachedView
	view                   func(string) View
	generation             *atomic.Uint64
}

type Service struct {
	mu         sync.Mutex
	running    *session
	view       func(string) View
	generation atomic.Uint64
}

func New(view func(string) View) *Service { return &Service{view: view} }

func (s *Service) Invalidate() { s.generation.Add(1) }

func Addresses() []string {
	addresses, _ := net.InterfaceAddrs()
	out := []string{}
	for _, addr := range addresses {
		prefix, err := netip.ParsePrefix(addr.String())
		if err == nil && prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
			out = append(out, prefix.Addr().String())
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (s *Service) Start(host string, modes []string) error {
	if !slices.Contains(Addresses(), host) && host != "127.0.0.1" {
		return errors.New("Выберите локальный IP-адрес компьютера.")
	}
	return s.start(host, modes)
}

func (s *Service) start(host string, modes []string) error {
	if len(modes) == 0 || len(modes) > len(allowedModes) {
		return errors.New("Выберите карточки дополнительного экрана.")
	}
	enabled := []string{}
	for _, mode := range modes {
		if !slices.Contains(allowedModes, mode) || slices.Contains(enabled, mode) {
			return errors.New("Выберите карточки дополнительного экрана.")
		}
		enabled = append(enabled, mode)
	}
	cert, fingerprint, err := certificate(host)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(host, "0"))
	if err != nil {
		return errors.New("Не удалось открыть дополнительный экран.")
	}
	active := &session{host: listener.Addr().String(), fingerprint: fingerprint, view: s.view, generation: &s.generation, modes: enabled, devices: map[string]Screen{}, cache: map[string]cachedView{}}
	active.url = "https://" + active.host
	if err = active.rotate(); err != nil {
		_ = listener.Close()
		return err
	}
	active.server = &http.Server{Handler: http.HandlerFunc(active.serve), TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, ReadHeaderTimeout: 4 * time.Second, ReadTimeout: 6 * time.Second, WriteTimeout: 8 * time.Second, IdleTimeout: 20 * time.Second, MaxHeaderBytes: 8 << 10}
	s.mu.Lock()
	if s.running != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return errors.New("Дополнительный экран уже открыт.")
	}
	s.running = active
	s.mu.Unlock()
	go func() {
		_ = active.server.Serve(tls.NewListener(listener, active.server.TLSConfig))
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.running == active {
			s.running = nil
		}
	}()
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	active := s.running
	s.running = nil
	s.mu.Unlock()
	if active != nil {
		_ = active.server.Close()
	}
}

func (s *Service) Status() Status {
	s.mu.Lock()
	active := s.running
	s.mu.Unlock()
	out := Status{Addresses: Addresses(), Modes: []string{}, Devices: []Screen{}}
	if active == nil {
		return out
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	now := time.Now()
	out.Running, out.URL, out.Fingerprint = true, active.url, active.fingerprint
	out.Modes = append([]string(nil), active.modes...)
	if now.Before(active.codeExpires) && active.attempts < 8 {
		out.Code, out.CodeExpires = active.code, active.codeExpires
	}
	for key, device := range active.devices {
		if now.Sub(device.PairedAt) >= deviceLifetime {
			delete(active.devices, key)
		} else {
			out.Devices = append(out.Devices, device)
		}
	}
	slices.SortFunc(out.Devices, func(a, b Screen) int { return a.PairedAt.Compare(b.PairedAt) })
	return out
}

func (s *Service) Rotate() error {
	s.mu.Lock()
	active := s.running
	s.mu.Unlock()
	if active == nil {
		return errors.New("Откройте дополнительный экран.")
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	return active.rotate()
}

func (s *Service) Revoke(id string) {
	s.mu.Lock()
	active := s.running
	s.mu.Unlock()
	if active == nil {
		return
	}
	active.mu.Lock()
	defer active.mu.Unlock()
	for key, screen := range active.devices {
		if screen.ID == id {
			delete(active.devices, key)
			return
		}
	}
}

func (s *session) rotate() error {
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return err
	}
	s.code = fmt.Sprintf("%08d", n.Int64())
	s.codeExpires = time.Now().Add(pairWindow)
	s.attempts = 0
	return nil
}

func certificate(host string) (tls.Certificate, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return tls.Certificate{}, "", err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "ED Copilot Second Screen"}, IPAddresses: []net.IP{net.ParseIP(host)}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	digest := sha256.Sum256(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, strings.ToUpper(hex.EncodeToString(digest[:])), nil
}

func (s *session) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
	if r.Host != s.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.url) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	case r.Method == "GET" && r.URL.Path == "/screen.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(script))
	case r.Method == "GET" && r.URL.Path == "/screen.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write([]byte(style))
	case r.Method == "POST" && r.URL.Path == "/api/pair":
		s.pair(w, r)
	case r.Method == "GET" && r.URL.Path == "/api/state":
		s.state(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *session) pair(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || r.Header.Get("Origin") != s.url {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var request struct {
		Code string `json:"code"`
	}
	reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128))
	if reader.Decode(&request) != nil || len(request.Code) != 8 || reader.Decode(new(any)) != io.EOF {
		http.Error(w, "Bad code", http.StatusBadRequest)
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, device := range s.devices {
		if now.Sub(device.PairedAt) >= deviceLifetime {
			delete(s.devices, token)
		}
	}
	if s.code == "" || !now.Before(s.codeExpires) || s.attempts >= 8 || len(s.devices) >= maxDevices {
		http.Error(w, "Pairing is closed", http.StatusTooManyRequests)
		return
	}
	s.attempts++
	if subtle.ConstantTimeCompare([]byte(request.Code), []byte(s.code)) != 1 {
		http.Error(w, "Invalid code", http.StatusForbidden)
		return
	}
	var entropy [40]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		http.Error(w, "Unavailable", http.StatusServiceUnavailable)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(entropy[:32])
	id := hex.EncodeToString(entropy[32:])
	address, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.devices[token] = Screen{ID: id, Address: address, PairedAt: now}
	s.code = ""
	s.codeExpires = time.Time{}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(deviceLifetime.Seconds())})
	w.WriteHeader(http.StatusNoContent)
}

func (s *session) state(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		http.Error(w, "Pair first", http.StatusUnauthorized)
		return
	}
	now := time.Now()
	s.mu.Lock()
	device, ok := s.devices[cookie.Value]
	if !ok || now.Sub(device.PairedAt) >= deviceLifetime {
		delete(s.devices, cookie.Value)
		s.mu.Unlock()
		http.Error(w, "Pair first", http.StatusUnauthorized)
		return
	}
	if now.Sub(device.LastSeen) < time.Second {
		s.mu.Unlock()
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	device.LastSeen = now
	s.devices[cookie.Value] = device
	mode := r.URL.Query().Get("mode")
	if !slices.Contains(s.modes, mode) {
		mode = s.modes[0]
	}
	s.mu.Unlock()
	generation := s.generation.Load()
	s.cacheMu.Lock()
	cached := s.cache[mode]
	if !now.Before(cached.until) || cached.generation != generation {
		view := View{}
		if s.view != nil {
			view = s.view(mode)
		}
		cached = cachedView{view: view, until: time.Now().Add(2 * time.Second), generation: generation}
		s.cache[mode] = cached
	}
	s.cacheMu.Unlock()
	s.mu.Lock()
	_, stillPaired := s.devices[cookie.Value]
	s.mu.Unlock()
	if !stillPaired {
		http.Error(w, "Pair first", http.StatusUnauthorized)
		return
	}
	if generation != s.generation.Load() {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		View
		Mode  string   `json:"mode"`
		Modes []string `json:"modes"`
	}{cached.view, mode, s.modes})
}
