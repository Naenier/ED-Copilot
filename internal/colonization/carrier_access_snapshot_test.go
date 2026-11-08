package colonization

import (
	"testing"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func TestCarrierWarehouseAccessRemainsVisibleIndependentlyOfAccounting(t *testing.T) {
	p, _, context := stockFixture(t)
	p.ApplyCAPI(stockResult(capiStock("42", 120), capiStock("43", 30), testNow), value.Obj(context["commander"]))
	assert := func(wantPersonal, wantSquadron, wantIncluded bool) {
		t.Helper()
		warehouses := rows(p.Snapshot(context)["carrierWarehouses"])
		if value.Bool(warehouses[0]["accessible"]) != wantPersonal || value.Bool(warehouses[1]["accessible"]) != wantSquadron || value.Bool(warehouses[0]["included"]) != wantIncluded {
			t.Fatal("physical access and opt-in accounting were conflated", warehouses)
		}
	}
	assert(true, false, true)
	action(t, p, M{"action": "configure", "includeCarrierWarehouse": false})
	assert(true, false, false)
	action(t, p, M{"action": "configure", "squadronWarehouseAccessConfirmed": true})
	assert(true, true, false)
	context["commander"] = M{"fid": "F2", "vehicle": "ship", "cargoForSync": []M{}}
	assert(false, false, false)
}
