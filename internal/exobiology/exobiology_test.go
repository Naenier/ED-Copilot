package exobiology

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

const fixtureAddress = "18446744073709551613"

func ev(kind string, values M) M {
	out := M{"event": kind, "timestamp": "2026-01-02T03:04:05Z", "SystemAddress": json.Number(fixtureAddress)}
	for k, x := range values {
		out[k] = x
	}
	return out
}
func fixture() (*Service, M) {
	s := New(nil)
	ctx := M{"commander": M{"system": "Synthetic Exo System", "systemAddress": json.Number(fixtureAddress), "body": "Synthetic Exo A 1", "bodyId": 1}}
	s.Apply(ev("LoadGame", M{"FID": "synthetic-commander"}), ctx)
	s.Apply(ev("Location", M{"BodyID": 1, "BodyName": "Synthetic Exo A 1", "BodyType": "Planet", "StarPos": []float64{0, 0, 0}}), ctx)
	s.Apply(ev("Scan", M{"BodyID": 1, "BodyName": "Synthetic Exo A 1", "PlanetClass": "High metal content body", "Landable": true, "AtmosphereType": "Ammonia", "Atmosphere": "thin ammonia", "SurfacePressure": 1700, "SurfaceTemperature": 175, "SurfaceGravity": 1, "Radius": 1000000, "Volcanism": "", "DistanceFromArrivalLS": 20}), ctx)
	return s, ctx
}
func organic(kind string, body int, stamp string) M {
	return ev("ScanOrganic", M{"ScanType": kind, "Body": body, "Species": "$Codex_Ent_Stratum_07_Name;", "Genus": "$Codex_Ent_Stratum_Genus_Name;", "Variant": "$Codex_Ent_Stratum_07_d_Name;", "Species_Localised": "Stratum Tectonicas", "Variant_Localised": "Stratum Tectonicas - Mauve", "timestamp": stamp})
}
func atStatus(s *Service, ctx M, lat, lon float64) {
	s.Apply(ev("Status", M{"BodyName": "Synthetic Exo A 1", "Latitude": lat, "Longitude": lon, "PlanetRadius": 1000000, "Heading": 90}), ctx)
}
func tracker(s *Service) M { return v.Obj(s.SnapshotForHUD()["tracker"]) }

func TestCatalogCompletenessAliasesAndIsolation(t *testing.T) {
	d := Catalog()
	if len(d.species) != 126 || len(d.genera) != 25 {
		t.Fatalf("catalog lost species/genera: %d/%d", len(d.species), len(d.genera))
	}
	for _, sp := range d.species {
		if sp.ID == "" || sp.Name == "" || sp.EnglishName == "" || len(sp.Habitat) == 0 {
			t.Fatalf("incomplete %s", sp.ID)
		}
		entry, ok := d.Entry("exo/" + sp.ID)
		if !ok || len(v.Rows(entry["sections"])) < 2 {
			t.Fatal("entry incomplete")
		}
		entry["title"] = "mutated"
		again, _ := d.Entry(sp.ID)
		if again["title"] == "mutated" {
			t.Fatal("entry aliases immutable data")
		}
	}
	raw := M{"Species": "$Codex_Ent_Aleoids_01_Name;"}
	if sp := d.ResolveOrganic(raw); sp == nil || !strings.EqualFold(sp.EnglishName, "Aleoida Arcus") {
		t.Fatalf("journal symbol unresolved")
	}
	for _, q := range []Query{{Limit: 1000}, {Query: "Stratum Tectonicas"}, {Query: "Тектоника"}, {Offset: 100000}, {Group: "aleoida"}} {
		got := d.List(q)
		if len(v.Rows(got["entries"])) > 48 || v.Int(got["offset"]) > v.Int(got["total"]) {
			t.Fatal("unbounded list")
		}
	}
	if genusID("$Codex_Ent_Bacterial_Genus_Name;") != "bacterium" {
		t.Fatal("localized genus-independent symbol failed")
	}
}

