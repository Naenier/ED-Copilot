package carrier

import (
	"math"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func validTank(raw any) (float64, bool) {
	tank, known := v.Number(raw)
	return tank, known && tank >= 0 && tank <= TankCapacity
}

func setTank(fuel M, tank float64, at any, estimated bool) {
	fuel["tankTons"], fuel["updatedAt"] = tank, at
	fuel["needsRefresh"], fuel["tankEstimated"] = false, estimated
}

func sameSystem(a, b M) bool {
	leftID, rightID := v.Str(a["systemAddress"]), v.Str(b["systemAddress"])
	if leftID != "" && rightID != "" {
		return leftID == rightID
	}
	left, right := v.Str(a["system"]), v.Str(b["system"])
	return left != "" && strings.EqualFold(left, right)
}

func sameOrbit(a, b M) bool {
	return sameSystem(a, b) && (a["bodyId"] == nil || b["bodyId"] == nil || v.Str(a["bodyId"]) == v.Str(b["bodyId"]))
}

func fleetJumpFuel(distance, occupied, tank float64) float64 {
	return math.Round(5 + distance*(Capacity+occupied+tank)/200000)
}

// CarrierLocation can arrive before the corresponding CarrierJump and lacks
// coordinates. Keep the departure location until that jump can be reconciled.
func observeCarrierArrival(c, origin, destination, pending, event M) {
	at := v.Time(event["timestamp"])
	if at.IsZero() {
		return
	}
	jump, fuel := v.Obj(c["jump"]), v.Obj(c["fuel"])
	last := v.Obj(jump["fuelLeg"])
	lastAt := v.Time(last["at"])
	if !lastAt.IsZero() && at.Before(lastAt) {
		return
	}
	requestAt := v.Str(pending["requestedAt"])
	duplicate := !lastAt.IsZero() && at.Sub(lastAt) < 5*time.Minute && sameOrbit(destination, v.Obj(last["destination"])) &&
		(requestAt == "" || requestAt == v.Str(last["requestAt"]))
	if !duplicate {
		from := v.Obj(jump["origin"])
		if len(from) == 0 || requestAt == "" {
			from = origin
		}
		last = M{"origin": v.Copy(from), "destination": v.Copy(destination), "at": event["timestamp"], "requestAt": requestAt}
		jump["fuelLeg"], jump["lastArrival"] = last, event["timestamp"]
		jump["origin"] = nil
		jump["pending"] = nil
	} else if destination["coordinates"] != nil {
		v.Obj(last["destination"])["coordinates"] = destination["coordinates"]
	}
	if tank, known := validTank(event["FuelLevel"]); known {
		if !v.Time(fuel["updatedAt"]).After(at) {
			setTank(fuel, tank, event["timestamp"], false)
		}
		return
	}
	fuelAt := v.Time(fuel["updatedAt"])
	if duplicate && !v.Bool(fuel["needsRefresh"]) || fuelAt.After(at) {
		return
	}
	// A stale pre-jump tank can only be retried for this same unresolved leg.
	if !duplicate && v.Bool(fuel["needsRefresh"]) {
		return
	}
	before, known := validTank(fuel["tankTons"])
	last["beforeTank"] = v.Nullable(before, known)
	space := v.Obj(c["space"])
	total, free := v.Num(space["capacityTons"]), v.Num(space["freeTons"])
	_, freeKnown := v.Number(space["freeTons"])
	estimated := false
	if known && total == Capacity && freeKnown && free >= 0 && free <= total {
		from, to := v.Obj(last["origin"]), v.Obj(last["destination"])
		distance, given := v.Number(event["JumpDist"])
		if !given {
			distance = trading.Distance(from["coordinates"], to["coordinates"])
			if math.IsNaN(distance) && sameSystem(from, to) && from["bodyId"] != nil && to["bodyId"] != nil && !sameOrbit(from, to) {
				distance = 0
			}
		}
		if distance >= 0 && distance <= 500.01 && (distance > 0 || sameSystem(from, to) && !sameOrbit(from, to)) {
			cost := fleetJumpFuel(distance, total-free, before)
			if cost <= before {
				setTank(fuel, before-cost, event["timestamp"], true)
				estimated = true
			}
		}
	}
	if !estimated {
		fuel["needsRefresh"], fuel["tankEstimated"] = true, false
	}
}

func applyCAPITank(c, record M, at, started time.Time) {
	fuel, location := v.Obj(c["fuel"]), v.Obj(c["location"])
	tank, known := validTank(record["fuel"])
	remoteSystem := v.Text(v.At(record, "currentStarSystem", "name"), record["currentStarSystem"])
	if !known || v.Str(location["system"]) != "" && !sameSystem(location, M{"system": remoteSystem}) {
		return
	}
	lastFuel := v.Time(fuel["updatedAt"])
	lastJump := v.Time(v.At(c, "jump", "lastArrival"))
	if !started.After(lastFuel) || !started.After(lastJump) || !at.After(lastFuel) {
		return
	}
	if before, ok := v.Number(v.At(c, "jump", "fuelLeg", "beforeTank")); ok && tank == before &&
		(v.Bool(fuel["needsRefresh"]) || v.Bool(fuel["tankEstimated"])) && at.Before(lastJump.Add(5*time.Minute)) {
		return
	}
	if old, ok := v.Number(fuel["tankTons"]); ok && !v.Bool(fuel["needsRefresh"]) && !v.Bool(fuel["tankEstimated"]) &&
		old != tank && at.Before(lastFuel.Add(5*time.Minute)) {
		return
	}
	setTank(fuel, tank, v.ISO(at), false)
}
