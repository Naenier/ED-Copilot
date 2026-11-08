package mining

import (
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudFixture() (*Service, M) {
	resources := []M{
		{"key": "rawa", "name": "Synthetic raw A", "kind": "raw", "grade": 4, "tier": "high"},
		{"key": "rawb", "name": "Synthetic raw B", "kind": "raw", "grade": 3, "tier": "medium"},
		{"key": "rawc", "name": "Synthetic raw C", "kind": "raw", "grade": 4, "tier": "high"},
		{"key": "ore", "name": "Synthetic ore", "kind": "commodity", "tier": "medium"},
	}
	return New(nil), M{"commander": M{"system": "Origin", "coordinates": []float64{0, 0, 0}}, "settings": M{"method": "laser", "minTier": "medium"}, "resources": resources}
}

func hudPlanet(s *Service, name string, coordinates any, raw string, landable, site bool) {
	materials := []M{}
	if raw != "" {
		materials = append(materials, M{"key": raw, "percent": 2})
	}
	body := M{"id": 1, "name": name + " 1", "landable": landable, "materials": materials, "rings": []M{}, "timestamp": "2026-11-08T12:00:00Z", "surfaceFinds": []M{}}
	if site {
		s.Signals[name+":"+name+" 1"] = []M{{"Type": "$PlanetaryMiningLocation_Name;", "Count": 1}}
		body["surfaceFinds"] = []M{{"site": "Synthetic site", "resources": []M{{"key": "ore", "name": "Synthetic ore", "count": 1}}}}
	}
	s.World[name] = M{"key": name, "name": name, "coordinates": coordinates, "bodies": M{"1": body}}
}

func hudNames(rows []M) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, v.Str(row["system"]))
	}
	return names
}

func TestHUDTargetsRejectDistantHighScoresBeforeTheTwoSiteLimit(t *testing.T) {
	s, ctx := hudFixture()
	for index, name := range []string{"Far A", "Far B", "Far C"} {
		hudPlanet(s, name, []float64{float64(200 + index*100), 0, 0}, "rawa", true, true)
	}
	hudPlanet(s, "Near A", []float64{25, 0, 0}, "rawb", true, true)
	hudPlanet(s, "Near B", []float64{35, 0, 0}, "rawb", true, true)
	for _, raw := range []bool{false, true} {
		got := hudNames(s.HUDTargets(ctx, raw))
		if !reflect.DeepEqual(got, []string{"Near A", "Near B"}) {
			t.Fatal("far high-score sites consumed the HUD limit", raw, got)
		}
	}
	if v.At(ctx, "settings", "method") != "laser" {
		t.Fatal("HUD activity changed the saved search method")
	}
	if rows := BuildTargets(s.WorldSnapshot(), s.Signals, v.Rows(ctx["resources"]), v.Obj(ctx["commander"]), v.Obj(ctx["settings"])); len(rows) != 0 {
		t.Fatal("general laser-search behaviour was changed", rows)
	}
}

func TestHUDTargetsUseBoundedRadiusAndMediumOrHighValue(t *testing.T) {
	s, ctx := hudFixture()
	hudPlanet(s, "At 75", []float64{75, 0, 0}, "rawa", true, true)
	hudPlanet(s, "At 120", []float64{120, 0, 0}, "rawa", true, true)
	if len(s.HUDTargets(ctx, true)) != 0 {
		t.Fatal("default HUD radius was not 50 light years")
	}
	v.Obj(ctx["settings"])["radiusLy"] = 500
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"At 75"}) {
		t.Fatal("configured HUD radius exceeded its 100 light year cap", got)
	}
	s, ctx = hudFixture()
	hudPlanet(s, "Medium", []float64{10, 0, 0}, "rawb", true, true)
	hudPlanet(s, "High", []float64{15, 0, 0}, "rawa", true, true)
	hudPlanet(s, "Unvalued site", []float64{1, 0, 0}, "", true, true)
	v.Obj(v.Obj(s.World["Unvalued site"]["bodies"])["1"])["surfaceFinds"] = []M{}
	v.Obj(ctx["settings"])["minTier"] = "high"
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"High"}) {
		t.Fatal("high-value filter ignored", got)
	}
	v.Obj(ctx["settings"])["minTier"] = "low"
	if got := s.HUDTargets(ctx, false); len(got) != 2 || v.Num(got[1]["score"]) < 48 {
		t.Fatal("low setting allowed an unvalued site into medium-or-better HUD recommendations", got)
	}
}

func TestHUDSurfaceAndRawTypesAreFilteredBeforeTheLimit(t *testing.T) {
	s, ctx := hudFixture()
	for index, name := range []string{"Raw only A", "Raw only B", "Raw only C"} {
		hudPlanet(s, name, []float64{float64(index + 1), 0, 0}, "rawa", true, false)
	}
	hudPlanet(s, "Actual mining site", []float64{10, 0, 0}, "", true, true)
	if got := hudNames(s.HUDTargets(ctx, false)); !reflect.DeepEqual(got, []string{"Actual mining site"}) {
		t.Fatal("raw-only planets were recommended for planetary extraction", got)
	}
	s, ctx = hudFixture()
	hudPlanet(s, "Site without materials A", []float64{1, 0, 0}, "", true, true)
	hudPlanet(s, "Site without materials B", []float64{2, 0, 0}, "", true, true)
	hudPlanet(s, "Cannot land", []float64{3, 0, 0}, "rawa", false, true)
	hudPlanet(s, "Raw location", []float64{15, 0, 0}, "rawb", true, false)
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Raw location"}) {
		t.Fatal("raw HUD limit was filled before checking landability and materials", got)
	}
}