func TestSamplingLogTwoSamplesAnalyseHasThreeAnchors(t *testing.T) {
	s, ctx := fixture()
	for i, kind := range []string{"Log", "Sample", "Sample", "Analyse"} {
		atStatus(s, ctx, 0, float64(min(i, 2))*0.1)
		e := organic(kind, 1, fmt.Sprintf("2026-01-02T03:04:%02dZ", 10+i))
		s.Apply(e, ctx)
	}
	tr := tracker(s)
	if tr["phase"] != "complete" || v.Int(tr["sampleCount"]) != 3 || len(v.Rows(tr["anchors"])) != 3 {
		t.Fatalf("third Sample/Analyse double counted: %+v", tr)
	}
	if len(v.Rows(s.Snapshot()["pending"])) != 1 {
		t.Fatal("completed specimen not pending")
	}
	if v.Num(tr["nearestSampleDistanceM"]) > 0.01 {
		t.Fatal("last sample not at third plant")
	}
	s.Apply(organic("Analyse", 1, "2026-01-02T03:04:13Z"), ctx)
	if len(v.Rows(s.Snapshot()["pending"])) != 1 {
		t.Fatal("duplicate analysis duplicated reward")
	}
}

func TestDistanceUsesNearestAnchorAndUnknownPosition(t *testing.T) {
	s, ctx := fixture()
	atStatus(s, ctx, 0, 179.99)
	s.Apply(organic("Log", 1, "2026-01-02T03:04:10Z"), ctx)
	atStatus(s, ctx, 0, -179.99)
	tr := tracker(s)
	d := v.Num(tr["nearestSampleDistanceM"])
	if math.Abs(d-349.06585) > 0.01 || tr["nearestSampleMeetsMin"] != false {
		t.Fatalf("seam/radius calculation wrong: %+v", tr)
	}
	atStatus(s, ctx, 0, -179.9)
	if tracker(s)["nearestSampleMeetsMin"] != true {
		t.Fatal("diversity distance never becomes ready")
	}
	s.Apply(ev("Status", M{"BodyName": "Synthetic Different Body", "Latitude": 0, "Longitude": 0, "PlanetRadius": 1e6}), ctx)
	tr = tracker(s)
	if tr["positionKnown"] != false || tr["nearestSampleMeetsMin"] != nil {
		t.Fatal("unrelated coordinates reused")
	}
}

func TestReplayRecoversOnlySavedMatchingAnchor(t *testing.T) {
	s, ctx := fixture()
	atStatus(s, ctx, 4, 5)
	first := organic("Log", 1, "2026-01-02T03:04:10Z")
	second := organic("Sample", 1, "2026-01-02T03:04:11Z")
	s.Apply(first, ctx)
	saved := s.ExportSettings()
	r := New(saved)
	_, base := fixture()
	replay := v.Merge(v.Copy(base), M{"replay": true})
	r.Apply(ev("LoadGame", M{"FID": "synthetic-commander"}), replay)
	r.Apply(ev("Scan", M{"BodyID": 1, "BodyName": "Synthetic Exo A 1", "Radius": 1e6}), replay)
	atStatus(r, base, 50, 50)
	r.Apply(first, replay)
	r.Apply(second, replay)
	tr := tracker(r)
	anchors := v.Rows(tr["anchors"])
	if len(anchors) != 1 || v.Num(anchors[0]["latitude"]) != 4 || v.Int(tr["recoveredSamples"]) != 1 {
		t.Fatalf("replay assigned current fix to old sample: %+v", tr)
	}
	r.Apply(organic("Log", 2, "2026-01-02T03:04:12Z"), replay)
	if len(v.Rows(tracker(r)["anchors"])) != 0 {
		t.Fatal("anchor crossed body")
	}
	r.Apply(ev("LoadGame", M{"FID": "different-synthetic-commander"}), replay)
	r.Apply(first, replay)
	if len(v.Rows(tracker(r)["anchors"])) != 0 {
		t.Fatal("anchor crossed commander")
	}
}

func TestBodyIDZeroAndLargeSystemAddressPreserved(t *testing.T) {
	s := New(nil)
	e := organic("Analyse", 0, "2026-01-02T03:04:10Z")
	s.Apply(e, M{})
	got := s.Snapshot()
	if v.Str(got["currentBodyKey"]) != fixtureAddress+":0" || len(v.Rows(got["pending"])) != 1 {
		t.Fatalf("body0/id64 lost: %+v", got["currentBodyKey"])
	}
	if v.Str(v.Rows(got["pending"])[0]["systemAddress"]) != fixtureAddress {
		t.Fatal("id64 rounded")
	}
}

