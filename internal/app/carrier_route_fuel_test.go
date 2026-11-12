package app

import (
	"encoding/csv"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRouteViewAndCSVFollowConfirmedTankUpdates(t *testing.T) {
	engine, _, _ := fixtureEngine(t, M{"carrierRouteGo": M{"state": tradeIntegrationRoute(t)}})
	engine.Apply(testEvent("CarrierStats", M{"timestamp": "2026-11-08T13:00:00Z", "CarrierID": 42, "FuelLevel": 742}), nil)
	engine.Apply(testEvent("CarrierLocation", M{"timestamp": "2026-11-08T13:00:01Z", "CarrierID": 42, "StarSystem": "Synthetic A", "SystemAddress": 111}), nil)
	check := func(wantTank, wantArrival float64) {
		t.Helper()
		state := engine.SnapshotForUI("route")
		if tank := v.At(state, "carrier", "fuel", "tankTons"); v.Num(tank) != wantTank {
			t.Fatalf("unexpected observed tank: %v", tank)
		}
		stops := v.Rows(v.At(state, "carrierRoute", "route", "stops"))
		if len(stops) != 2 || !v.Bool(stops[0]["tankObserved"]) || v.Num(stops[0]["tankTons"]) != wantTank || v.Num(stops[1]["tankTons"]) != wantArrival {
			t.Fatalf("live route did not use the tracker snapshot: %+v", stops)
		}
	}
	check(742, 642)
	engine.Apply(testEvent("CarrierDepositFuel", M{"timestamp": "2026-11-08T13:01:00Z", "CarrierID": 42, "Amount": 100, "Total": 842}), nil)
	check(842, 742)
	export, err := engine.ExportCSV("route")
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(export, "\ufeff")))
	reader.Comma = ';'
	rows, err := reader.ReadAll()
	if err != nil || len(rows) != 3 || rows[1][9] != "842" || rows[2][9] != "742" {
		t.Fatalf("CSV did not use the same fuel projection: %v %v", rows, err)
	}
}