func TestHUDUnknownDistanceOnlyAllowsTheNamedCurrentSystemAndInvalidatesItsCache(t *testing.T) {
	s, ctx := hudFixture()
	hudPlanet(s, "Current A", nil, "rawa", true, false)
	hudPlanet(s, "Current B", nil, "rawa", true, false)
	ctx["commander"] = M{"system": " current a "}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Current A"}) {
		t.Fatal("unknown remote distances were treated as zero", got)
	}
	key := s.hudTargetsKey
	ctx["commander"] = M{"system": "Current B"}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Current B"}) || s.hudTargetsKey == key {
		t.Fatal("system-only navigation reused recommendations from another system", got)
	}
	ctx["commander"] = M{}
	if rows := s.HUDTargets(ctx, true); len(rows) != 0 {
		t.Fatal("no location still produced remote recommendations", rows)
	}
}

func TestHUDRawEngineeringNeedAndCacheTrackRelevantDeficitKeysOnly(t *testing.T) {
	s, ctx := hudFixture()
	hudPlanet(s, "Raw A", []float64{1, 0, 0}, "rawa", true, false)
	hudPlanet(s, "Raw B", []float64{2, 0, 0}, "rawb", true, false)
	hudPlanet(s, "Raw C", []float64{3, 0, 0}, "rawc", true, false)
	ctx["engineering"] = M{"materials": []M{{"key": "rawb", "category": "raw", "needed": 2}}}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Raw B"}) {
		t.Fatal("unknown stock was silently treated as no deficit", got)
	}
	key := s.hudTargetsKey
	ctx["engineering"] = M{"materials": []M{{"key": "rawb", "category": "raw", "needed": 9, "missing": 5}}, "catalog": M{"irrelevant": "changed"}, "engineers": []M{{"rank": 5}}}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Raw B"}) || s.hudTargetsKey != key {
		t.Fatal("irrelevant engineering details invalidated a key-only site selection", got)
	}
	ctx["engineering"] = M{"materials": []M{{"key": "rawc", "category": "raw", "needed": 1, "missing": 1}, {"key": "rawa", "category": "raw", "needed": 0, "missing": 3}, {"key": "rawb", "category": "encoded", "needed": 9, "missing": 9}}}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Raw C"}) || s.hudTargetsKey == key {
		t.Fatal("changed engineering plan kept the old material target", got)
	}
	ctx["engineering"] = M{"materials": []M{{"key": "rawb", "category": "raw", "needed": 2, "missing": 0}}}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Raw A", "Raw B"}) {
		t.Fatal("a fully stocked plan was treated as an active deficit", got)
	}
}

func TestHUDTargetsPreferTheCurrentSystemBeforeNearbyHighScorePlanets(t *testing.T) {
	s, ctx := hudFixture()
	hudPlanet(s, "High A", []float64{1, 0, 0}, "rawa", true, true)
	hudPlanet(s, "High B", []float64{2, 0, 0}, "rawa", true, true)
	hudPlanet(s, "Origin", []float64{0, 0, 0}, "rawb", true, true)
	for _, raw := range []bool{false, true} {
		if got := hudNames(s.HUDTargets(ctx, raw)); !reflect.DeepEqual(got, []string{"Origin", "High A"}) {
			t.Fatal("nearby high scores displaced the current medium-value site", raw, got)
		}
	}
	all := BuildTargets(s.WorldSnapshot(), s.Signals, v.Rows(ctx["resources"]), v.Obj(ctx["commander"]), M{"method": "all"})
	if len(all) != 3 || all[0]["system"] == "Origin" || all[1]["system"] == "Origin" {
		t.Fatal("general target ranking was changed by the HUD-specific preference", all)
	}
}

func TestHUDTargetDistanceAndScoreTiesUseDeterministicIDs(t *testing.T) {
	s, ctx := hudFixture()
	for _, name := range []string{"Zeta", "Beta", "Alpha"} {
		hudPlanet(s, name, []float64{5, 0, 0}, "rawa", true, false)
	}
	if got := hudNames(s.HUDTargets(ctx, true)); !reflect.DeepEqual(got, []string{"Alpha", "Beta"}) {
		t.Fatal("equivalent HUD sites depend on map iteration order", got)
	}
}

func TestHUDFilteringPreservesCandidateDTOAndDoesNotMutateInputsOrCache(t *testing.T) {
	s, ctx := hudFixture()
	hudPlanet(s, "Candidate", []float64{10, 0, 0}, "rawb", true, true)
	beforeWorld, beforeContext := v.Clone(s.World), v.Clone(ctx)
	all := BuildTargets(s.WorldSnapshot(), s.Signals, v.Rows(ctx["resources"]), v.Obj(ctx["commander"]), M{"method": "all"})
	got := s.HUDTargets(ctx, true)
	if len(got) != 1 || len(all) != 1 || !reflect.DeepEqual(v.Clone(got[0]), v.Clone(all[0])) {
		t.Fatal("filter changed the selected candidate DTO", got, all)
	}
	got[0]["system"] = "mutated result"
	v.Rows(got[0]["materials"])[0]["percent"] = 999
	if s.HUDTargets(ctx, true)[0]["system"] != "Candidate" || !reflect.DeepEqual(beforeWorld, v.Clone(s.World)) || !reflect.DeepEqual(beforeContext, v.Clone(ctx)) {
		t.Fatal("HUD results alias their cache or observation inputs")
	}
}