func TestFootfallTriStateAndReceiptAreIndependent(t *testing.T) {
	for _, state := range []any{nil, false, true} {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			s, ctx := fixture()
			if state != nil {
				s.Apply(ev("Scan", M{"BodyID": 1, "WasFootfalled": state, "timestamp": "2026-01-02T03:04:06Z"}), ctx)
			}
			before := v.Obj(s.Snapshot()["selectedBody"])
			expected := "unknown"
			if state == false {
				expected = "unwalked"
			}
			if state == true {
				expected = "walked"
			}
			if before["firstFootfall"] != expected {
				t.Fatal("absence treated as false")
			}
			s.Apply(ev("Disembark", M{"BodyID": 1, "OnPlanet": true, "OnStation": false}), ctx)
			s.Apply(organic("Analyse", 1, "2026-01-02T03:04:12Z"), ctx)
			pending := v.Rows(s.Snapshot()["pending"])[0]
			if pending["firstFootfall"] != (state == false) {
				t.Fatal("unproven first footfall bonus")
			}
			base := v.Num(pending["baseCredits"])
			estimate := v.Num(v.Obj(s.Snapshot()["summary"])["pendingEstimatedCredits"])
			factor := 1.0
			if state == false {
				factor = 5
			}
			if estimate != base*factor {
				t.Fatal("wrong first discovery multiplier")
			}
		})
	}
}

func TestSalesFIFOActualBonusAndDeath(t *testing.T) {
	s, ctx := fixture()
	s.Apply(organic("Analyse", 1, "2026-01-02T03:04:10Z"), ctx)
	s.Apply(organic("Analyse", 2, "2026-01-02T03:04:11Z"), ctx)
	bio := v.Copy(organic("Analyse", 9, ""))
	bio["Value"], bio["Bonus"] = 100, 400
	s.Apply(ev("SellOrganicData", M{"MarketID": 99, "BioData": []M{bio}}), ctx)
	snap := s.Snapshot()
	sale := v.Rows(snap["sales"])[0]
	if v.Num(sale["credits"]) != 500 || v.Int(sale["bodyId"]) != 1 || len(v.Rows(snap["pending"])) != 1 {
		t.Fatalf("sale did not use actual receipts and FIFO origin: %+v", sale)
	}
	s.Apply(ev("Died", M{}), ctx)
	snap = s.Snapshot()
	if len(v.Rows(snap["pending"])) != 0 || snap["tracker"] != nil || v.Num(v.Obj(snap["summary"])["soldCredits"]) != 500 {
		t.Fatal("death erased sold income or kept unsold data")
	}
	s.Apply(organic("Log", 2, "2026-01-02T03:05:01Z"), ctx)
	if v.Int(tracker(s)["sampleCount"]) != 1 {
		t.Fatal("lost canister resurrected")
	}
}

func TestSaleUnknownBonusNotFreeAndHistoryTotalsRemainExact(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("SellOrganicData", M{"BioData": []M{{"Species_Localised": "Stratum Tectonicas", "Value": 100}}}), ctx)
	if v.Rows(s.Snapshot()["sales"])[0]["credits"] != nil {
		t.Fatal("missing bonus guessed zero")
	}
	for i := 0; i < 520; i++ {
		s.Apply(ev("SellOrganicData", M{"timestamp": fmt.Sprintf("2026-01-02T04:%02d:%02dZ", i/60, i%60), "BioData": []M{{"Value": 10, "Bonus": 0}}}), ctx)
	}
	snap := s.Snapshot()
	summary := v.Obj(snap["summary"])
	if v.Int(summary["salesCount"]) != 521 || v.Num(summary["soldCredits"]) != 5200 || len(v.Rows(snap["sales"])) != 200 {
		t.Fatalf("bounded history corrupted totals: %+v", summary)
	}
}

func TestGenusHintAndOneSpeciesPerGenusValueRange(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("SAASignalsFound", M{"BodyID": 1, "Signals": []M{{"Type": "$SAA_SignalType_Biological;", "Count": 1}}, "Genuses": []M{{"Genus": "$Codex_Ent_Stratum_Genus_Name;"}}}), ctx)
	snap := s.Snapshot()
	b := v.Obj(snap["selectedBody"])
	for _, p := range v.Rows(b["predictions"]) {
		if p["group"] != "stratum" {
			t.Fatalf("DSS genus filter ignored: %s", p["group"])
		}
	}
	vr := v.Obj(b["valueRange"])
	if v.Num(vr["maximum"]) > 19010800 {
		t.Fatal("summed all species instead of one genus slot")
	}
	if v.Num(vr["minimum"]) <= 0 {
		t.Fatal("known range missing")
	}
}

