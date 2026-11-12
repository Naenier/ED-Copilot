package carrier

import (
	"math"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const routeLowTankTons = 100

// The provider's stop balances already include its automatic top-ups. Use
// the starting inventory from the request/result, then model our own stops.
func plannedStartingFuel(raw, first, options M) (float64, float64, bool) {
	providerTank, tankOK := v.Number(first["tankTons"])
	providerCargo, cargoOK := v.Number(first["cargoTritiumTons"])
	if !tankOK || !cargoOK || providerTank < 0 || providerTank > TankCapacity || providerCargo < 0 || providerCargo > Capacity {
		return 0, 0, false
	}
	if v.Bool(options["calculateStartingFuel"]) {
		// For minimum mode the origin stop includes the hypothetical preload.
		return providerTank, providerCargo, true
	}
	originStock := providerTank + providerCargo - v.Num(first["restockTons"])
	for _, sources := range [][2]any{{raw["fuel_loaded"], raw["tritium_stored"]}, {options["tankTons"], options["cargoTritiumTons"]}} {
		tank, ok := v.Number(sources[0])
		if !ok || tank < 0 || tank > TankCapacity {
			continue
		}
		cargo, known := v.Number(sources[1])
		if !known {
			cargo = originStock - tank
		}
		if cargo >= 0 && cargo <= Capacity && math.Abs(tank+cargo-originStock) < .01 {
			return tank, cargo, true
		}
	}
	// Older saves may only contain the provider's first stop. Undo its
	// external acquisition, preserving total inventory when possible.
	if originStock < 0 {
		return 0, 0, false
	}
	tank := math.Min(providerTank, originStock)
	return tank, originStock - tank, true
}

func routeTritiumCapacity(used any, cargo float64) float64 {
	occupied, known := v.Number(used)
	if !known || occupied < cargo {
		return Capacity
	}
	return math.Max(cargo, Capacity-(occupied-cargo))
}

// Each row shows inventory upon arrival. Transfer and acquisition take
// place at that stop, before the next jump. An unconfirmed inventory never
// becomes a fabricated zero or permits a jump that cannot be funded.
func projectRouteFuel(stops []M, start int, tank float64, cargoValue any, cargoCapacity, startRestock float64) bool {
	cargo, cargoKnown := v.Number(cargoValue)
	reachable := true
	for index := start; index < len(stops); index++ {
		stop := stops[index]
		for _, field := range []string{"tankTons", "tankBeforeTopupTons", "tankAfterTons", "fuelRemainingTons", "cargoTritiumTons", "tankAfterTopupTons", "cargoAfterTopupTons", "transferTons", "fuelShortfallTons"} {
			delete(stop, field)
		}
		if !reachable {
			continue
		}
		stop["tankTons"], stop["tankBeforeTopupTons"] = tank, tank
		if cargoKnown {
			stop["cargoTritiumTons"] = cargo
		}
		acquire := v.Num(stop["restockTons"])
		if index == start {
			acquire = startRestock
		}
		next := index+1 < len(stops)
		required := 0.0
		if next {
			var known bool
			required, known = v.Number(stops[index+1]["fuelUsedTons"])
			if !known || required < 0 {
				reachable = false
				continue
			}
		}
		topUp := next && (tank < routeLowTankTons || tank < required)
		switch {
		case topUp && cargoKnown:
			transfer := math.Min(TankCapacity-tank, cargo)
			stop["transferTons"] = transfer
			tank += transfer
			cargo -= transfer
		case !topUp:
			stop["transferTons"] = 0
		}
		if acquire > 0 {
			// Acquired tritium may go straight into the tank when required
			// or when it would otherwise exceed the warehouse capacity.
			direct := math.Max(0, required-tank)
			if topUp {
				direct = TankCapacity - tank
			}
			if cargoKnown {
				direct = math.Max(direct, acquire-(cargoCapacity-cargo))
			}
			direct = math.Min(math.Min(acquire, TankCapacity-tank), direct)
			tank += direct
			cargo += acquire - direct
		}
		stop["tankAfterTopupTons"] = tank
		if cargoKnown {
			stop["cargoAfterTopupTons"] = cargo
		}
		if required > tank+.01 {
			if cargoKnown {
				stop["fuelShortfallTons"] = required - tank
			}
			reachable = false
		}
		tank = math.Max(0, tank-required)
	}
	return reachable
}

func restoreRouteFuel(state M) {
	route := v.Obj(state["route"])
	stops := v.Rows(route["stops"])
	if len(stops) < 2 || route["fuelModel"] == "arrival" {
		return
	}
	options := v.Obj(route["options"])
	if len(options) == 0 {
		options = v.Obj(state["options"])
	}
	raw := v.Obj(route["providerResult"])
	tank, cargo, ok := plannedStartingFuel(raw, stops[0], options)
	if !ok {
		return
	}
	firstRestock := v.Num(stops[0]["restockTons"])
	if v.Bool(options["calculateStartingFuel"]) {
		firstRestock = 0
	}
	capacity := routeTritiumCapacity(v.First(raw["capacity_used"], options["usedCapacityTons"]), cargo)
	// An unfunded leg leaves subsequent balances unknown, preserving visits.
	projectRouteFuel(stops, 0, tank, cargo, capacity, firstRestock)
	updateRouteFuelSummary(v.Obj(state["summary"]), stops)
	state["assumptions"] = RouteAssumptions(options)
	route["fuelModel"] = "arrival"
}

func updateRouteFuelSummary(summary M, stops []M) {
	first, last := stops[0], stops[len(stops)-1]
	summary["startingTankTons"] = first["tankTons"]
	summary["startingCargoTritiumTons"] = first["cargoTritiumTons"]
	summary["startingTritiumTons"] = v.Num(first["tankTons"]) + v.Num(first["cargoTritiumTons"])
	summary["arrivalTankTons"] = last["tankAfterTopupTons"]
	summary["arrivalCargoTritiumTons"] = last["cargoAfterTopupTons"]
	summary["arrivalTritiumTons"], summary["reserveShortfallTons"] = nil, nil
	if arrival, known := routeFuelTotal(last); known {
		summary["arrivalTritiumTons"] = arrival
		summary["reserveShortfallTons"] = math.Max(0, v.Num(summary["reserveTritiumTons"])-arrival)
	}
}

func routeFuelTotal(stop M) (float64, bool) {
	tank, tankKnown := v.Number(stop["tankAfterTopupTons"])
	cargo, cargoKnown := v.Number(stop["cargoAfterTopupTons"])
	return tank + cargo, tankKnown && cargoKnown
}

// Live observations affect a detached display/export projection only. A
// planned origin elsewhere or a past occurrence in a loop cannot be rebased.
func applyLiveRouteFuel(state, observed M) {
	route := v.Obj(state["route"])
	stops := v.Rows(route["stops"])
	fuel, location := v.Obj(observed["fuel"]), v.Obj(observed["location"])
	tank, tankKnown := v.Number(fuel["tankTons"])
	if len(stops) < 2 || !v.Bool(observed["owned"]) || v.Bool(fuel["needsRefresh"]) || !tankKnown || tank < 0 || tank > TankCapacity {
		return
	}
	current := 0
	for current+1 < len(stops) && v.Bool(stops[current+1]["visited"]) {
		current++
	}
	stop := stops[current]
	matches := v.Str(location["system"]) != "" && strings.EqualFold(v.Str(location["system"]), v.Str(stop["name"]))
	if liveID, stopID := v.Str(location["systemAddress"]), v.Str(stop["id64"]); liveID != "" && stopID != "" {
		matches = liveID == stopID
	}
	if !matches {
		return
	}
	if arrival := v.Time(v.At(observed, "jump", "lastArrival")); !arrival.IsZero() && v.Time(fuel["updatedAt"]).Before(arrival) {
		return
	}
	cargo, cargoKnown := v.Number(fuel["reserveTons"])
	cargoKnown = cargoKnown && cargo >= 0 && cargo <= Capacity
	stock := v.Nullable(cargo, cargoKnown)
	estimated := v.Bool(fuel["tankEstimated"])
	live := M{"system": stop["name"], "index": current, "tankTons": tank, "cargoTritiumTons": stock, "updatedAt": fuel["updatedAt"], "estimated": estimated}
	state["liveFuel"] = live
	options := v.Obj(route["options"])
	if len(options) == 0 {
		options = v.Obj(state["options"])
	}
	if v.Bool(options["calculateStartingFuel"]) {
		return
	}
	used := options["usedCapacityTons"]
	if capacity, capacityKnown := v.Number(v.At(observed, "space", "capacityTons")); capacityKnown {
		if free, freeKnown := v.Number(v.At(observed, "space", "freeTons")); freeKnown && free >= 0 && free <= capacity {
			used = capacity - free
		}
	}
	plannedRestock := v.Num(stop["restockTons"])
	if expectedAfter, known := routeFuelTotal(stop); known && cargoKnown {
		plannedRestock = math.Min(plannedRestock, math.Max(0, expectedAfter-tank-cargo))
	}
	stop["restockTons"] = plannedRestock
	live["reachable"] = projectRouteFuel(stops, current, tank, stock, routeTritiumCapacity(used, cargo), plannedRestock)
	stop["tankObserved"] = !estimated
	stop["tankEstimated"] = estimated
	live["projected"] = true
	last := stops[len(stops)-1]
	live["arrivalTankTons"], live["arrivalCargoTritiumTons"] = last["tankAfterTopupTons"], last["cargoAfterTopupTons"]
	if total, known := routeFuelTotal(last); known {
		live["reserveShortfallTons"] = math.Max(0, v.Num(v.At(state, "summary", "reserveTritiumTons"))-total)
	}
}
