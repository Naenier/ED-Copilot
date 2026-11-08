package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestExobiologyJournalReplayAndActivityScope(t *testing.T) {
	e, file, dir := fixtureEngine(t, nil)
	events := []M{
		testEvent("Location", M{"StarSystem": "Synthetic Exo", "SystemAddress": json.Number("18446744073709551613"), "BodyID": 1, "BodyName": "Synthetic Exo 1", "BodyType": "Planet", "Docked": false, "StarPos": []float64{0, 0, 0}}),
		testEvent("Scan", M{"SystemAddress": json.Number("18446744073709551613"), "BodyID": 1, "BodyName": "Synthetic Exo 1", "Landable": true, "AtmosphereType": "Ammonia", "SurfacePressure": 1700, "SurfaceTemperature": 175, "SurfaceGravity": 1, "Radius": 1000000, "PlanetClass": "High metal content body"}),
		testEvent("SAASignalsFound", M{"SystemAddress": json.Number("18446744073709551613"), "BodyID": 1, "BodyName": "Synthetic Exo 1", "Signals": []M{{"Type": "$SAA_SignalType_Biological;", "Count": 3}}, "Genuses": []M{{"Genus": "$Codex_Ent_Stratum_Genus_Name;"}}}),
	}
	for _, kind := range []string{"Log", "Sample", "Sample", "Analyse"} {
		events = append(events, testEvent("ScanOrganic", M{"SystemAddress": json.Number("18446744073709551613"), "Body": 1, "ScanType": kind, "Species": "$Codex_Ent_Stratum_01_Name;", "Variant": "$Codex_Ent_Stratum_01_m_Name;", "Genus": "$Codex_Ent_Stratum_Genus_Name;"}))
	}
	writeEvents(t, file, true, events)
	e.Reader.Poll()
	state := e.SnapshotForUI("exobiology")
	ex := v.Obj(state["exobiology"])
	if v.Int(v.At(ex, "tracker", "sampleCount")) != 3 || len(v.Rows(ex["pending"])) != 1 {
		t.Fatal("journal sample chain lost", ex["tracker"], ex["summary"])
	}
	if len(p.Build("exobiology", "Образцы", "", state).Rows) != 1 {
		t.Fatal("specimen not visible in workspace")
	}
	doOK(t, e, "/api/settings", M{"activity": "exobiology"})
	hud := e.SnapshotForHUD()
	if len(v.Obj(hud["exobiology"])) == 0 {
		t.Fatal("exobiology activity lacks snapshot")
	}
	if !strings.Contains(hudTextForTest(p.BuildHUD(hud, time.Now())), "3 / 3") {
		t.Fatal("HUD lost sample progress")
	}
	doOK(t, e, "/api/settings", M{"activity": "flight"})
	if e.SnapshotForHUD()["exobiology"] != nil {
		t.Fatal("flight HUD eagerly builds biological catalogue")
	}
	if e.SnapshotForSync()["exobiology"] != nil {
		t.Fatal("sync includes unneeded catalogue")
	}
	bodyKey := v.Str(ex["currentBodyKey"])
	doOK(t, e, "/api/exobiology", M{"action": "bookmark", "bodyKey": bodyKey, "notes": "Private field note"})
	doOK(t, e, "/api/exobiology", M{"action": "configure", "minValue": 10000000, "includeBacterium": false})
	e.Close()
	restored, err := New(Options{DataDir: dir, JournalDir: strings.TrimSuffix(file, "Journal.2026-11-07T120000.01.log"), DisableNetwork: true, ManualPolling: true})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	snapshot := restored.SnapshotForUI("exobiology")
	if len(v.Rows(v.At(snapshot, "exobiology", "pending"))) != 1 || len(v.Rows(v.At(snapshot, "exobiology", "bookmarks"))) != 1 || v.At(snapshot, "exobiology", "settings", "includeBacterium") != false {
		t.Fatal("replay/save duplicate or lost biological state")
	}
}

func hudTextForTest(doc p.HUDDocument) string {
	out := ""
	for _, s := range doc.Sections {
		for _, l := range s.Lines {
			out += l.Text + "\n"
		}
	}
	return out
}

func TestExobiologyPublicSitesBoundedReadOnly(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	revision := e.SnapshotForUI("overview")["revision"]
	result, err := e.ExobiologySites(M{"query": "", "limit": 1000000, "radiusLy": 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Rows(result["entries"])) > 24 || v.Int(result["limit"]) > 24 {
		t.Fatal("unbounded public sites result")
	}
	for _, r := range v.Rows(result["entries"]) {
		if r["distanceLy"] != nil || r["coordinatesKnown"] != false {
			t.Fatal("approximate index presented as exact coordinates")
		}
	}
	if e.SnapshotForUI("overview")["revision"] != revision {
		t.Fatal("read-only sites search changed engine revision")
	}
	if _, err = e.ExobiologySites(M{"radiusLy": -1}); err == nil {
		t.Fatal("negative radius accepted")
	}
}