func TestPredictionUnknownFieldsAreExplicitAndConfirmedRescued(t *testing.T) {
	s := New(nil)
	s.Apply(ev("ScanOrganic", M{"Body": 1, "ScanType": "Log", "Species_Localised": "Stratum Tectonicas"}), M{"replay": true})
	b := v.Obj(s.Snapshot()["selectedBody"])
	found := false
	for _, p := range v.Rows(b["predictions"]) {
		if p["speciesId"] == "stratum_stratum_tectonicas" {
			found = true
			if p["tier"] != "confirmed" || p["guaranteed"] != false || p["probabilityPercent"] != nil {
				t.Fatal("unproven guaranteed prediction")
			}
		}
	}
	if !found {
		t.Fatal("observed species hidden by unknown conditions")
	}
	if v.Int(v.Obj(s.Snapshot()["predictionQuality"])["sealedBodies"]) != 0 {
		t.Fatal("historic scan fabricated forecast")
	}
}

func TestPressureAlwaysPascalsAndAlternativeConditions(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("Scan", M{"BodyID": 1, "AtmosphereType": "CarbonDioxide", "SurfacePressure": 17.875, "SurfaceTemperature": 185, "Volcanism": "", "timestamp": "2026-01-02T03:04:06Z"}), ctx)
	b := s.bodies[fixtureAddress+":1"]
	pred := s.predict(b)
	renibus := findSpecies(pred, "concha_concha_renibus")
	if renibus == nil || renibus["tier"] == "unlikely" {
		t.Fatalf("185K OR condition or Pa conversion failed: %+v", renibus)
	}
	s.Apply(ev("Scan", M{"BodyID": 1, "SurfaceTemperature": 170, "timestamp": "2026-01-02T03:04:07Z"}), ctx)
	renibus = findSpecies(s.predict(b), "concha_concha_renibus")
	if renibus["tier"] != "unlikely" {
		t.Fatal("failed OR constraint passed")
	}
	s.Apply(ev("Scan", M{"BodyID": 1, "AtmosphereType": "Water", "SurfaceTemperature": 250, "timestamp": "2026-01-02T03:04:08Z"}), ctx)
	if findSpecies(s.predict(b), "concha_concha_renibus")["tier"] == "unlikely" {
		t.Fatal("CO2-only upper temperature applied to water")
	}
}
func findSpecies(rows []M, id string) M {
	for _, r := range rows {
		if r["speciesId"] == id {
			return r
		}
	}
	return nil
}

func TestGasBandAndZeroSignalsAreNotUnknown(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("Scan", M{"BodyID": 1, "AtmosphereType": "Neon", "AtmosphereComposition": []M{{"Name": "Neon", "Percent": 40}}, "PlanetClass": "Icy body", "SurfaceTemperature": 55, "timestamp": "2026-01-02T03:04:06Z"}), ctx)
	b := s.bodies[fixtureAddress+":1"]
	if findSpecies(s.predict(b), "bacterium_bacterium_acies")["tier"] != "unlikely" {
		t.Fatal("gas fraction ignored")
	}
	s.Apply(ev("FSSBodySignals", M{"BodyID": 1, "Signals": []M{{"Type": "Biological", "Count": 0}}}), ctx)
	for _, p := range s.predict(b) {
		if p["speciesId"] != "ingensradices_ingensradices_unicus" {
			t.Fatal("zero signals treated as unknown")
		}
	}
}

func TestLiveForecastAuditIsSealedBeforeDiscovery(t *testing.T) {
	s, ctx := fixture()
	s.Apply(organic("Log", 1, "2026-01-02T03:04:10Z"), ctx)
	quality := v.Obj(s.Snapshot()["predictionQuality"])
	if v.Int(quality["sealedBodies"]) != 1 || v.Int(quality["observations"]) != 1 {
		t.Fatal("forecast not scored")
	}
	s.Apply(organic("Analyse", 1, "2026-01-02T03:04:11Z"), ctx)
	if v.Int(v.Obj(s.Snapshot()["predictionQuality"])["observations"]) != 1 {
		t.Fatal("same species scored repeatedly")
	}
}

