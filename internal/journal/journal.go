// Package journal incrementally reads complete UTF-8 journal lines without writing game files.
package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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
	identities                       map[string]string
	side                             map[string][]byte
	activeFID                        string
	scanning                         bool
	filesRead, total, events         int
	lastEvent, lastUpdate, errorText string
	rescan                           bool
	previousNames                    []string
}

func New(dir string, apply Callback, reset func()) *Reader {
	return &Reader{Dir: dir, Apply: apply, Reset: reset, positions: map[string]int64{}, identities: map[string]string{}, side: map[string][]byte{}, rescan: true}
}
func Decode(b []byte) (M, error) {
	d := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(b, []byte{239, 187, 191})))
	d.UseNumber()
	var m M
	e := d.Decode(&m)
	return m, e
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
	r.identities = map[string]string{}
	r.mu.Unlock()
	return nil
}
func (r *Reader) Rescan() { r.mu.Lock(); r.rescan = true; r.mu.Unlock() }
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
	for _, name := range names {
		present[name] = true
		if st, err := os.Stat(filepath.Join(r.Dir, name)); err == nil && st.Size() < r.positions[name] {
			r.rescan = true
			delete(r.identities, name)
		}
		if r.identities[name] == "" {
			fid, e := fileIdentity(filepath.Join(r.Dir, name))
			if e != nil {
				r.errorText = e.Error()
				return
			}
			r.identities[name] = fid
		}
	}
	for name := range r.identities {
		if !present[name] {
			delete(r.identities, name)
		}
	}
	latest := ""
	for i := len(names) - 1; i >= 0; i-- {
		if fid := r.identities[names[i]]; fid != "" {
			latest = fid
			break
		}
	}
	if latest != r.activeFID {
		r.activeFID = latest
		r.rescan = true
	}
	selected := []string{}
	for _, name := range names {
		fid := r.identities[name]
		if fid == "" || latest == "" || fid == latest {
			selected = append(selected, name)
		}
	}
	for _, name := range selected {
		if st, e := os.Stat(filepath.Join(r.Dir, name)); e == nil && st.Size() < r.positions[name] {
			r.rescan = true
		}
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
		if e = r.read(name, replay); e != nil {
			r.errorText = e.Error()
			r.rescan = true
			break
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
	for _, name := range []string{"Status.json", "Cargo.json", "Market.json", "NavRoute.json", "Outfitting.json", "Shipyard.json", "ShipLocker.json"} {
		b, e := os.ReadFile(filepath.Join(r.Dir, name))
		if e != nil || len(b) > 8*1024*1024 || bytes.Equal(r.side[name], b) {
			continue
		}
		event, e := Decode(b)
		if e != nil {
			continue
		}
		r.side[name] = append([]byte(nil), b...)
		if event["event"] == nil {
			event["event"] = strings.TrimSuffix(name, ".json")
		}
		r.dispatch(event, M{"file": name, "replay": replay, "side": true})
	}
}
func fileIdentity(file string) (string, error) {
	f, e := os.Open(file)
	if e != nil {
		return "", e
	}
	defer f.Close()
	b := make([]byte, 64*1024)
	n, e := f.Read(b)
	if e != nil && e != io.EOF {
		return "", e
	}
	for _, line := range bytes.Split(b[:n], []byte{'\n'}) {
		m, e := Decode(line)
		if e == nil && (m["event"] == "Commander" || m["event"] == "LoadGame") && v.Str(m["FID"]) != "" {
			return v.Str(m["FID"]), nil
		}
	}
	return "", nil
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
				if parseErr == nil && v.Str(event["event"]) != "" {
					r.dispatch(event, M{"file": name, "offset": offset, "replay": replay})
					r.events++
					r.lastEvent = v.Str(event["timestamp"])
					r.lastUpdate = v.ISO(time.Now())
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
