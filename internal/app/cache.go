package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxCachedSystems = 300
const maxOnlineCacheBytes = 64 * 1024 * 1024

// onlineCache stores community observations independently of journal replay.
// The legacy file stays unchanged so the previous application can still run.
type onlineCache struct {
	mu      sync.Mutex
	file    string
	records []M
}

func openOnlineCache(dir string) (*onlineCache, error) {
	c := &onlineCache{file: filepath.Join(dir, "online-cache-go-v2.json"), records: []M{}}
	rows, err := readOnlineCache(c.file)
	legacy := false
	if os.IsNotExist(err) {
		rows, err = readOnlineCache(filepath.Join(dir, "online-cache.json"))
		legacy = err == nil
	}
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	for _, row := range rows {
		if cacheName(row["name"]) != "" {
			c.merge(row)
		}
	}
	if legacy {
		err = profile.Write(c.file, c.records)
	}
	return c, err
}
func readOnlineCache(file string) ([]M, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxOnlineCacheBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxOnlineCacheBytes {
		return nil, errors.New("Слишком большой кэш найденных систем")
	}
	d := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(raw, []byte{239, 187, 191})))
	d.UseNumber()
	var records []M
	if err = d.Decode(&records); err != nil {
		return nil, fmt.Errorf("Кэш найденных систем: %w", err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return nil, errors.New("Некорректное продолжение кэша систем")
	}
	return records, nil
}
func cacheName(raw any) string { return strings.ToLower(strings.TrimSpace(v.Str(raw))) }
func (c *onlineCache) Records() []M {
	c.mu.Lock()
	defer c.mu.Unlock()
	return v.Rows(v.Clone(c.records))
}
func (c *onlineCache) merge(record M) {
	key := cacheName(record["name"])
	if key == "" {
		return
	}
	next := []M{}
	var previous M
	for _, r := range c.records {
		if cacheName(r["name"]) == key {
			previous = r
		} else {
			next = append(next, r)
		}
	}
	next = append(next, mergeRemoteRecords(previous, record))
	if len(next) > maxCachedSystems {
		next = next[len(next)-maxCachedSystems:]
	}
	c.records = next
}
func (c *onlineCache) Merge(record M) (M, error) {
	if cacheName(record["name"]) == "" {
		return nil, errors.New("У найденной системы нет названия")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.merge(record)
	return v.Copy(c.records[len(c.records)-1]), profile.Write(c.file, c.records)
}

func mergeRemoteRecords(previous, incoming M) M {
	out := v.Copy(previous)
	for key, value := range incoming {
		if value != nil {
			out[key] = v.Clone(value)
		}
	}
	for _, key := range []string{"address", "coordinates"} {
		if incoming[key] == nil {
			out[key] = previous[key]
		}
	}
	bodies := v.Rows(v.Clone(previous["bodies"]))
	for _, body := range v.Rows(incoming["bodies"]) {
		if cacheName(body["name"]) == "" {
			continue
		}
		index := -1
		for i, old := range bodies {
			if cacheName(old["name"]) == cacheName(body["name"]) {
				index = i
				break
			}
		}
		var old M
		if index >= 0 {
			old = bodies[index]
		}
		next := v.Copy(old)
		for key, value := range body {
			if value != nil && value != "" {
				next[key] = v.Clone(value)
			}
		}
		if len(v.Rows(body["materials"])) == 0 {
			next["materials"] = v.Clone(old["materials"])
		}
		next["signals"] = mergeRemoteSignals(v.Rows(old["signals"]), v.Rows(body["signals"]))
		rings := v.Rows(v.Clone(old["rings"]))
		for _, ring := range v.Rows(body["rings"]) {
			if cacheName(ring["name"]) == "" {
				continue
			}
			at := -1
			for i, prior := range rings {
				if cacheName(prior["name"]) == cacheName(ring["name"]) {
					at = i
					break
				}
			}
			var prior M
			if at >= 0 {
				prior = rings[at]
			}
			r := v.Merge(v.Copy(prior), v.Copy(ring))
			r["signals"] = mergeRemoteSignals(v.Rows(prior["signals"]), v.Rows(ring["signals"]))
			if at >= 0 {
				rings[at] = r
			} else {
				rings = append(rings, r)
			}
		}
		next["rings"] = rings
		if index >= 0 {
			bodies[index] = next
		} else {
			bodies = append(bodies, next)
		}
	}
	out["bodies"] = bodies
	return out
}
func mergeRemoteSignals(old, incoming []M) []M {
	out := v.Rows(v.Clone(old))
	for _, s := range incoming {
		key := v.Key(v.First(s["key"], s["Type"], s["name"]))
		if key == "" {
			continue
		}
		at := -1
		for i, prior := range out {
			if v.Key(v.First(prior["key"], prior["Type"], prior["name"])) == key {
				at = i
				break
			}
		}
		if at >= 0 {
			out[at] = v.Copy(s)
		} else {
			out = append(out, v.Copy(s))
		}
	}
	return out
}

func (e *Engine) MergeRemoteSystem(system M) {
	if e.onlineCache != nil {
		merged, err := e.onlineCache.Merge(system)
		if merged != nil {
			system = merged
		}
		if err != nil {
			e.Notice("Не удалось сохранить найденную систему: " + err.Error())
		}
	}
	e.domainMu.Lock()
	e.Mining.MergeRemoteSystem(system)
	e.domainMu.Unlock()
	e.Notify()
}
func (e *Engine) restoreRemoteCache() {
	if e.onlineCache == nil {
		return
	}
	records := e.onlineCache.Records()
	e.domainMu.Lock()
	for _, system := range records {
		e.Mining.MergeRemoteSystem(system)
	}
	e.domainMu.Unlock()
	e.Notify()
}