func TestSettingsValidationTransactionalAndSnapshotNoAliases(t *testing.T) {
	s, ctx := fixture()
	before := s.ExportSettings()
	if err := s.Update(M{"action": "configure", "includeBacterium": false, "radiusLy": -1}); err == nil {
		t.Fatal("invalid setting accepted")
	}
	if !reflect.DeepEqual(before["settings"], s.ExportSettings()["settings"]) {
		t.Fatal("partial settings applied")
	}
	if err := s.Update(M{"action": "bookmark", "bodyKey": fixtureAddress + ":1"}); err != nil {
		t.Fatal(err)
	}
	s.Update(M{"action": "add-waypoint", "bodyKey": fixtureAddress + ":1", "latitude": 4, "longitude": 5, "label": "Synthetic waypoint"})
	got := s.Snapshot()
	v.Obj(got["settings"])["radiusLy"] = -999
	v.Obj(v.Obj(got["selectedBody"])["scan"])["Radius"] = 0
	v.Rows(got["bookmarks"])[0]["name"] = "changed"
	again := s.Snapshot()
	if v.Num(v.Obj(again["settings"])["radiusLy"]) < 0 || v.Num(v.Obj(v.Obj(again["selectedBody"])["scan"])["Radius"]) == 0 || v.Rows(again["bookmarks"])[0]["name"] == "changed" {
		t.Fatal("DTO mutates domain")
	}
	atStatus(s, ctx, 4, 5)
	if len(v.Rows(s.Snapshot()["surfaceMarks"])) != 1 {
		t.Fatal("waypoint missing")
	}
}

func TestSitesBoundedAndDistancesHonest(t *testing.T) {
	q := SiteQuery{Query: "", Limit: 10000, Coordinates: []float64{0, 0, 0}, RadiusLy: 100000}
	result := Catalog().Sites(q)
	rows := v.Rows(result["entries"])
	if len(rows) == 0 || len(rows) > 48 || result["live"] != false {
		t.Fatal("site index missing/unbounded")
	}
	for _, r := range rows {
		if r["distanceLy"] != nil || r["coordinatesKnown"] != false || r["distanceLowerBoundLy"] == nil {
			t.Fatal("approximate coordinate misrepresented")
		}
		r["system"] = "changed"
	}
	again := Catalog().Sites(q)
	if v.Rows(again["entries"])[0]["system"] == "changed" {
		t.Fatal("site cache mutated")
	}
}

func TestConcurrentSnapshotsAndJournal(t *testing.T) {
	s, ctx := fixture()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				snap := s.SnapshotForHUD()
				raw, _ := json.Marshal(snap)
				if strings.Contains(string(raw), "data:image/") {
					t.Error("HUD contains media bytes")
				}
			}
		}()
	}
	for i := 0; i < 30; i++ {
		s.Apply(ev("FSSBodySignals", M{"BodyID": i + 1, "Signals": []M{{"Type": "Biological", "Count": 2}}, "timestamp": fmt.Sprintf("2026-01-02T05:00:%02dZ", i)}), ctx)
	}
	wg.Wait()
}

