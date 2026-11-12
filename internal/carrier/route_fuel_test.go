package carrier

import (
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func fullTankProviderRoute() M {
	response := routeFixture()
	result := v.Obj(response["result"])
	result["fuel_loaded"], result["tritium_stored"], result["capacity_used"] = 742, 2079, 6987
	for i, row := range v.Rows(result["jumps"]) {
		row["fuel_used"] = []int{0, 87, 25}[i]
		row["fuel_in_tank"] = 1000
		row["tritium_in_market"] = []int{1821, 1734, 1709}[i]
	}
	return response
}

func TestRouteFuelStartsAtActualTankAndDebitsTankWithoutUnneededTransfers(t *testing.T) {
	options := routeOptions(t)
	options["tankTons"], options["cargoTritiumTons"], options["usedCapacityTons"] = 742, 2079, 6987
	route, err := NormalizeProviderRoute(fullTankProviderRoute(), options, M{})
	if err != nil {
		t.Fatal(err)
	}
	for i, stop := range v.Rows(v.At(route, "route", "stops")) {
		closeNumber(t, stop["tankTons"], []float64{742, 655, 630}[i])
		closeNumber(t, stop["cargoTritiumTons"], 2079)
		closeNumber(t, stop["transferTons"], 0)
		closeNumber(t, stop["restockTons"], 0)
	}
	closeNumber(t, v.At(route, "summary", "arrivalTankTons"), 630)
	closeNumber(t, v.At(route, "summary", "arrivalCargoTritiumTons"), 2079)
}

func TestRouteFuelSeparatesWarehouseTransferAndExternalRestock(t *testing.T) {
	response := routeFixture()
	raw := v.Obj(response["result"])
	raw["fuel_loaded"], raw["tritium_stored"], raw["capacity_used"] = 100, 100, 100
	jumps := v.Rows(raw["jumps"])
	jumps[0]["fuel_in_tank"] = 200
	jumps[1]["fuel_used"], jumps[1]["fuel_in_tank"], jumps[1]["restock_amount"] = 87, 213, 100
	jumps[2]["fuel_used"], jumps[2]["fuel_in_tank"] = 120, 93
	options := routeOptions(t)
	options["tankTons"], options["cargoTritiumTons"], options["usedCapacityTons"] = 100, 100, 100
	result, err := NormalizeProviderRoute(response, options, M{})
	if err != nil {
		t.Fatal(err)
	}
	stops := v.Rows(v.At(result, "route", "stops"))
	closeNumber(t, stops[0]["transferTons"], 0)
	closeNumber(t, stops[1]["tankTons"], 13)
	closeNumber(t, stops[1]["transferTons"], 100)
	closeNumber(t, stops[1]["restockTons"], 100)
	closeNumber(t, stops[1]["tankAfterTopupTons"], 213)
	closeNumber(t, stops[2]["tankTons"], 93)
	closeNumber(t, stops[2]["transferTons"], 0)
}

func TestSavedRouteFuelMigratesAndUsesFreshMatchingCarrierWithoutMutatingPlan(t *testing.T) {
	route, err := NormalizeProviderRoute(fullTankProviderRoute(), routeOptions(t), M{})
	if err != nil {
		t.Fatal(err)
	}
	stored := v.Obj(route["route"])
	delete(stored, "fuelModel")
	for i, stop := range v.Rows(stored["stops"]) {
		stop["tankTons"] = 1000
		stop["cargoTritiumTons"] = []int{1821, 1734, 1709}[i]
		stop["transferTons"] = []int{258, 87, 25}[i]
	}
	planner := NewRoutePlanner(route)
	if err := planner.MarkVisited(1, "manual", "2026-11-07T12:30:00Z"); err != nil {
		t.Fatal(err)
	}
	observed := M{"owned": true, "location": M{"system": "B", "systemAddress": "18446744073709551614"}, "fuel": M{"tankTons": 600, "reserveTons": 2010, "updatedAt": "2026-11-07T12:31:00Z"}}
	live := planner.SnapshotForCarrier(observed)
	stops := v.Rows(v.At(live, "route", "stops"))
	closeNumber(t, stops[0]["tankTons"], 742)
	closeNumber(t, stops[1]["tankTons"], 600)
	closeNumber(t, stops[2]["tankTons"], 575)
	if !v.Bool(stops[1]["tankObserved"]) || !v.Bool(stops[1]["visited"]) || v.At(live, "liveFuel", "projected") != true {
		t.Fatal("fresh matching tank did not project from the confirmed stop", stops[1], live["liveFuel"])
	}
	saved := planner.ExportSettings()
	persisted := v.Rows(v.At(saved, "state", "route", "stops"))
	if v.At(saved, "state", "liveFuel") != nil || persisted[1]["tankObserved"] != nil {
		t.Fatal("carrier observation leaked into planned route state")
	}
	closeNumber(t, persisted[1]["tankTons"], 655)
	closeNumber(t, v.At(saved, "state", "summary", "completedJumps"), 1)

	v.Obj(observed["fuel"])["tankEstimated"] = true
	estimated := planner.SnapshotForCarrier(observed)
	estimateRow := v.Rows(v.At(estimated, "route", "stops"))[1]
	if v.Bool(estimateRow["tankObserved"]) || !v.Bool(estimateRow["tankEstimated"]) {
		t.Fatal("estimated tank was promoted to an observation", estimateRow)
	}
	v.Obj(observed["location"])["systemAddress"] = "1"
	if got := planner.SnapshotForCarrier(observed); got["liveFuel"] != nil {
		t.Fatal("matching text with a conflicting ID attached unrelated fuel")
	}
}

func TestRouteLiveShortageLeavesUnreachableTankUnknown(t *testing.T) {
	options := routeOptions(t)
	result, err := NormalizeProviderRoute(fullTankProviderRoute(), options, M{})
	if err != nil {
		t.Fatal(err)
	}
	planner := NewRoutePlanner(result)
	observed := M{"owned": true, "location": M{"system": "A"}, "fuel": M{"tankTons": 50, "reserveTons": 0}}
	snapshot := planner.SnapshotForCarrier(observed)
	stops := v.Rows(v.At(snapshot, "route", "stops"))
	closeNumber(t, stops[0]["tankTons"], 50)
	closeNumber(t, stops[0]["fuelShortfallTons"], 37)
	if stops[1]["tankTons"] != nil || stops[2]["tankTons"] != nil || v.At(snapshot, "liveFuel", "arrivalTankTons") != nil {
		t.Fatal("unreachable stop retained a fabricated fuel amount", stops)
	}
	csv, err := RouteCSV(snapshot)
	if err != nil || !strings.Contains(csv, "Перелить со склада") || !strings.Contains(csv, "Получить извне") {
		t.Fatal(csv, err)
	}
}
