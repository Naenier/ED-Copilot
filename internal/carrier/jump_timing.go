package carrier

import (
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// FleetCarrierCooldown is the normal in-game FSD recovery period. The
// Journal records jump arrival but does not report when the FSD is ready.
const FleetCarrierCooldown = 5 * time.Minute

// The destination CarrierLocation appears when the jump starts. Actual
// arrival normally follows about a minute later when the jump sequence ends.
const fleetCarrierTransitEstimate = time.Minute

// JumpTiming describes the owner's current jump request and last observed
// arrival. Reaching Departure means an arrival is still to be confirmed.
type JumpTiming struct {
	CarrierID, Destination, ArrivalSystem          string
	RequestedAt, Departure, Arrival, CooldownUntil time.Time
	ConfirmedRequestAt                             time.Time
	EstimatedArrival, EstimatedCooldownUntil       time.Time
	Phase                                          string
}

func (t *Tracker) JumpTiming(now time.Time) JumpTiming {
	return jumpTiming(t.Current(), now)
}

func jumpTiming(c M, now time.Time) JumpTiming {
	jump := v.Obj(c["jump"])
	pending := v.Obj(jump["pending"])
	transit := v.Obj(jump["transit"])
	confirmed := v.Obj(jump["confirmedArrival"])
	request := pending
	if len(request) == 0 {
		request = transit
	}
	result := JumpTiming{
		CarrierID:          v.Str(c["id"]),
		Destination:        v.Str(request["system"]),
		ArrivalSystem:      v.Str(confirmed["system"]),
		RequestedAt:        v.Time(request["requestedAt"]),
		Departure:          v.Time(request["departureTime"]),
		Arrival:            v.Time(confirmed["timestamp"]),
		ConfirmedRequestAt: v.Time(confirmed["requestAt"]),
		Phase:              "idle",
	}
	if !result.Arrival.IsZero() {
		result.CooldownUntil = result.Arrival.Add(FleetCarrierCooldown)
		if now.Before(result.CooldownUntil) {
			result.Phase = "cooldown"
		}
	}
	if len(transit) != 0 && !result.Departure.IsZero() {
		result.EstimatedArrival = result.Departure.Add(fleetCarrierTransitEstimate)
		result.EstimatedCooldownUntil = result.EstimatedArrival.Add(FleetCarrierCooldown)
		switch {
		case now.Before(result.EstimatedArrival):
			result.Phase = "departure-passed"
		case now.Before(result.EstimatedCooldownUntil):
			result.Phase = "estimated-cooldown"
		default:
			result.Phase = "idle"
		}
	}
	if len(pending) != 0 {
		switch {
		case result.Departure.IsZero():
			result.Phase = "scheduled"
		case now.Before(result.Departure):
			result.Phase = "countdown"
		default:
			result.Phase = "departure-passed"
		}
	}
	return result
}

// JumpSnapshot is small enough to include in any overlay activity, even when
// the carrier warehouse and route are not needed.
func (t *Tracker) JumpSnapshot(now time.Time) M {
	c := t.Current()
	jump := v.Obj(c["jump"])
	timing := jumpTiming(c, now)
	var pending, lastArrival, cooldownMs, cooldown, seconds, estimatedArrival, estimatedCooldown any
	if p := v.Obj(jump["pending"]); len(p) != 0 {
		pending = v.Copy(p)
	}
	if !timing.Arrival.IsZero() {
		lastArrival = M{"timestamp": v.ISO(timing.Arrival), "system": timing.ArrivalSystem, "bodyId": v.At(jump, "confirmedArrival", "bodyId")}
		cooldown = v.ISO(timing.CooldownUntil)
		cooldownMs = timing.CooldownUntil.UnixMilli()
	}
	if pending != nil && !timing.Departure.IsZero() {
		seconds = max(0, timing.Departure.Sub(now).Seconds())
	}
	if !timing.EstimatedArrival.IsZero() {
		estimatedArrival, estimatedCooldown = v.ISO(timing.EstimatedArrival), v.ISO(timing.EstimatedCooldownUntil)
	}
	return M{"pending": pending, "destination": timing.Destination, "lastArrival": lastArrival, "cooldownUntil": cooldown, "cooldownUntilMs": cooldownMs,
		"estimatedArrival": estimatedArrival, "estimatedCooldownUntil": estimatedCooldown, "cooldownEstimated": cooldown != nil || estimatedCooldown != nil,
		"phase": timing.Phase, "secondsToDeparture": seconds, "arrivalConfirmed": pending == nil && estimatedArrival == nil && lastArrival != nil}
}