func TestUnselectedTargetsHonorProfitAndGenusFilters(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("FSSBodySignals", M{"BodyID": 1, "Signals": []M{{"Type": "Biological", "Count": 1}}}), ctx)
	s.Apply(ev("Scan", M{"BodyID": 2, "BodyName": "Synthetic Exo A 2", "PlanetClass": "Rocky body", "Landable": true, "AtmosphereType": "CarbonDioxide", "SurfacePressure": 3000, "SurfaceTemperature": 177, "SurfaceGravity": 1, "Radius": 900000, "Volcanism": ""}), ctx)
	s.Apply(ev("SAASignalsFound", M{"BodyID": 2, "Signals": []M{{"Type": "Biological", "Count": 1}}, "Genuses": []M{{"Genus": "$Codex_Ent_Stratum_Genus_Name;"}}}), ctx)
	if err := s.Update(M{"action": "select-body", "bodyKey": fixtureAddress + ":2"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(M{"action": "configure", "selectedGenus": "stratum", "minValue": 15000000}); err != nil {
		t.Fatal(err)
	}
	rows := v.Rows(s.Snapshot()["targets"])
	if len(rows) != 1 || rows[0]["key"] != fixtureAddress+":1" {
		t.Fatalf("unselected profitable target lost / low-value target retained: %v", func() []any {
			x := []any{}
			for _, r := range rows {
				x = append(x, r["key"])
			}
			return x
		}())
	}
	before := s.bodies[fixtureAddress+":1"].PredictionRevision
	s.SnapshotForHUD()
	if before != s.bodies[fixtureAddress+":1"].PredictionRevision {
		t.Fatal("idle HUD rebuilt target prediction")
	}
	s.Update(M{"action": "configure", "minValue": 0})
	if len(v.Rows(s.Snapshot()["targets"])) != 2 {
		t.Fatal("setting change retained stale target filters")
	}
}

func TestParentStarObservationInvalidatesColdBodyPrediction(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("Scan", M{"BodyID": 1, "PlanetClass": "Rocky body", "AtmosphereType": "None", "Atmosphere": "", "SurfacePressure": 0, "SurfaceTemperature": 400, "Parents": []M{{"Star": 0}}, "timestamp": "2026-01-02T03:04:06Z"}), ctx)
	b := s.bodies[fixtureAddress+":1"]
	before := findSpecies(s.predict(b), "anemone_luteolum")
	if before["tier"] == "likely" {
		t.Fatal("unknown parent star silently matched")
	}
	old := b.PredictionRevision
	s.Apply(ev("Scan", M{"BodyID": 0, "BodyName": "Synthetic Exo A", "StarType": "B", "Luminosity": "IV", "timestamp": "2026-01-02T03:04:07Z"}), ctx)
	after := findSpecies(s.predict(b), "anemone_luteolum")
	if b.PredictionRevision == old || after["tier"] == "unlikely" {
		t.Fatalf("new parent star not reflected: %v", after["reasons"])
	}
}

func BenchmarkExobiologyHUDCached(b *testing.B) {
	s, _ := fixture()
	s.SnapshotForHUD()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.SnapshotForHUD()
	}
}

func TestHistogramRankingSharesAndGenusPriorAreDifferentFromProbability(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("FSSBodySignals", M{"BodyID": 1, "Signals": []M{{"Type": "Biological", "Count": 1}}}), ctx)
	rows := s.predict(s.bodies[fixtureAddress+":1"])
	sums := map[string]float64{}
	hasPrior := false
	for _, r := range rows {
		if n, ok := v.Number(r["relativeSpeciesSharePercent"]); ok {
			sums[v.Str(r["group"])] += n
		}
		if n, ok := v.Number(r["genusObservedRatePercent"]); ok {
			hasPrior = true
			if n < 0 || n > 100 {
				t.Fatal("invalid observed genus rate")
			}
		}
		if r["probabilityPercent"] != nil || r["guaranteed"] != false {
			t.Fatal("uncalibrated model presented as probability")
		}
	}
	for g, sum := range sums {
		if math.Abs(sum-100) > 1e-6 {
			t.Fatalf("relative shares for %s sum to %g", g, sum)
		}
	}
	if !hasPrior {
		t.Fatal("original genus backoff prior not applied")
	}
}

func TestScannerTriangulationAndAmbiguousFallback(t *testing.T) {
	radius := 1000000.0
	origin := M{"latitude": 0.0, "longitude": 0.0}
	east := fromOffset(origin, localPoint{100, 0}, radius)
	a := v.Merge(v.Copy(origin), M{"heading": 45.0})
	b := v.Merge(east, M{"heading": 315.0})
	located := LocatePlant([]M{a, b}, radius)
	want := fromOffset(origin, localPoint{50, 50}, radius)
	if located["locationBasis"] != "crossing" || GreatCircleDistance(v.Num(located["latitude"]), v.Num(located["longitude"]), v.Num(want["latitude"]), v.Num(want["longitude"]), radius) > 0.01 {
		t.Fatalf("crossing wrong: %+v", located)
	}
	b["heading"] = 45.0
	if LocatePlant([]M{a, b}, radius)["locationBasis"] != "scanned-from" {
		t.Fatal("parallel lines fabricated plant")
	}
	single := v.Merge(origin, M{"heading": 90, "altitudeAboveGroundM": 100, "pitch": -45})
	ahead := LocatePlant([]M{single}, radius)
	if ahead["locationBasis"] != "ahead" || math.Abs(GreatCircleDistance(0, 0, v.Num(ahead["latitude"]), v.Num(ahead["longitude"]), radius)-100) > 0.01 {
		t.Fatal("known height/pitch projection failed")
	}
}

