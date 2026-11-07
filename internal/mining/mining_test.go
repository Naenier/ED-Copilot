package mining

import (
	"context"
	"encoding/json"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func mn(t *testing.T, got any, want float64) {
	t.Helper()
	if math.Abs(v.Num(got)-want) > 1e-8 {
		t.Fatalf("got %v want %g", got, want)
	}
}
func me(kind, at string, rest M) M { return v.Merge(M{"event": kind, "timestamp": at}, rest) }
func TestPreflightModulesCargoAndLoadoutInvalidation(t *testing.T) {
	modules := []M{}
	for _, item := range []string{"hpt_mininglaser_fixed_small", "int_refinery_size2_class2", "int_multidronecontrol_mining_size3_class2", "int_detailedsurfacescanner_tiny"} {
		modules = append(modules, ClassifyModule(M{"Item": item, "On": true, "Health": 1}))
	}
	loadout := M{"known": true, "modules": modules}
	preflight := Preflight(loadout, "laser", M{"vehicle": "ship", "cargoCapacity": 100, "cargoTons": 50}, 10)
	if preflight["status"] != "ready" {
		t.Fatal(preflight)
	}
	preflight = Preflight(loadout, "laser", M{"vehicle": "ship", "cargoCapacity": 100, "cargoTons": 100}, 10)
	if preflight["status"] != "incomplete" {
		t.Fatal("full cargo not flagged")
	}
	if CargoCapacity(M{"vehicle": "srv", "cargoCapacity": 100}, loadout) != nil {
		t.Fatal("SRV capacity borrowed ship capacity")
	}
	modules = append(modules, ClassifyModule(M{"Item": "hpt_mining_seismchrgwarhd_fixed_medium", "AmmoInClip": 0, "AmmoInHopper": 0}))
	modules = append(modules, ClassifyModule(M{"Item": "hpt_mining_abrblstr_fixed_small"}))
	if Preflight(M{"known": true, "modules": modules}, "core", M{"cargoCapacity": 100, "cargoTons": 0}, 5)["status"] != "incomplete" {
		t.Fatal("empty seismic ammunition not flagged")
	}
	s := New(nil)
	s.Apply(me("Loadout", "2026-11-07T12:00:00Z", M{"Modules": []M{{"Item": "int_refinery_size2_class2"}}, "CargoCapacity": 100, "ShipID": 1}), M{"commander": M{"ship": "Python"}})
	s.Apply(me("ModuleBuy", "2026-11-07T12:00:01Z", nil), nil)
	if v.Bool(s.Loadout["known"]) {
		t.Fatal("module change retained stale loadout")
	}
}
func TestMiningOperationClockCompositionAndBoundary(t *testing.T) {
	s := New(nil)
	ctx := M{"commander": M{"system": "Test", "body": "Test A Ring", "ship": "Python", "vehicle": "ship", "cargoCapacity": 100, "cargoTons": 10}, "settings": M{"method": "all", "minTier": "medium", "minLaserPercent": 15}, "file": "Journal.synthetic", "gameRunning": true, "now": "2026-11-07T12:02:00Z"}
	s.Apply(me("ProspectedAsteroid", "2026-11-07T12:00:00Z", M{"Remaining": 100, "Content": "$AsteroidMaterialContent_High;", "Materials": []M{{"Name": "Platinum", "Proportion": 30}}, "MotherlodeMaterial": "Monazite"}), ctx)
	s.Apply(me("LaunchDrone", "2026-11-07T12:00:05Z", M{"Type": "Prospector"}), ctx)
	s.Apply(me("MiningRefined", "2026-11-07T12:01:00Z", M{"Type": "Platinum", "Count": 10}), ctx)
	snap := s.Snapshot(ctx)
	mn(t, v.At(snap, "current", "tonsPerHour"), 300)
	mn(t, v.At(snap, "current", "tonsPerProspector"), 10)
	composition := v.Rows(v.At(snap, "current", "composition"))
	if len(composition) != 2 {
		t.Fatal(composition)
	}
	mn(t, v.At(snap, "stats", "total", "refinedTons"), 10)
	if v.At(snap, "prospect", "decision") != "mine" {
		t.Fatal(snap["prospect"])
	}
	ctx["now"] = "2026-11-07T22:00:00Z"
	snap = s.Snapshot(ctx)
	if !v.Bool(v.At(snap, "current", "clockCapped")) {
		t.Fatal("idle app clock not capped")
	}
	mn(t, v.At(snap, "current", "elapsedSeconds"), 360)
	s.Apply(me("Docked", "2026-11-07T12:03:00Z", nil), ctx)
	snap = s.Snapshot(ctx)
	if snap["current"] != nil || len(v.Rows(snap["history"])) != 1 {
		t.Fatal("operation did not close", snap)
	}
	s.Apply(me("MiningRefined", "2026-11-07T12:05:00Z", M{"Type": "Gold"}), v.Merge(v.Copy(ctx), M{"countInPeriod": false}))
	mn(t, v.At(s.Snapshot(ctx), "stats", "total", "refinedTons"), 10)
	if err := s.ResetRun("bad"); err == nil {
		t.Fatal("invalid reset date accepted")
	}
	if err := s.ResetRun("2026-11-07T12:10:00Z"); err != nil {
		t.Fatal(err)
	}
	restored := New(s.ExportSettings())
	if restored.ResetAt != "2026-11-07T12:10:00Z" {
		t.Fatal("manual period lost")
	}
}
func TestTargetsDistinguishRingMethodsAndPlanetaryUnknowns(t *testing.T) {
	world := []M{{"key": "1", "name": "Test", "address": "1", "coordinates": []float64{0, 0, 0}, "bodies": []M{{"id": 1, "name": "Test 1", "reserve": "PristineResources", "rings": []M{{"name": "Test 1 A Ring", "type": "eRingClass_MetalRich"}}}, {"id": 2, "name": "Test 2", "landable": true, "materials": []M{{"key": "polonium", "percent": .1}}}, {"id": 3, "name": "Test 3", "landable": true}}}}
	signals := map[string][]M{"1:Test 1 A Ring": {{"Type": "Platinum", "Count": 2}}, "1:Test 3": {{"Type": "$PlanetaryMiningLocation_Name;", "Count": 3}}}
	resources := trading.Resources()
	cmd := M{"coordinates": []float64{0, 0, 0}}
	targets := BuildTargets(world, signals, resources, cmd, M{"method": "laser"})
	if len(targets) != 0 {
		t.Fatal("metal rich platinum should not be laser target", targets)
	}
	targets = BuildTargets(world, signals, resources, cmd, M{"method": "core"})
	if len(targets) != 1 || targets[0]["confidence"] != "confirmed" {
		t.Fatal(targets)
	}
	targets = BuildTargets(world, signals, resources, cmd, M{"method": "surface-planet"})
	if len(targets) != 1 || targets[0]["tier"] != "unknown" || !v.Bool(targets[0]["compositionUnknown"]) {
		t.Fatal("planetary sites turned into valuable confirmed commodity", targets)
	}
	targets = BuildTargets(world, signals, resources, cmd, M{"method": "raw"})
	if len(targets) != 1 || targets[0]["tier"] != "high" {
		t.Fatal("rare raw materials missing", targets)
	}
}
func TestProspectContentDoesNotMakeWorthlessOreValuable(t *testing.T) {
	settings := M{"method": "laser", "minTier": "medium", "minLaserPercent": 15}
	prospect := M{"remaining": 100, "content": "High", "materials": []M{{"key": "bauxite", "percent": 99, "name": "Бокситы"}}}
	result := AssessProspect(prospect, settings, trading.Resources())
	if result["decision"] != "skip" {
		t.Fatal("raw Content used as commodity value", result)
	}
	prospect = M{"remaining": 0, "materials": []M{{"key": "platinum", "percent": 30}}}
	if AssessProspect(prospect, settings, trading.Resources())["decision"] != "skip" {
		t.Fatal("depleted asteroid accepted")
	}
	prospect = M{"motherlode": "monazite", "coreCracked": true, "materials": []M{}}
	if AssessProspect(prospect, M{"method": "core", "minTier": "medium"}, trading.Resources())["decision"] != "unknown" {
		t.Fatal("cracked core shown as intact")
	}
}
func TestPlanTradingAndMiningRates(t *testing.T) {
	context := M{"commander": M{"vehicle": "ship", "cargoCapacity": 100, "cargo": []M{{"key": "platinum", "count": 20}}}, "resources": []M{{"key": "platinum", "name": "Платина", "kind": "commodity", "price": 200, "priceAt": "2026-11-06T10:00:00Z"}}, "now": "2026-11-07T12:00:00Z", "history": []M{{"id": "run", "elapsedSeconds": 600, "yields": []M{{"key": "platinum", "tonsPerHour": 40}}}}}
	plan := EstimatePlan(M{"targetTons": 200, "purchasePrice": 50, "operatingCost": 10, "tripMinutes": 20}, context)
	mn(t, plan["remainingTons"], 180)
	mn(t, plan["miningMinutes"], 270)
	mn(t, plan["trips"], 2)
	mn(t, plan["profit"], 29980)
	if plan["rateSource"] != "journal" {
		t.Fatal(plan)
	}
	plan = EstimatePlan(M{"mode": "trade", "targetTons": 100, "sellPrice": 200, "purchasePrice": 100, "operatingCost": 50, "tripMinutes": 10}, context)
	mn(t, plan["miningMinutes"], 0)
	mn(t, plan["estimatedProfitPerHour"], 59700)
	plan = EstimatePlan(M{"resourceKey": "polonium", "sellPrice": 100}, context)
	if v.Bool(plan["applicable"]) || plan["sellPrice"] != nil {
		t.Fatal("raw materials given market revenue", plan)
	}
}
func TestRemoteDumpSignalsSupplementButDoNotReplaceLocalScan(t *testing.T) {
	s := New(nil)
	ctx := M{"commander": M{"system": "Test", "systemAddress": "9007199254740999", "coordinates": []float64{0, 0, 0}}}
	s.Apply(me("Scan", "2026-11-07T12:00:00Z", M{"BodyID": 1, "BodyName": "Test 1", "Landable": true, "SurfaceGravity": 9.80665}), ctx)
	remote := M{"name": "Test", "address": "9007199254740999", "coordinates": []float64{0, 0, 0}, "bodies": []M{{"id": 1, "name": "Test 1", "source": "spansh", "gravity": 99, "signals": []M{{"Type": "$PlanetaryMiningLocation_Name;", "Count": 2}}, "rings": []M{{"name": "Test 1 A Ring", "type": "Metallic", "signals": []M{{"key": "platinum", "count": 1}}}}}}}
	s.MergeRemoteSystem(remote)
	world := s.WorldSnapshot()
	body := v.Rows(world[0]["bodies"])[0]
	mn(t, body["gravity"], 1)
	targets := v.Rows(s.Snapshot(M{"commander": v.Obj(ctx["commander"]), "settings": M{"method": "surface-planet"}})["targets"])
	if len(targets) != 1 {
		t.Fatal("remote planetary signal not merged")
	}
	s.Apply(me("SAASignalsFound", "2026-11-07T12:01:00Z", M{"BodyName": "Test 1", "Signals": []M{{"Type": "$PlanetaryMiningLocation_Name;", "Count": 3}}}), ctx)
	s.MergeRemoteSystem(remote)
	targets = v.Rows(s.Snapshot(M{"commander": v.Obj(ctx["commander"]), "settings": M{"method": "surface-planet"}})["targets"])
	mn(t, targets[0]["miningSites"], 3)
}

type miningDoer func(*http.Request) (*http.Response, error)

func (f miningDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }
func miningResponse(payload any) *http.Response {
	b, _ := json.Marshal(payload)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}
func TestMiningSearchMockAllSourcesAndID64(t *testing.T) {
	calls := map[string]int{}
	dump := M{"system": M{"name": "Test", "id64": "9007199254740999", "coords": M{"x": 0, "y": 0, "z": 0}, "bodies": []M{{"bodyId": 1, "name": "Test 1", "isLandable": true, "signals": M{"$PlanetaryMiningLocation_Name;": 2}, "rings": []M{{"name": "Test 1 A Ring", "type": "Metallic", "signals": M{"Platinum": 1}}}}}}}
	search := NewSearcher(miningDoer(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.Path]++
		switch {
		case strings.Contains(r.URL.Path, "/dump/"):
			return miningResponse(dump), nil
		case strings.Contains(r.URL.Path, "/bodies/search"):
			return miningResponse(M{"results": []M{}}), nil
		case strings.Contains(r.URL.Path, "sphere-systems"):
			return miningResponse([]M{}), nil
		default:
			t.Fatalf("unexpected endpoint %s", r.URL)
			return nil, nil
		}
	}))
	updates := 0
	s := New(nil)
	search.OnSystem = func(system M) { updates++; s.MergeRemoteSystem(system) }
	out, err := search.Search(context.Background(), M{"system": "Test", "resourceKey": "platinum", "radiusLy": 50}, M{"world": []M{{"name": "Test", "address": "9007199254740999", "coordinates": []float64{0, 0, 0}}}})
	if err != nil || updates != 1 {
		t.Fatal(out, err, updates)
	}
	if len(v.Rows(out["systems"])) != 1 || calls["/api/dump/9007199254740999"] != 1 {
		t.Fatal("exact ID64 endpoint not used", calls)
	}
	targets := v.Rows(s.Snapshot(M{"commander": M{"coordinates": []float64{0, 0, 0}}, "settings": M{"method": "surface-planet"}})["targets"])
	if len(targets) != 1 {
		t.Fatal("search result missing planetary target", targets)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	search.Client = miningDoer(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })
	if _, err := search.Search(ctx, M{"system": "Test"}, M{}); err == nil {
		t.Fatal("cancelled search succeeded")
	}
}

