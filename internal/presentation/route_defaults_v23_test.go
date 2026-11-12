package presentation

import (
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestFreshRouteFormDefaultsToKnownActualCarrierLoad(t *testing.T) {
	s := M{"commander": M{"system": "Synthetic A"}, "carrierRoute": carrier.EmptyRouteState(), "carrier": M{"location": M{"system": "Synthetic A"}, "fuel": M{"tankTons": 1000, "reserveTons": 2079}, "space": M{"capacityTons": 25000, "freeTons": 18013}}}
	a := coreAction(t, "route", "Остановки", "route.calculate", s, nil, nil)
	body := coreBody(t, a, M{"destinationSystem": "Synthetic B"})
	if body["calculateStartingFuel"] != false {
		t.Fatal("fresh form opted known actual fuel into hypothetical minimum", body)
	}
	options, err := carrier.NormalizeRouteOptions(body, s)
	if err != nil {
		t.Fatal(err)
	}
	payload := carrier.BuildRoutePayload(options, []M{{"name": "Synthetic A", "id64": "111"}, {"name": "Synthetic B", "id64": "222"}}, nil)
	if v.Num(payload["capacity_used"]) != 6987 || v.Num(payload["fuel_loaded"]) != 1000 || v.Num(payload["tritium_stored"]) != 2079 {
		t.Fatal("UI form changed observed carrier loading", payload)
	}
	help := coreField(t, a, "calculateStartingFuel").Help
	if !strings.Contains(help, "фактические") || !strings.Contains(help, "гипотетический") {
		t.Fatal("minimum/actual choice is not explained", help)
	}
}

func TestFreshRouteMinimumPolicyDistinguishesUnknownFuelFromKnownZero(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fuel    M
		minimum bool
	}{{"unknown", M{}, true}, {"null", M{"tankTons": nil, "reserveTons": nil}, true}, {"known zero tank", M{"tankTons": 0}, false}, {"known zero warehouse", M{"reserveTons": 0}, false}, {"known tank", M{"tankTons": 500}, false}, {"known warehouse", M{"reserveTons": 200}, false}} {
		t.Run(tc.name, func(t *testing.T) {
			s := M{"commander": M{"system": "Synthetic A"}, "carrierRoute": carrier.EmptyRouteState(), "carrier": M{"fuel": tc.fuel}}
			a := coreAction(t, "route", "", "route.calculate", s, nil, nil)
			if got := coreField(t, a, "calculateStartingFuel").Default; got != tc.minimum {
				t.Fatalf("minimum=%v want=%v", got, tc.minimum)
			}
		})
	}
}

func TestSavedExplicitMinimumChoiceIsNotOverriddenByCarrierSnapshot(t *testing.T) {
	for _, minimum := range []bool{true, false} {
		s := M{"commander": M{"system": "Synthetic A"}, "carrier": M{"fuel": M{"tankTons": 1000, "reserveTons": 2079}}, "carrierRoute": M{"status": "idle", "options": M{"sourceSystem": "Saved origin", "destinationSystem": "Saved destination", "calculateStartingFuel": minimum, "tankTons": 300, "cargoTritiumTons": 100, "usedCapacityTons": 500}}}
		a := coreAction(t, "route", "", "route.calculate", s, nil, nil)
		body := coreBody(t, a, nil)
		if body["calculateStartingFuel"] != minimum || v.Num(body["tankTons"]) != 300 || v.Num(body["cargoTritiumTons"]) != 100 || body["sourceSystem"] != "Saved origin" {
			t.Fatal("saved user choice replaced by default policy", body)
		}
	}
}

func TestMatchingCarrierRefreshesSavedRouteFormFuel(t *testing.T) {
	s := M{
		"carrier":      M{"location": M{"system": "A"}, "fuel": M{"tankTons": 742, "reserveTons": 2079}, "space": M{"capacityTons": 25000, "freeTons": 18013}},
		"carrierRoute": M{"status": "ready", "options": M{"sourceSystem": "A", "destinationSystem": "B", "calculateStartingFuel": false, "tankTons": 1000, "cargoTritiumTons": 1967, "usedCapacityTons": 6800}},
	}
	a := coreAction(t, "route", "", "route.calculate", s, nil, nil)
	body := coreBody(t, a, nil)
	if v.Num(body["tankTons"]) != 742 || v.Num(body["cargoTritiumTons"]) != 2079 || v.Num(body["usedCapacityTons"]) != 6987 {
		t.Fatal("saved plan masked current carrier load", body)
	}
}
