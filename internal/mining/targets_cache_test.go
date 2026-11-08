package mining

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/trading"
)

func targetCacheFixture() ([]M, map[string][]M) {
	world := []M{{"key": "alpha", "name": "Synthetic Alpha", "coordinates": []float64{0, 0, 0}, "bodies": []M{
		{"id": 1, "name": "Synthetic Alpha 1", "landable": true, "gravity": 0.8, "distanceLs": 200, "reserve": "PristineResources", "source": "journal", "timestamp": "2026-11-08T12:00:00Z", "rings": []M{{"name": "Alpha Metallic Ring", "type": "eRingClass_Metallic"}, {"name": "Alpha Rich Ring", "type": "eRingClass_MetalRich"}}, "materials": []M{{"key": "polonium", "percent": 1.2}, {"key": "carbon", "percent": 15}}, "surfaceFinds": []M{{"site": "Synthetic Site", "resources": []M{{"key": "gold", "name": "Золото", "count": 3}}}}},
		{"id": 2, "name": "Synthetic Alpha 2", "landable": false, "distanceLs": 12000, "reserve": "LowResources", "rings": []M{{"name": "Alpha Icy Ring", "type": "eRingClass_Icy"}, {"name": "Alpha Rocky Ring", "type": "eRingClass_Rocky"}}},
	}}}
	signals := map[string][]M{
		"alpha:Alpha Metallic Ring": {{"Type": "Платина", "Count": 2}, {"Type": "Unknown Synthetic Resource", "Count": 1}},
		"alpha:Alpha Rich Ring":     {{"Type": "painite", "Count": 1}},
		"alpha:Synthetic Alpha 1":   {{"Type": "$SAA_SignalType_PlanetaryMiningLocation;", "Count": 2}, {"Type": "$SAA_SignalType_Geological;", "Count": 1}},
	}
	return world, signals
}

func TestBuildTargetsPerCallCacheKeepsOriginalGolden(t *testing.T) {
	world, signals := targetCacheFixture()
	actual := map[string][]M{}
	for _, method := range []string{"all", "laser", "core", "surface", "surface-planet", "raw"} {
		actual[method] = BuildTargets(world, signals, trading.Resources(), M{"coordinates": []float64{3, 0, 0}}, M{"method": method})
	}
	got, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "targets-before-percall-cache.json")
	if os.Getenv("ED_COPILOT_RECORD_TARGETS_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("target evaluation changed from the original all-method golden")
	}
}

func TestBuildTargetsTemplatesStayLocalAndOutputsDoNotAliasInputsOrEachOther(t *testing.T) {
	world := []M{{"key": "synthetic", "name": "Synthetic", "bodies": []M{{"id": 1, "name": "Synthetic 1", "rings": []M{{"name": "A Ring", "type": "metallic"}, {"name": "B Ring", "type": "metallic"}}}}}}
	resources := trading.Resources()
	before, _ := json.Marshal(resources)
	rows := BuildTargets(world, nil, resources, nil, M{"method": "all"})
	if len(rows) != 2 {
		t.Fatal("fixture must exercise reused ring templates", len(rows))
	}
	first, second := vRowsResource(rows[0]), vRowsResource(rows[1])
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("fixture has no valuable ring resources")
	}
	methods := first[0]["methods"].([]string)
	if len(methods) == 0 {
		t.Fatal("fixture method list empty")
	}
	methods[0] = "external mutation"
	if second[0]["methods"].([]string)[0] == "external mutation" {
		t.Fatal("two target rows alias a cached methods list")
	}
	after, _ := json.Marshal(resources)
	if !bytes.Equal(before, after) {
		t.Fatal("building or editing output mutated resource inputs")
	}
	for _, resource := range resources {
		if resource["key"] == "platinum" {
			resource["tier"] = "low"
		}
	}
	rows = BuildTargets(world, map[string][]M{"synthetic:A Ring": {{"Type": "Платина", "Count": 1}}}, resources, nil, M{"method": "all"})
	for _, row := range rows {
		if row["ring"] == "A Ring" {
			for _, resource := range vRowsResource(row) {
				if resource["key"] == "platinum" {
					if resource["tier"] != "low" {
						t.Fatal("alias/template cache outlived its call or ignored caller resource data")
					}
					return
				}
			}
		}
	}
	t.Fatal("localized platinum alias missing")
}

func vRowsResource(row M) []M {
	if items, ok := row["resources"].([]M); ok {
		return items
	}
	return nil
}

func TestBuildTargetsDoesNotMemoizeUnknownSignalDisplayNames(t *testing.T) {
	world := []M{{"key": "s", "name": "Synthetic", "bodies": []M{{"id": 1, "name": "Synthetic 1", "rings": []M{{"name": "A Ring", "type": "metallic"}, {"name": "B Ring", "type": "metallic"}}}}}}
	signals := map[string][]M{"s:A Ring": {{"Type": "UnknownSymbol", "Type_Localised": "Первое название"}}, "s:B Ring": {{"Type": "UnknownSymbol", "Type_Localised": "Второе название"}}}
	rows := BuildTargets(world, signals, trading.Resources(), nil, M{"method": "all"})
	want := map[string]string{"A Ring": "Первое название", "B Ring": "Второе название"}
	for _, row := range rows {
		values := vRowsResource(row)
		if len(values) != 1 || values[0]["name"] != want[fmt.Sprint(row["ring"])] {
			t.Fatal("unknown signal inherited a different observation's label", row)
		}
	}
}

func BenchmarkBuildTargetsLargeWorld(b *testing.B) {
	world := make([]M, 0, 500)
	for index := 0; index < 500; index++ {
		name := fmt.Sprintf("Synthetic %04d", index)
		bodies := []M{}
		for bodyIndex := 0; bodyIndex < 10; bodyIndex++ {
			body := fmt.Sprint(name, " ", bodyIndex)
			bodies = append(bodies, M{"id": bodyIndex, "name": body, "landable": true, "rings": []M{{"name": body + " Ring", "type": "eRingClass_Metallic"}}, "materials": []M{{"key": "polonium", "percent": 1}}})
		}
		world = append(world, M{"key": name, "name": name, "coordinates": []float64{float64(index), 0, 0}, "bodies": bodies})
	}
	resources := trading.Resources()
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		BuildTargets(world, nil, resources, M{"coordinates": []float64{0, 0, 0}}, M{"method": "all"})
	}
}
