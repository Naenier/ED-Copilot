package carrier

import (
	"reflect"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestRouteActualAndMinimumPayloadKeepTheirDifferentInventorySemantics(t *testing.T) {
	snapshot := M{"carrier": M{"location": M{"system": "Synthetic A"}, "space": M{"capacityTons": 25000, "freeTons": 18013}, "fuel": M{"tankTons": 1000, "reserveTons": 2079}}}
	before := v.Copy(snapshot)
	systems := []M{{"id64": "111", "name": "Synthetic A"}, {"id64": "222", "name": "Synthetic B"}}
	actual, err := NormalizeRouteOptions(M{"destinationSystem": "Synthetic B", "calculateStartingFuel": false}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	payload := BuildRoutePayload(actual, systems, nil)
	if v.Num(payload["capacity_used"]) != 6987 || v.Num(payload["fuel_loaded"]) != 1000 || v.Num(payload["tritium_stored"]) != 2079 || v.Num(payload["calculate_starting_fuel"]) != 0 {
		t.Fatal("actual route silently unloaded fuel", payload)
	}
	minimum, err := NormalizeRouteOptions(M{"destinationSystem": "Synthetic B", "calculateStartingFuel": true}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	proposed := BuildRoutePayload(minimum, systems, nil)
	if v.Num(proposed["capacity_used"]) != 4908 || v.Num(proposed["calculate_starting_fuel"]) != 1 || proposed["fuel_loaded"] != nil || proposed["tritium_stored"] != nil {
		t.Fatal("minimum payload no longer represents a hypothetical optimized load", proposed)
	}
	if !reflect.DeepEqual(before, v.Copy(snapshot)) {
		t.Fatal("route planning modified observed carrier inventory")
	}
}
