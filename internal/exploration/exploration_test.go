package exploration

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const owner = "live-cmdr"

var base = time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)

func eventAt(kind string, minute int, raw M) M {
	return v.Merge(M{"event": kind, "timestamp": v.ISO(base.Add(time.Duration(minute) * time.Minute))}, raw)
}
func checkedUpdate(t *testing.T, s *Service, key string, raw M) M {
	t.Helper()
	out, err := s.Update(key, raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func journalFields(id any, name string) M {
	return M{"SystemAddress": id, "StarSystem": name}
}
func planet(id any, name string, number int, class string) M {
	return v.Merge(journalFields(id, name), M{"BodyID": number, "BodyName": fmt.Sprintf("%s %d", name, number), "PlanetClass": class, "MassEM": 1.0})
}

func TestExpeditionVisitScanMappingSaleAndLoss(t *testing.T) {
	s := New(nil)
	s.now = func() time.Time { return base }
	draft := checkedUpdate(t, s, owner, M{"action": "save-expedition", "name": "Beyond the bubble", "goal": "Explore worlds", "days": 2, "returnSystem": "Sol"})
	exp := v.Str(v.At(draft, "expedition", "id"))
	checkedUpdate(t, s, owner, M{"action": "start-expedition", "id": exp})
	id := json.Number("18446744073709551614")
	xy := M{"StarPos": []float64{10, 4, 2}}
	s.Apply(owner, eventAt("FSDJump", 1, v.Merge(journalFields(id, "Reach"), xy)), nil)
	s.Apply(owner, eventAt("FSSAllBodiesFound", 2, v.Merge(journalFields(id, "Reach"), M{"Count": 1})), nil)
	s.Apply(owner, eventAt("Scan", 3, planet(id, "Reach", 1, "Earthlike body")), nil)
	query := M{"mode": "expeditions", "expeditionId": exp, "systemAddress": string(id), "bodyId": "1"}
	data := s.Snapshot(owner, query)
	scanned := v.Int(v.At(data, "wallet", "onboardEstimate"))
	if scanned < 100000 || v.At(data, "system", "visitedAt") == "" || v.At(data, "system", "fssAt") == "" ||
		v.At(data, "body", "mappedAt") != nil || v.At(data, "body", "visitedAt") != nil {
		t.Fatal("visit, scan and mapping conflated", data["wallet"], data["system"], data["body"])
	}
	s.Apply(owner, eventAt("SupercruiseExit", 4, v.Merge(planet(id, "Reach", 1, "Earthlike body"), M{"Body": "Reach 1"})), nil)
	s.Apply(owner, eventAt("SAAScanComplete", 5, v.Merge(planet(id, "Reach", 1, "Earthlike body"), M{"ProbesUsed": 3, "EfficiencyTarget": 4})), nil)
	data = s.Snapshot(owner, query)
	mapped := v.Int(v.At(data, "wallet", "onboardEstimate"))
	if mapped <= scanned || v.At(data, "body", "mappedAt") == nil || v.At(data, "body", "visitedAt") == nil || v.At(data, "body", "dataState") != "onboard" {
		t.Fatal("mapping and body visit were not recorded", data["wallet"], data["body"])
	}
	// A death on foot preserves the cartographic data aboard the ship.
	s.Apply(owner, eventAt("Died", 6, M{"OnFoot": true}), M{"commander": M{"vehicle": "foot"}})
	if value := v.Int(v.At(s.Snapshot(owner, query), "wallet", "onboardEstimate")); value != mapped {
		t.Fatalf("foot death cleared estimate: %d", value)
	}
	sale := eventAt("MultiSellExplorationData", 7, M{"Discovered": []M{{"SystemName": "Reach", "NumBodies": 1}}, "TotalEarnings": 1400000})
	s.Apply(owner, sale, nil)
	s.Apply(owner, sale, M{"replay": true})
	data = s.Snapshot(owner, query)
	receipts := v.Rows(data["receipts"])
	if len(receipts) != 1 || v.Int(receipts[0]["credits"]) != 1400000 || v.Int(receipts[0]["estimated"]) != mapped ||
		v.Int(v.At(data, "wallet", "onboardEstimate")) != 0 || v.At(data, "body", "dataState") != "sold" {
		t.Fatal("named sale was not confirmed once", data["wallet"], receipts, data["body"])
	}
	s.Apply(owner, eventAt("Scan", 8, planet(id, "Reach", 1, "Earthlike body")), nil)
	if v.Int(v.At(s.Snapshot(owner, query), "wallet", "onboardEstimate")) != 0 {
		t.Fatal("sold scan put back aboard")
	}
	s.Apply(owner, eventAt("Scan", 9, planet(id, "Reach", 2, "Water world")), nil)
	s.Apply(owner, eventAt("Died", 10, nil), M{"commander": M{"vehicle": "ship"}})
	s = New(s.ExportSettings())
	s.Apply(owner, sale, M{"replay": true})
	query["bodyId"] = "2"
	data = s.Snapshot(owner, query)
	if v.Int(v.At(data, "wallet", "onboardEstimate")) != 0 || v.Int(v.At(data, "wallet", "lostEstimate")) <= 0 ||
		v.Int(v.At(data, "wallet", "confirmedCredits")) != 1400000 || v.At(data, "body", "dataState") != "lost" ||
		len(v.Rows(data["receipts"])) != 2 {
		t.Fatal("lost and sold states did not survive restart", data["wallet"], data["body"])
	}
}

func TestSurveyStagesImportExportAndBoxelBounds(t *testing.T) {
	s := New(nil)
	id := json.Number("9007199254740993")
	input := M{"action": "save-survey", "name": "Stars", "criterion": "mapping", "mappingTypes": []string{"earthlike"},
		"notes": "Plan notes", "targets": []M{{"system": "Reach", "address": string(id), "note": "Keep"}, {"system": "Next"}}}
	state := checkedUpdate(t, s, owner, input)
	project := v.Str(v.At(state, "survey", "id"))
	query := M{"mode": "surveys", "surveyId": project}
	progress := func(key string) M {
		list := v.Rows(v.At(s.Snapshot(key, query), "survey", "results", "rows"))
		return v.Obj(list[0]["progress"])
	}
	s.Apply(owner, eventAt("FSDJump", 1, journalFields(id, "Reach")), nil)
	if got := progress(owner); got["visitedAt"] == nil || got["fssAt"] != nil || got["mappedAt"] != nil {
		t.Fatal(got)
	}
	s.Apply(owner, eventAt("FSSAllBodiesFound", 2, v.Merge(journalFields(id, "Reach"), M{"Count": 2})), nil)
	if got := progress(owner); got["fssAt"] == nil || got["mappedAt"] != nil {
		t.Fatal(got)
	}
	s.Apply(owner, eventAt("Scan", 3, v.Merge(journalFields(id, "Reach"), M{"BodyID": 0, "BodyName": "Reach", "StarType": "G", "StellarMass": 1})), nil)
	s.Apply(owner, eventAt("Scan", 4, planet(id, "Reach", 1, "Earthlike body")), nil)
	if got := progress(owner); got["mappedAt"] != nil {
		t.Fatal(got)
	}
	s.Apply(owner, eventAt("SAAScanComplete", 5, planet(id, "Reach", 1, "Earthlike body")), nil)
	if got := progress(owner); got["mappedAt"] == nil {
		t.Fatal(got)
	}
	if next := v.At(s.Snapshot(owner, query), "survey", "next", "system"); next != "Next" {
		t.Fatal("wrong next system", next)
	}
	file, err := s.ExportSurvey(owner, project)
	if err != nil {
		t.Fatal(err)
	}
	imported := checkedUpdate(t, s, "other", M{"action": "import-survey", "text": file["text"]})
	newTargets := v.Rows(v.At(imported, "survey", "results", "rows"))
	if len(newTargets) != 2 || newTargets[0]["address"] != string(id) || newTargets[0]["note"] != "Keep" ||
		v.At(newTargets[0], "progress", "mappedAt") == nil || v.At(imported, "survey", "notes") != "Plan notes" ||
		v.Int(v.At(imported, "worlds", "total")) != 0 {
		t.Fatal("round-trip or commander isolation failed", imported)
	}
	if _, err := s.ExportSurvey("other", project); err == nil {
		t.Fatal("another commander exported the original survey")
	}
	newID := v.At(imported, "survey", "id")
	checkedUpdate(t, s, "other", v.Merge(v.Copy(input), M{"id": newID, "mappingTypes": []string{"water"}}))
	if got := progress("other"); got["mappedAt"] != nil {
		t.Fatal("imported mapping progress reused after changing the mapping criterion", got)
	}

	boxelPlan := checkedUpdate(t, s, owner, M{"action": "save-survey", "name": "Boxel", "criterion": "visit",
		"boxelSeed": "Thraikoo PS-U e2-5", "sequenceFrom": 0, "sequenceTo": 6})
	if v.Int(v.At(boxelPlan, "survey", "boxel", "widthLy")) != 160 || v.At(boxelPlan, "survey", "next", "system") != "Thraikoo PS-U e2-0" ||
		v.At(boxelPlan, "survey", "next", "candidate") != true {
		t.Fatal("invalid procedural candidate plan", boxelPlan)
	}
	file, err = s.ExportSurvey(owner, v.Str(v.At(boxelPlan, "survey", "id")))
	if err != nil {
		t.Fatal(err)
	}
	imported = checkedUpdate(t, s, "other", M{"action": "import-survey", "text": file["text"]})
	if v.Int(v.At(imported, "survey", "sequenceFrom")) != 0 || v.At(imported, "survey", "next", "candidate") != true {
		t.Fatal(imported)
	}
	after := New(s.ExportSettings()).Snapshot("other", M{"mode": "surveys"})
	if v.At(after, "survey", "name") != "Boxel" {
		t.Fatal("active survey selection was lost", after["survey"])
	}
	if _, err := s.Update(owner, M{"action": "save-survey", "name": "Wrong", "boxelSeed": "Sol", "sequenceFrom": 0, "sequenceTo": 6}); err == nil {
		t.Fatal("accepted a nonprocedural seed")
	}
}

func TestFindingsRulesAndRecordsDoNotNotifyTwice(t *testing.T) {
	s := New(nil)
	first := eventAt("Scan", 1, v.Merge(planet(15, "Reach", 1, "Earthlike body"), M{"SurfaceGravity": 29.4, "SurfaceTemperature": 280}))
	if notices := s.Apply(owner, first, nil); len(notices) != 1 || notices[0]["rule"] != "earthlike" {
		t.Fatal(notices)
	}
	checkedUpdate(t, s, owner, M{"action": "save-finding-profile", "id": "default", "rules": M{
		"gravity": M{"mode": "notify", "threshold": 2, "cooldown": 0},
	}})
	if notices := s.Apply(owner, first, nil); len(notices) != 0 {
		t.Fatal("rule update replayed an old notification", notices)
	}
	moon := v.Merge(planet(15, "Reach", 2, "Icy body"), M{"Parents": []M{{"Planet": 8}}, "SemiMajorAxis": 1300000, "Eccentricity": .1, "SurfaceTemperature": 60})
	s.Apply(owner, eventAt("Scan", 2, moon), nil)
	parent := v.Merge(planet(15, "Reach", 8, "Sudarsky class I gas giant"), M{"Rings": []M{{"InnerRad": 1000000, "OuterRad": 2000000, "RingClass": "eRingClass_Icy"}}})
	if notices := s.Apply(owner, eventAt("Scan", 3, parent), nil); len(notices) != 1 || notices[0]["rule"] != "inside-rings" || notices[0]["bodyId"] != "2" {
		t.Fatal("moon orbit was not matched to known parent rings", notices)
	}
	s = New(s.ExportSettings())
	if notices := s.Apply(owner, eventAt("Scan", 4, planet(16, "Further", 1, "Earthlike body")), M{"replay": true}); len(notices) != 0 {
		t.Fatal("replay emitted a finding", notices)
	}
	state := s.Snapshot(owner, M{"mode": "findings", "rule": "inside-rings"})
	found := v.Rows(v.At(state, "findings", "rows"))
	if len(found) != 1 || v.Int(v.At(found[0], "values", "ringOuterM")) != 2000000 {
		t.Fatal("missing finding evidence", found)
	}
	var cold M
	for _, r := range v.Rows(state["records"]) {
		if r["key"] == "cold" {
			cold = r
		}
	}
	if v.Int(cold["value"]) != 60 || cold["observedAt"] == nil {
		t.Fatal("personal record lost", cold)
	}
}

func TestExplorationHistoryIsPaged(t *testing.T) {
	s := New(nil)
	for i := 1; i <= 65; i++ {
		s.Apply(owner, eventAt("Location", i, journalFields(i, fmt.Sprintf("Test %d", i))), nil)
	}
	data := s.Snapshot(owner, M{"page": 1})
	rows := v.Rows(v.At(data, "systems", "rows"))
	if len(rows) != 30 || rows[0]["system"] != "Test 35" || v.Int(v.At(data, "systems", "total")) != 65 {
		t.Fatal("history was not paginated correctly", data["systems"])
	}
	if rows := v.Rows(v.At(s.Snapshot(owner, M{"page": 1000000}), "systems", "rows")); len(rows) != 5 {
		t.Fatal("invalid last page", rows)
	}
}

func TestSurveySkipVisitedDoesNotSkipNewlyArrivedCurrentTarget(t *testing.T) {
	s := New(nil)
	s.now = func() time.Time { return base.Add(2 * time.Minute) }
	s.Apply(owner, eventAt("Location", 1, journalFields(11, "Previously visited")), nil)
	plan := checkedUpdate(t, s, owner, M{"action": "save-survey", "name": "FSS", "criterion": "fss", "skipVisited": true,
		"targets": []M{{"system": "Previously visited", "address": "11"}, {"system": "New", "address": "12"}}})
	query := M{"mode": "surveys", "surveyId": v.At(plan, "survey", "id")}
	s.Apply(owner, eventAt("FSDJump", 3, journalFields(12, "New")), nil)
	got := s.Snapshot(owner, query)
	if v.At(got, "survey", "next", "system") != "New" || v.Int(v.At(got, "survey", "skipped")) != 1 {
		t.Fatal("arriving at a new target skipped its remaining FSS work", got["survey"])
	}
	s.Apply(owner, eventAt("FSSAllBodiesFound", 4, v.Merge(journalFields(12, "New"), M{"Count": 1})), nil)
	got = s.Snapshot(owner, query)
	if v.Int(v.At(got, "survey", "completed")) != 1 || v.At(got, "survey", "next") != nil {
		t.Fatal("confirmed FSS did not advance the survey", got["survey"])
	}
}
