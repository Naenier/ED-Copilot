// Package journal incrementally reads complete UTF-8 journal lines without writing game files.
package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type M = v.M
type Callback func(event, meta M)
type Reader struct {
	mu     sync.Mutex
	pollMu sync.Mutex
	Dir    string
	Apply  Callback
	Reset  func()
	// AfterReplay restores persisted observations after journal records have
	// rebuilt state, before current side files are applied. It runs unlocked.
	AfterReplay                      func()
	Notice                           func(string)
	positions                        map[string]int64
	identities                       map[string]journalIdentity
	side                             map[string][]byte
	active                           journalIdentity
	lastLogin                        time.Time
	scanning                         bool
	filesRead, total, events         int
	lastEvent, lastUpdate, errorText string
	rescan                           bool
	previousNames                    []string
}

type journalIdentity struct {
	fid, mode     string
	part          int
	inspectedSize int64
}

func New(dir string, apply Callback, reset func()) *Reader {
	return &Reader{Dir: dir, Apply: apply, Reset: reset, positions: map[string]int64{}, identities: map[string]journalIdentity{}, side: map[string][]byte{}, rescan: true}
}

// GameMode identifies release journals that can establish a persistent workspace.
func GameMode(version string) string {
	mode := journalMode(version)
	if mode == "live" || mode == "legacy" {
		return mode
	}
	return ""
}

func journalMode(version string) string {
	version = strings.ToLower(strings.TrimSpace(version))
	switch {
	case strings.Contains(version, "beta"), strings.Contains(version, "alpha"), strings.Contains(version, "pts"):
		return "test"
	case strings.HasPrefix(version, "4."):
		return "live"
	case strings.HasPrefix(version, "3."):
		return "legacy"
	default:
		return ""
	}
}

