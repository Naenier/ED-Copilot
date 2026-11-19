package carrier

import (
	"testing"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCarrierTimingRestoresScheduleAndWaitsForTrustedArrival(t *testing.T) {
	start := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	departure := start.Add(45 * time.Minute)
	owner := M{"commander": M{"fid": "Synthetic owner"}, "planWorkspace": "live-workspace"}
	location := ce("CarrierLocation", M{"StarSystem": "Origin", "SystemAddress": "100", "BodyID": 1, "timestamp": v.ISO(start)})
	request := ce("CarrierJumpRequest", M{"SystemName": "Target", "SystemAddress": "200", "BodyID": 2,
		"DepartureTime": v.ISO(departure), "timestamp": v.ISO(start.Add(time.Second))})

	tr := New(nil)
	for _, event := range []M{location, request} {
		tr.Apply(event, owner)
	}
	if timing := tr.JumpTiming(departure.Add(-time.Second)); timing.Phase != "countdown" || !timing.Departure.Equal(departure) {
		t.Fatalf("scheduled deadline not reconstructed from the journal: %+v", timing)
	}
	if timing := tr.JumpTiming(departure.Add(2 * time.Minute)); timing.Phase != "departure-passed" || !timing.Arrival.IsZero() {
		t.Fatalf("passing a deadline proved arrival: %+v", timing)
	}
	tr.Apply(ce("CarrierJump", M{"CarrierID": nil, "MarketID": "43", "StarSystem": "Target", "SystemAddress": "200", "timestamp": v.ISO(departure)}), owner)
	if timing := tr.JumpTiming(departure); timing.Phase != "departure-passed" {
		t.Fatalf("another carrier's jump changed the owner's timer: %+v", timing)
	}

	// The real journal can report the destination before the jump sequence ends.
	for _, delay := range []time.Duration{0, 50 * time.Second} {
		at := departure.Add(delay)
		tr.Apply(ce("CarrierLocation", M{"StarSystem": "TARGET", "SystemAddress": "200", "BodyID": 2, "timestamp": v.ISO(at)}), owner)
		if timing := tr.JumpTiming(at); timing.Phase != "departure-passed" || !timing.Arrival.IsZero() {
			t.Fatalf("destination location was taken as an arrival: %+v", timing)
		}
	}
	if timing := tr.JumpTiming(departure.Add(time.Minute)); timing.Phase != "estimated-cooldown" || !timing.EstimatedCooldownUntil.Equal(departure.Add(6*time.Minute)) {
		t.Fatalf("missing bounded estimate while away from the carrier: %+v", timing)
	}

	arrival := departure.Add(63 * time.Second)
	for _, at := range []time.Time{arrival, arrival.Add(time.Second), arrival.Add(7 * time.Minute)} {
		tr.Apply(ce("CarrierJump", M{"CarrierID": nil, "MarketID": "42", "StarSystem": "Target", "SystemAddress": "200", "BodyID": 2, "timestamp": v.ISO(at)}), owner)
		timing := tr.JumpTiming(at)
		if !timing.Arrival.Equal(arrival) || !timing.CooldownUntil.Equal(arrival.Add(FleetCarrierCooldown)) || !timing.EstimatedCooldownUntil.IsZero() || timing.ArrivalSystem != "Target" || !timing.ConfirmedRequestAt.Equal(start.Add(time.Second)) {
			t.Fatalf("arrival failed to replace the estimate exactly once: %+v", timing)
		}
	}
	if timing := tr.JumpTiming(arrival.Add(FleetCarrierCooldown)); timing.Phase != "idle" {
		t.Fatalf("expired recovery remained active: %+v", timing)
	}
}

func TestCarrierTimingCancellationAndProfileChange(t *testing.T) {
	tr := New(nil)
	start := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	owner := M{"commander": M{"fid": "Synthetic owner"}, "planWorkspace": "live-workspace"}
	request := ce("CarrierJumpRequest", M{"SystemName": "Target", "timestamp": v.ISO(start)})
	tr.Apply(request, owner)
	if timing := tr.JumpTiming(start); timing.Phase != "scheduled" || !timing.Departure.IsZero() {
		t.Fatalf("missing time produced a guessed departure: %+v", timing)
	}
	tr.Apply(ce("CarrierJumpCancelled", M{"timestamp": v.ISO(start.Add(-time.Second))}), owner)
	if timing := tr.JumpTiming(start); timing.Phase != "scheduled" {
		t.Fatalf("stale cancellation cleared a new request: %+v", timing)
	}
	tr.Apply(ce("CarrierJumpCancelled", M{"timestamp": v.ISO(start.Add(time.Second))}), owner)
	tr.Apply(request, owner)
	if timing := tr.JumpTiming(start); timing.Phase != "idle" || !timing.RequestedAt.IsZero() {
		t.Fatalf("duplicate request rearmed a cancelled jump: %+v", timing)
	}
	tr.Apply(v.Merge(request, M{"timestamp": v.ISO(start.Add(2 * time.Second))}), owner)
	tr.Apply(ce("Commander", nil), M{"commander": M{"fid": "Second commander"}, "planWorkspace": "other-live-workspace"})
	if timing := tr.JumpTiming(start); timing.CarrierID != "" || timing.Phase != "idle" {
		t.Fatalf("next commander inherited the carrier timing: %+v", timing)
	}
}

func TestCarrierLocationDoesNotConfirmUnchangedOrbit(t *testing.T) {
	tr := New(nil)
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	departure := now.Add(time.Minute)
	tr.Apply(ce("CarrierLocation", M{"StarSystem": "Origin", "SystemAddress": "100", "BodyID": 1, "timestamp": v.ISO(now)}), nil)
	tr.Apply(ce("CarrierJumpRequest", M{"SystemName": "Origin", "SystemAddress": "100", "BodyID": 2,
		"DepartureTime": v.ISO(departure), "timestamp": v.ISO(now.Add(time.Second))}), nil)
	for _, body := range []any{nil, 1} {
		tr.Apply(ce("CarrierLocation", M{"StarSystem": "Origin", "SystemAddress": "100", "BodyID": body, "timestamp": v.ISO(departure)}), nil)
		if timing := tr.JumpTiming(departure); !timing.EstimatedArrival.IsZero() || !timing.Arrival.IsZero() {
			t.Fatalf("old orbit classified as a jump: %+v", timing)
		}
	}
	tr.Apply(ce("CarrierLocation", M{"StarSystem": "Origin", "SystemAddress": "100", "BodyID": 2, "timestamp": v.ISO(departure)}), nil)
	if timing := tr.JumpTiming(departure); timing.EstimatedArrival.IsZero() || !timing.Arrival.IsZero() {
		t.Fatalf("new orbit did not produce a transit estimate: %+v", timing)
	}
}