func TestStaleStatusNeverAnchorsNewScanAndAltitudeDatumIsExplicit(t *testing.T) {
	s, ctx := fixture()
	atStatus(s, ctx, 4, 5)
	s.Apply(organic("Log", 1, "2026-01-02T04:04:10Z"), ctx)
	if len(v.Rows(tracker(s)["anchors"])) != 0 {
		t.Fatal("stale Status anchored a delayed sample")
	}
	b := s.bodies[fixtureAddress+":1"]
	ground := fixFrom(M{"Latitude": 0, "Longitude": 0, "Altitude": 100}, b, "")
	average := fixFrom(M{"Latitude": 0, "Longitude": 0, "Altitude": 100, "Flags": 1 << 29}, b, "")
	if ground["altitudeAboveGroundM"] != float64(100) || average["altitudeAboveGroundM"] != nil || average["elevationM"] != float64(100) {
		t.Fatal("altitude datum mixed")
	}
}

func TestDiversityAlertOnlyOnceUntilNextSample(t *testing.T) {
	s, ctx := fixture()
	atStatus(s, ctx, 0, 0)
	s.Apply(organic("Log", 1, "2026-01-02T03:04:10Z"), ctx)
	for i := 0; i < 5; i++ {
		atStatus(s, ctx, 0, 0.1)
	}
	count := 0
	for _, alert := range s.alerts {
		if alert["kind"] == "sample-distance-ready" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("distance cue repeated %d times", count)
	}
}

func TestSchemaRoundTripMarksBoundedAndPrivateRaysStayPrivate(t *testing.T) {
	s, ctx := fixture()
	atStatus(s, ctx, 0, 0)
	s.Apply(organic("Log", 1, "2026-01-02T03:04:10Z"), ctx)
	s.addCodexMark(M{"id": "synthetic-codex", "bodyKey": fixtureAddress + ":1", "speciesId": "stratum_stratum_tectonicas", "latitude": 0.1, "longitude": 0.1, "planetRadiusM": 1e6, "heading": 45, "at": "2026-01-02T03:04:10Z", "owner": "synthetic-commander", "kind": "codex", "label": "Synthetic composition scan"})
	saved := s.ExportSettings()
	restored := New(v.Obj(v.Clone(saved)))
	if v.Int(restored.ExportSettings()["schemaVersion"]) != SchemaVersion || len(restored.marks) != 2 {
		t.Fatal("schema round trip lost durable marks")
	}
	restored.Apply(ev("LoadGame", M{"FID": "synthetic-commander"}), ctx)
	restored.Apply(ev("Scan", M{"BodyID": 1, "BodyName": "Synthetic Exo A 1", "Radius": 1e6}), ctx)
	restored.Apply(organic("Log", 1, "2026-01-02T03:04:10Z"), v.Merge(v.Copy(ctx), M{"replay": true}))
	for _, row := range v.Rows(restored.Snapshot()["surfaceMarks"]) {
		if row["owner"] != nil || row["scans"] != nil {
			t.Fatal("private scanner history or commander identifier reached renderer")
		}
	}
	atStatus(restored, ctx, 0, 0)
	restored.fixSeen = time.Now().Add(-time.Minute)
	if tracker(restored)["positionKnown"] != false {
		t.Fatal("expired live position remained usable")
	}
	tooMany := []M{}
	for i := 0; i < maxSavedMarks+20; i++ {
		tooMany = append(tooMany, M{"id": fmt.Sprint(i), "bodyKey": "synthetic:1", "latitude": 0, "longitude": 0})
	}
	if len(New(M{"surfaceMarks": tooMany}).marks) != maxSavedMarks {
		t.Fatal("durable marks unbounded")
	}
}

func TestAllRenderedCatalogProseHasEnglishTranslation(t *testing.T) {
	hasCyrillic := func(text string) bool {
		for _, r := range text {
			if unicode.In(r, unicode.Cyrillic) {
				return true
			}
		}
		return false
	}
	var inspect func(any)
	inspect = func(raw any) {
		if text, ok := raw.(string); ok && hasCyrillic(text) {
			if translated := localization.Text("en", text); hasCyrillic(translated) {
				t.Errorf("untranslated domain prose: %s", text)
			}
		}
		for _, item := range v.Arr(raw) {
			inspect(item)
		}
		for _, item := range v.Obj(raw) {
			inspect(item)
		}
	}
	for _, sp := range Catalog().species {
		entry, _ := Catalog().Entry(sp.ID)
		for _, key := range []string{"title", "description", "properties", "sections", "related"} {
			inspect(entry[key])
		}
	}
}