func Decode(b []byte) (M, error) {
	b = bytes.TrimPrefix(b, []byte{239, 187, 191})
	if !utf8.Valid(b) {
		return nil, errors.New("invalid UTF-8 journal record")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m M
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil || len(bytes.TrimSpace(b[d.InputOffset():])) != 0 {
		return nil, errors.New("expected one journal object")
	}
	if kind, _ := m["event"].(string); strings.EqualFold(kind, "fileheader") {
		m["event"] = "Fileheader"
	}
	return m, nil
}
func (r *Reader) State() M {
	r.mu.Lock()
	defer r.mu.Unlock()
	return M{"journalDir": r.Dir, "scanning": r.scanning, "filesRead": r.filesRead, "filesTotal": r.total, "eventCount": r.events, "lastEvent": r.lastEvent, "lastUpdate": r.lastUpdate, "error": r.errorText}
}
func (r *Reader) SetDirectory(dir string) error {
	dir, e := filepath.Abs(dir)
	if e != nil {
		return e
	}
	stat, e := os.Stat(dir)
	if e != nil || !stat.IsDir() {
		return fmt.Errorf("Папка журналов не найдена")
	}
	r.pollMu.Lock()
	defer r.pollMu.Unlock()
	r.mu.Lock()
	r.Dir = dir
	r.rescan = true
	r.identities = map[string]journalIdentity{}
	r.mu.Unlock()
	return nil
}
func (r *Reader) Rescan() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rescan = true
	r.identities = map[string]journalIdentity{}
}
func (r *Reader) Run(ctx context.Context) {
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		r.Poll()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (r *Reader) Poll() {
	r.pollMu.Lock()
	defer r.pollMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	entries, e := os.ReadDir(r.Dir)
	if e != nil {
		r.errorText = "Не удалось прочитать папку журналов: " + e.Error()
		return
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "Journal.") && strings.HasSuffix(entry.Name(), ".log") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	present := map[string]bool{}
	sizes := make(map[string]int64, len(names))
	for _, name := range names {
		present[name] = true
		file := filepath.Join(r.Dir, name)
		if st, err := os.Stat(file); err == nil {
			sizes[name] = st.Size()
			if st.Size() < r.positions[name] {
				r.rescan = true
				delete(r.identities, name)
			}
		}
		identity, cached := r.identities[name]
		size, known := sizes[name]
		if !cached || (identity.fid == "" && (!known || size != identity.inspectedSize)) {
			identity, e := fileIdentity(file)
			if e != nil {
				r.errorText = e.Error()
				return
			}
			identity.inspectedSize = size
			r.identities[name] = identity
		}
	}
	for name := range r.identities {
		if !present[name] {
			delete(r.identities, name)
		}
	}
	sessionOwners := map[string]journalIdentity{}
	for _, name := range names {
		identity, session := r.identities[name], SessionKey(name)
		if identity.fid == "" || session == "" {
			continue
		}
		if existing, found := sessionOwners[session]; found {
			if existing.fid != identity.fid || (existing.mode != "" && identity.mode != "" && existing.mode != identity.mode) {
				sessionOwners[session] = journalIdentity{}
				continue
			}
			identity.mode = v.Text(identity.mode, existing.mode)
		}
		sessionOwners[session] = identity
	}
	identityFor := func(name string) journalIdentity {
		identity := r.identities[name]
		if identity.fid == "" && identity.part > 1 {
			owner := sessionOwners[SessionKey(name)]
			if owner.fid != "" && (identity.mode == "" || owner.mode == "" || identity.mode == owner.mode) {
				identity.fid = owner.fid
				identity.mode = v.Text(identity.mode, owner.mode)
			}
		}
		return identity
	}
	var latest journalIdentity
	for i := len(names) - 1; i >= 0; i-- {
		if identity := identityFor(names[i]); identity.fid != "" {
			latest = identity
			break
		}
	}
	if latest.fid != r.active.fid || latest.mode != r.active.mode {
		r.active = latest
		r.rescan = true
	}
	selected := []string{}
	for _, name := range names {
		identity := identityFor(name)
		if identity.part > 0 && identity.fid == "" && latest.fid != "" {
			continue
		}
		if identity.fid != "" && latest.fid != "" && identity.fid != latest.fid {
			continue
		}
		if identity.mode != "" && latest.mode != "" && identity.mode != latest.mode {
			continue
		}
		selected = append(selected, name)
	}
	// Only appending newer sessions preserves event order. A removed session
	// or an older imported log requires a full rebuild of all derived totals.
	if len(selected) < len(r.previousNames) {
		r.rescan = true
	} else {
		for i, name := range r.previousNames {
			if selected[i] != name {
				r.rescan = true
				break
			}
		}
	}
	replay := r.rescan
	eventsBefore := r.events
	if replay {
		r.positions = map[string]int64{}
		r.side = map[string][]byte{}
		r.events = 0
		r.lastLogin = time.Time{}
		r.lastEvent, r.lastUpdate = "", ""
		r.rescan = false
		r.scanning = true
		if r.Reset != nil {
			r.mu.Unlock()
			r.Reset()
			r.mu.Lock()
		}
	}
	r.total = len(selected)
	r.filesRead = 0
	r.errorText = ""
	for _, name := range selected {
		if size, known := sizes[name]; !known || size != r.positions[name] {
			if e = r.read(name, replay); e != nil {
				r.errorText = e.Error()
				r.rescan = true
				break
			}
		}
		r.filesRead++
	}
	r.scanning = false
	if r.errorText == "" {
		r.previousNames = append([]string(nil), selected...)
	}
	if (replay || r.events != eventsBefore) && r.AfterReplay != nil {
		r.mu.Unlock()
		r.AfterReplay()
		r.mu.Lock()
	}
	if r.errorText != "" {
		return
	}
	for _, name := range []string{"Status.json", "Cargo.json", "Market.json", "NavRoute.json", "Outfitting.json", "Shipyard.json", "ShipLocker.json"} {
		b, e := readSideFile(filepath.Join(r.Dir, name))
		if e != nil || bytes.Equal(r.side[name], b) {
			continue
		}
		event, e := Decode(b)
		if e != nil {
			continue
		}
		kind := strings.TrimSuffix(name, ".json")
		actual, found := event["event"]
		switch {
		case !found, kind == "NavRoute" && actual == "Route":
			event["event"] = kind
		case actual == kind, kind == "NavRoute" && actual == "NavRouteClear", kind == "ShipLocker" && actual == "ShipLockerMaterials":
		default:
			continue
		}
		if at := v.Time(event["timestamp"]); !at.IsZero() && at.Before(r.lastLogin) {
			continue
		}
		r.side[name] = b
		r.dispatch(event, M{"file": name, "replay": replay, "side": true})
	}
}

// SessionKey groups the numbered journal parts for one game session.
func SessionKey(name string) string {
	if !strings.HasPrefix(name, "Journal.") || !strings.HasSuffix(name, ".log") {
		return ""
	}
	stem := strings.TrimSuffix(name, ".log")
	dot := strings.LastIndexByte(stem, '.')
	if dot < len("Journal.") {
		return ""
	}
	if part, err := strconv.Atoi(stem[dot+1:]); err != nil || part < 1 {
		return ""
	}
	return stem[:dot]
}

func readSideFile(file string) ([]byte, error) {
	const limit = 8 * 1024 * 1024
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > limit {
		return nil, errors.New("journal side file exceeds size limit")
	}
	return b, err
}

func fileIdentity(file string) (journalIdentity, error) {
	var identity journalIdentity
	f, e := os.Open(file)
	if e != nil {
		return identity, e
	}
	defer f.Close()
	b := make([]byte, 64*1024)
	n, e := io.ReadFull(f, b)
	if e != nil && e != io.EOF && e != io.ErrUnexpectedEOF {
		return identity, e
	}
	pending := b[:n]
	for {
		line, rest, complete := bytes.Cut(pending, []byte{'\n'})
		if !complete {
			break
		}
		pending = rest
		m, e := Decode(line)
		if e != nil {
			continue
		}
		kind, _ := m["event"].(string)
		switch kind {
		case "Fileheader":
			identity.mode = journalMode(v.Str(m["gameversion"]))
			identity.part = v.Int(m["part"])
		case "Commander", "LoadGame":
			if fid, _ := m["FID"].(string); identity.fid == "" && strings.TrimSpace(fid) != "" {
				identity.fid = fid
			}
			if kind == "LoadGame" && identity.mode == "" {
				identity.mode = journalMode(v.Str(m["gameversion"]))
			}
			if identity.fid != "" && (identity.mode != "" || kind == "LoadGame") {
				return identity, nil
			}
		}
	}
	return identity, nil
}
func (r *Reader) dispatch(event, meta M) {
	r.mu.Unlock()
	defer r.mu.Lock()
	if r.Apply != nil {
		r.Apply(event, meta)
	}
}
func (r *Reader) read(name string, replay bool) error {
	f, e := os.Open(filepath.Join(r.Dir, name))
	if e != nil {
		return e
	}
	defer f.Close()
	pos := r.positions[name]
	if _, e = f.Seek(pos, io.SeekStart); e != nil {
		return e
	}
	pending := make([]byte, 0, 128*1024)
	buffer := make([]byte, 128*1024)
	for {
		n, readErr := f.Read(buffer)
		if n > 0 {
			pending = append(pending, buffer[:n]...)
			if len(pending) > 4*1024*1024 {
				return fmt.Errorf("Слишком длинная строка журнала %s", name)
			}
			consumed := 0
			for {
				end := bytes.IndexByte(pending[consumed:], '\n')
				if end < 0 {
					break
				}
				end += consumed
				line := pending[consumed:end]
				offset := pos + int64(consumed)
				event, parseErr := Decode(line)
				kind, _ := event["event"].(string)
				if parseErr == nil && strings.TrimSpace(kind) != "" {
					r.dispatch(event, M{"file": name, "offset": offset, "replay": replay})
					r.events++
					r.lastEvent = v.Str(event["timestamp"])
					r.lastUpdate = v.ISO(time.Now())
					if kind == "Commander" || kind == "LoadGame" {
						if at := v.Time(event["timestamp"]); at.After(r.lastLogin) {
							r.lastLogin = at
						}
					}
				}
				consumed = end + 1
			}
			pos += int64(consumed)
			r.positions[name] = pos
			pending = append(pending[:0], pending[consumed:]...)
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