func TestPlanetaryMiningRawMaterialsAndExternalSales(t *testing.T) {
	s := New(nil)
	cmd := M{"system": "Surface Test", "systemAddress": "123", "coordinates": []float64{1, 2, 3}, "body": "Surface Test 1", "vehicle": "srv", "srvCargoCapacity": 4, "cargoTons": 1}
	ctx := M{"commander": cmd, "settings": M{"method": "surface-planet"}, "file": "synthetic"}
	for _, e := range []M{me("Location", "2026-11-07T12:00:00Z", M{"StarSystem": "Surface Test", "SystemAddress": "123", "StarPos": []float64{1, 2, 3}}), me("Scan", "2026-11-07T12:00:01Z", M{"BodyID": 1, "BodyName": "Surface Test 1", "Landable": true, "DistanceFromArrivalLS": 400, "ReserveLevel": "PristineResources", "Materials": M{"Polonium": .1}, "Rings": []M{{"Name": "Surface Test 1 A Ring", "RingClass": "Icy"}}}), me("ApproachSettlement", "2026-11-07T12:00:02Z", M{"BodyName": "Surface Test 1", "NearestDestination": "$PlanetaryMiningLocation_Name;#index=2"}), me("MiningRefined", "2026-11-07T12:01:00Z", M{"Type": "coltan", "Count": 2}), me("MiningRefined", "2026-11-07T12:02:00Z", M{"Type": "coltan", "Count": 1}), me("MaterialCollected", "2026-11-07T12:02:01Z", M{"Name": "Polonium", "Category": "Raw", "Count": 3}), me("AsteroidCracked", "2026-11-07T12:02:02Z", nil), me("MarketSell", "2026-11-07T12:03:00Z", M{"Type": "coltan", "Count": 3, "TotalSale": 300})} {
		s.Apply(e, ctx)
	}
	snap := s.Snapshot(ctx)
	if v.At(snap, "surface", "site") != "Участок 2" {
		t.Fatal(snap["surface"])
	}
	mn(t, v.Rows(v.At(snap, "surface", "lastRefined"))[0]["count"], 3)
	mn(t, v.At(snap, "stats", "total", "materialUnits"), 3)
	mn(t, v.At(snap, "stats", "total", "asteroidsCracked"), 1)
	mn(t, v.At(snap, "stats", "total", "revenue"), 300)
	s.Apply(me("MarketSell", "2026-11-07T12:04:00Z", M{"Type": "coltan", "Count": 3, "TotalSale": 300}), v.Merge(v.Copy(ctx), M{"ownMarket": true}))
	mn(t, v.At(s.Snapshot(ctx), "stats", "total", "revenue"), 300)
	if s.Update(M{"method": "raw", "tonsPerHour": -1}) == nil {
		t.Fatal("negative mining speed accepted")
	}
	if s.Update(M{"method": "raw", "tonsPerHour": 100, "buyPrice": 5}) != nil {
		t.Fatal("valid options refused")
	}
	if s.Update(M{"action": "reset", "timestamp": "2026-11-07T13:00:00Z"}) != nil {
		t.Fatal("manual reset refused")
	}
	edsm := EDSMSystem(M{"bodies": []M{{"bodyId": 1, "name": "Fallback 1", "isLandable": true, "materials": M{"Yttrium": .1}, "rings": []M{{"name": "Fallback A Ring", "type": "Icy"}}}}}, M{"name": "Fallback", "id64": "999", "coords": M{"x": 1, "y": 2, "z": 3}})
	if edsm["address"] != "999" || len(v.Rows(edsm["bodies"])) != 1 {
		t.Fatal(edsm)
	}
	systems := SpanshSystems(M{"results": []M{{"system_name": "Fallback", "system_id64": "999", "system_x": 1, "system_y": 2, "system_z": 3, "body_id": 1, "name": "Fallback 1", "is_landable": true, "materials": []M{{"name": "Polonium", "share": .1}}, "rings": []M{{"name": "Fallback A Ring", "type": "Icy", "signals": []M{{"name": "Tritium", "count": 1}}}}}}})
	if len(systems) != 1 || len(v.Rows(systems[0]["bodies"])) != 1 {
		t.Fatal(systems)
	}
}