func TestObservedStarGatesPreserveCompanionsAndWhiteDwarfs(t *testing.T) {
	s, ctx := fixture()
	b := s.bodies[fixtureAddress+":1"]
	b.Scan["Parents"] = []M{{"Star": 2}}
	s.Apply(ev("Scan", M{"BodyID": 0, "StarType": "M", "Luminosity": "V"}), ctx)
	a := assessment{}
	s.checkObservedStarGate(&a, b, gateForSpecies("stratum_stratum_tectonicas"))
	if len(a.unknown) == 0 || len(a.reasons) > 0 {
		t.Fatal("unscanned explicit host replaced with main star")
	}
	s.Apply(ev("Scan", M{"BodyID": 2, "StarType": "DA", "Luminosity": "VII", "timestamp": "2026-01-02T03:04:06Z"}), ctx)
	a = assessment{}
	s.checkObservedStarGate(&a, b, gateForSpecies("electricae_electricae_pluma"))
	if a.passed != 1 || len(a.reasons) > 0 {
		t.Fatal("observed white-dwarf Pluma habitat lost to older prose")
	}
	s.Apply(ev("Scan", M{"BodyID": 3, "StarType": "B", "Luminosity": "IVab", "timestamp": "2026-01-02T03:04:07Z"}), ctx)
	a = assessment{}
	s.checkObservedStarGate(&a, b, gateForSpecies("anemone_luteolum"))
	if a.passed != 1 {
		t.Fatal("B companion ignored / luminosity suffix not normalised")
	}
	for raw, want := range map[string]string{"DA": "D", "TTS7": "TTS", "WN": "W", "Black Hole": "H", "AeBe": "A"} {
		if starClass(raw) != want {
			t.Fatalf("star class %s", raw)
		}
	}
}

func TestConfirmedGeneraRemainMandatoryInValueRange(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("FSSBodySignals", M{"BodyID": 1, "Signals": []M{{"Type": "Biological", "Count": 1}}}), ctx)
	s.Apply(ev("ScanOrganic", M{"Body": 1, "ScanType": "Log", "Species_Localised": "Bacterium Aurasus"}), ctx)
	b := s.bodies[fixtureAddress+":1"]
	rows := s.predict(b)
	for _, row := range rows {
		if row["group"] != "bacterium" {
			t.Fatal("filled signal slot still advertised another genus")
		}
	}
	rangeDTO := s.bodyValueRange(b, rows)
	sp := Catalog().ResolveOrganic(M{"name": "Bacterium Aurasus"})
	if v.Num(rangeDTO["minimum"]) != v.Num(sp.BaseCredits) || v.Num(rangeDTO["maximum"]) != v.Num(sp.BaseCredits) {
		t.Fatal("known genus replaced with more valuable hypothetical genus")
	}
}

func TestSystemScanAndRadiusSearchContinueWhileFlying(t *testing.T) {
	s, ctx := fixture()
	s.Apply(ev("FSSDiscoveryScan", M{"BodyCount": 8}), ctx)
	s.Apply(ev("FSSBodySignals", M{"BodyID": 1, "Signals": []M{{"Type": "Biological", "Count": 1}}}), ctx)
	s.Apply(ev("FSSAllBodiesFound", M{"Count": 8}), ctx)
	system := v.Obj(s.SnapshotForHUD()["system"])
	if v.Int(system["bodiesTotal"]) != 8 || system["fssComplete"] != true || v.Int(system["scannedBodies"]) != 1 || v.Int(system["biologicalBodies"]) != 1 {
		t.Fatal("system scanning overview lost")
	}
	s.Update(M{"action": "configure", "radiusLy": 50})
	s.Apply(ev("FSDJump", M{"StarSystem": "Synthetic Distant System", "SystemAddress": "synthetic-distant", "StarPos": []float64{1000, 0, 0}}), M{})
	if s.SnapshotForHUD()["currentBody"] != nil && len(v.Obj(s.SnapshotForHUD()["currentBody"])) != 0 {
		t.Fatal("old surface body followed jump")
	}
	if len(v.Rows(s.Snapshot()["targets"])) != 0 {
		t.Fatal("radius filter disabled because commander is flying")
	}
}
