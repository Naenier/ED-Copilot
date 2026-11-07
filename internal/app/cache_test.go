package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/profile"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func remoteCacheFixture() M {
	return M{"name": "Synthetic Origin", "address": json.Number("18446744073709551615"), "coordinates": []float64{0, 0, 0}, "bodies": []M{
		{"id": 1, "name": "Synthetic Origin 1", "source": "spansh", "timestamp": "2026-11-07T12:05:00Z", "landable": false, "gravity": 9, "materials": []M{{"key": "polonium", "percent": .2}}, "rings": []M{{"name": "Synthetic Origin 1 A Ring", "type": "Icy", "signals": []M{{"key": "painite", "count": 9}}}}},
		{"id": 2, "name": "Synthetic Origin 2", "source": "spansh", "timestamp": "2026-11-07T12:05:00Z", "landable": true, "reserve": "PristineResources", "materials": []M{{"key": "polonium", "percent": .2}}, "rings": []M{{"name": "Synthetic Origin 2 A Ring", "type": "Metallic", "signals": []M{{"key": "platinum", "count": 1}}}}},
	}}
}
func cachedBody(t *testing.T, state M, name string) M {
	t.Helper()
	for _, system := range v.Rows(state["world"]) {
		for _, body := range v.Rows(system["bodies"]) {
			if body["name"] == name {
				return body
			}
		}
	}
	t.Fatal("cached body missing", name)
	return nil
}
func TestLegacyOnlineCacheSurvivesReplayRestartAndJournalPriority(t *testing.T) {
	old, file, dir := fixtureEngine(t, nil)
	old.Close()
	legacy := filepath.Join(dir, "online-cache.json")
	if err := profile.Write(legacy, []M{remoteCacheFixture()}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(Options{DataDir: dir, JournalDir: filepath.Dir(file), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	assert := func() {
		t.Helper()
		state := e.Snapshot()
		local := cachedBody(t, state, "Synthetic Origin 1")
		remote := cachedBody(t, state, "Synthetic Origin 2")
		if v.Num(local["gravity"]) != 1 || local["landable"] != true || v.Num(v.Rows(local["materials"])[0]["percent"]) != .2 {
			t.Fatal("cache replaced local scan or failed to supplement materials", local)
		}
		if !v.Bool(remote["remoteOnly"]) || len(v.Rows(remote["rings"])) != 1 {
			t.Fatal("remote discovery lost", remote)
		}
		found := false
		for _, target := range v.Rows(state["targets"]) {
			if target["body"] == "Synthetic Origin 1" && target["kind"] == "ring" {
				hotspots := v.Rows(target["hotspots"])
				if len(hotspots) != 1 || hotspots[0]["key"] != "platinum" {
					t.Fatal("remote signals overrode local scanner", hotspots)
				}
			}
			if target["body"] == "Synthetic Origin 2" && target["kind"] == "ring" {
				found = true
			}
		}
		if !found {
			t.Fatal("offline target absent after cache restore")
		}
	}
	assert()
	records := e.onlineCache.Records()
	if len(records) != 1 || records[0]["address"] != json.Number("18446744073709551615") {
		t.Fatal("cache ID64 rounded", records)
	}
	if _, err = os.Stat(filepath.Join(dir, "online-cache-go-v2.json")); err != nil {
		t.Fatal("legacy cache not migrated", err)
	}
	e.MergeRemoteSystem(M{"name": "synthetic origin", "bodies": []M{{"id": 2, "name": "Synthetic Origin 2", "distanceLs": 123, "materials": []M{}, "rings": []M{{"name": "Synthetic Origin 2 A Ring", "signals": []M{{"key": "painite", "count": 2}}}}}}})
	remote := cachedBody(t, e.Snapshot(), "Synthetic Origin 2")
	if v.Num(remote["distanceLs"]) != 123 || len(v.Rows(remote["materials"])) != 1 || len(v.Rows(v.Rows(remote["rings"])[0]["signals"])) != 2 {
		t.Fatal("partial community update discarded prior fields", remote)
	}
	e.Reader.Rescan()
	e.Reader.Poll()
	assert()
	if b, err := os.ReadFile(legacy); err != nil || !bytes.Equal(b, before) {
		t.Fatal("legacy cache overwritten", err)
	}
	e.Close()
	restored, err := New(Options{DataDir: dir, DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if len(v.Rows(cachedBody(t, restored.Snapshot(), "Synthetic Origin 2")["materials"])) != 1 {
		t.Fatal("Go cache lost on restart")
	}
	if v.At(restored.Snapshot(), "connection", "journalDir") != filepath.Dir(file) {
		t.Fatal("saved journal directory ignored")
	}
}

func TestOnlineCacheBoundedMergeOrderAndCorruptRecovery(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "online-cache.json")
	rows := []M{}
	for i := 0; i < 302; i++ {
		rows = append(rows, M{"name": fmt.Sprintf("System %03d", i), "address": json.Number(fmt.Sprint(9007199254740999 + int64(i))), "coordinates": []float64{float64(i), 0, 0}, "bodies": []M{{"id": 1, "name": fmt.Sprintf("System %03d 1", i)}}})
	}
	if err := profile.Write(file, rows); err != nil {
		t.Fatal(err)
	}
	c, err := openOnlineCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Records(); len(got) != 300 || got[0]["name"] != "System 002" || got[299]["name"] != "System 301" {
		t.Fatal("cache retention order incorrect")
	}
	merged, err := c.Merge(M{"name": "system 002", "bodies": []M{{"id": 2, "name": "System 002 2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Rows(merged["bodies"])) != 2 || merged["address"] != json.Number("9007199254741001") {
		t.Fatal("duplicate-system merge lost bodies or exact address", merged)
	}
	got := c.Records()
	if len(got) != 300 || got[299]["name"] != "system 002" {
		t.Fatal("updated record not most recent")
	}
	got[299]["name"] = "mutated caller"
	if c.Records()[299]["name"] != "system 002" {
		t.Fatal("cache snapshot exposed shared storage")
	}
	if _, err = c.Merge(M{"name": ""}); err == nil {
		t.Fatal("nameless cache record accepted")
	}
	if err = os.WriteFile(filepath.Join(dir, "online-cache-go-v2.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err = openOnlineCache(dir); err == nil || len(c.Records()) != 0 {
		t.Fatal("corrupt cache did not report bounded empty fallback", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "online-cache-go-v2.json"), []byte("[] {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = openOnlineCache(dir); err == nil {
		t.Fatal("trailing invalid cache content accepted")
	}
}
