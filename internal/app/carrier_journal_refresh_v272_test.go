package app

import (
	"context"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
	"time"
)

func TestCarrierJournalTransferWithoutIDsRefreshesAllViewsWithoutReplay(t *testing.T) {
	e, file, _ := fixtureEngine(t, M{"settings": M{"activity": "colonization"}})
	at := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	writeEvents(t, file, true, []M{
		{"event": "CarrierStats", "timestamp": v.ISO(at.Add(-time.Minute)), "CarrierID": 777, "Callsign": "SYN-777", "Name": "Synthetic Carrier"},
		{"event": "Docked", "timestamp": v.ISO(at.Add(-time.Minute)), "MarketID": 777, "StationType": "FleetCarrier", "StationName": "SYN-777"},
	})
	e.Reader.Poll()
	baseline := M{"status": M{"connected": true, "fid": "FSYNTHETIC"}, "data": M{"carrier": M{"market": M{"id": 777}, "name": M{"callsign": "SYN-777"}, "cargo": []M{{"commodity": "steel", "qty": 20}}}}, "metadata": M{"source": "Frontier CAPI", "observedAt": M{"carrier": v.ISO(at)}, "requestStartedAt": M{"carrier": v.ISO(at.Add(-time.Second))}}}
	if !e.consumeFrontierResult(context.Background(), baseline) {
		t.Fatal("fixture baseline rejected")
	}
	if _, err := e.Do("/api/colonization", M{"action": "configure", "includeCarrierWarehouse": true}); err != nil {
		t.Fatal(err)
	}
	before := e.Reader.State()
	revision := v.Num(e.SnapshotForHUD()["revision"])
	for len(e.updates) > 0 {
		<-e.updates
	}
	writeEvents(t, file, true, []M{{"event": "CargoTransfer", "timestamp": v.ISO(at.Add(time.Minute)), "Transfers": []M{{"Type": "steel", "Count": 7, "Direction": "tocarrier"}}}})
	e.Reader.Poll()
	for _, state := range []M{e.SnapshotForHUD(), e.SnapshotForUI("colonization"), e.SnapshotForUI("carrier")} {
		if v.Num(v.At(state, "carrier", "warehouse", "knownTons")) != 27 {
			t.Fatal("carrier view did not update", v.At(state, "carrier", "warehouse"))
		}
		if colony := v.Obj(state["colonization"]); colony["carrierWarehouses"] != nil {
			fleet := v.Rows(colony["carrierWarehouses"])[0]
			if !v.Bool(fleet["known"]) || v.Num(fleet["knownTons"]) != 27 {
				t.Fatal("colony/HUD warehouse did not update", fleet)
			}
		}
	}
	after := e.Reader.State()
	if v.Num(after["eventCount"]) != v.Num(before["eventCount"])+1 || v.Num(e.SnapshotForHUD()["revision"]) <= revision || v.Bool(after["scanning"]) {
		t.Fatal("live update required replay or did not revise UI", before, after)
	}
	select {
	case <-e.Updates():
	default:
		t.Fatal("missing UI notification")
	}
}
