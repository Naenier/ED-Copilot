package app

import (
	"slices"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/notifications"
	"github.com/Naenier/ED-Copilot/internal/riches"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func notificationKinds(e *Engine) []string {
	events := e.section7.notifications.Snapshot(e.planWorkspace.key)["events"].([]notifications.Event)
	kinds := make([]string, len(events))
	for i, event := range events {
		kinds[i] = event.Kind
	}
	return kinds
}

func TestCarrierNotificationsRequireConfirmedArrival(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	researchResult(t, e, "/api/notifications/update", M{"preferences": M{"enabled": true, "sound": true}})
	now := time.Now().UTC().Truncate(time.Second)
	departure := now.Add(-15 * time.Second)
	arrival := now.Add(time.Second)
	apply := func(kind string, at time.Time, fields M) {
		t.Helper()
		e.Apply(v.Merge(M{"event": kind, "timestamp": v.ISO(at), "CarrierID": "42"}, fields), nil)
	}
	apply("CarrierLocation", now.Add(-time.Minute), M{"StarSystem": "Origin", "SystemAddress": "10"})
	apply("CarrierJumpRequest", now.Add(-45*time.Second), M{"SystemName": "Destination", "SystemAddress": "20", "DepartureTime": v.ISO(departure)})
	if !e.tickCarrierNotifications(departure.Add(-time.Second)) || e.tickCarrierNotifications(departure.Add(-time.Second)) {
		t.Fatal("departure reminder did not emit just once")
	}
	apply("CarrierLocation", departure, M{"StarSystem": "Destination", "SystemAddress": "20"})
	apply("CarrierJump", arrival, M{"CarrierID": nil, "MarketID": "43", "StarSystem": "Destination", "SystemAddress": "20"})
	if e.tickCarrierNotifications(now.Add(2 * time.Minute)) {
		t.Fatal("location at departure and another carrier's jump must not confirm arrival")
	}
	if got := notificationKinds(e); !slices.Equal(got, []string{"carrier", "carrierScheduled"}) {
		t.Fatalf("unexpected notices before confirmed arrival: %v", got)
	}
	for _, at := range []time.Time{arrival, arrival.Add(time.Second)} {
		apply("CarrierJump", at, M{"CarrierID": nil, "MarketID": "42", "StarSystem": "Destination", "SystemAddress": "20"})
	}
	if got := e.TakeNotificationSound(); got != 2 {
		t.Fatalf("arrival was not independently audible: %d", got)
	}
	end := arrival.Add(carrier.FleetCarrierCooldown)
	if e.tickCarrierNotifications(end.Add(-time.Second)) || !e.tickCarrierNotifications(end) || e.tickCarrierNotifications(end.Add(time.Second)) {
		t.Fatal("cooldown did not emit exactly once at its deadline")
	}
	want := []string{"carrierCooldown", "carrierArrived", "carrier", "carrierScheduled"}
	if got := notificationKinds(e); !slices.Equal(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestMappingNotificationWaitsForAllBodiesAcrossActivities(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	researchResult(t, e, "/api/notifications/update", M{"preferences": M{"enabled": true, "sound": true}})
	now := time.Now().UTC().Add(-time.Second)
	e.richesJourneys[e.planWorkspace.key] = &richesJourney{
		CreatedAt: now.Add(-time.Minute),
		Route: riches.Route{Stops: []riches.Stop{{
			System: "Mapping target", Bodies: []riches.Body{{Name: "Mapping target 1"}, {Name: "Mapping target 2"}},
		}}},
	}
	apply := func(kind string, fields M, meta M) {
		t.Helper()
		e.Apply(v.Merge(M{"event": kind, "timestamp": v.ISO(now), "StarSystem": "Mapping target"}, fields), meta)
		now = now.Add(time.Millisecond)
	}
	apply("FSDJump", nil, nil)
	if got := e.TakeNotificationSound(); got != 1 {
		t.Fatalf("waypoint did not play across activities: %d", got)
	}
	doOK(t, e, "/api/settings", M{"activity": "cartography"})
	apply("Scan", M{"BodyName": "Mapping target 1"}, nil)
	apply("SAAScanComplete", M{"BodyName": "Mapping target 2"}, nil)
	if got := notificationKinds(e); !slices.Equal(got, []string{"cartographyStop"}) {
		t.Fatalf("mapping completed before the final surface scan: %v", got)
	}
	for range 2 {
		apply("SAAScanComplete", M{"BodyName": "Mapping target 1"}, nil)
	}
	if got := notificationKinds(e); !slices.Equal(got, []string{"cartographyComplete", "cartographyStop"}) || e.TakeNotificationSound() != 2 {
		t.Fatalf("cartography completion did not emit once with sound: %v", got)
	}
}

func TestEngineerNotificationsIgnoreBulkAndRankChanges(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	researchResult(t, e, "/api/notifications/update", M{"preferences": M{"enabled": true}})
	now := time.Now().UTC().Add(-time.Second)
	apply := func(fields, meta M) {
		t.Helper()
		e.Apply(v.Merge(M{"event": "EngineerProgress", "timestamp": v.ISO(now), "Engineer": "Felicity Farseer"}, fields), meta)
		now = now.Add(time.Millisecond)
	}
	apply(M{"Engineers": []M{{"Engineer": "Felicity Farseer", "Progress": "Known"}}}, nil)
	apply(M{"Progress": "Invited"}, nil)
	apply(M{"Progress": "Unlocked", "Rank": 1}, nil)
	apply(M{"Progress": "Unlocked", "Rank": 2}, nil)
	apply(M{"Engineer": "Elvira Martuuk", "Progress": "Invited"}, M{"replay": true})
	want := []string{"engineerUnlocked", "engineerInvited"}
	if got := notificationKinds(e); !slices.Equal(got, want) {
		t.Fatalf("only journal invitation and unlock transitions warrant notifications, got %v", got)
	}
}

func TestEstimatedCarrierCooldownDoesNotDuplicateConfirmedCooldown(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	researchResult(t, e, "/api/notifications/update", M{"preferences": M{"enabled": true}})
	now := time.Now().UTC().Truncate(time.Second)
	departure := now.Add(-6*time.Minute - time.Second)
	apply := func(kind string, at time.Time, fields M) {
		t.Helper()
		e.Apply(v.Merge(M{"event": kind, "timestamp": v.ISO(at), "CarrierID": "42", "SystemAddress": "20"}, fields), nil)
	}
	apply("CarrierJumpRequest", departure.Add(-15*time.Minute), M{"SystemName": "Destination", "DepartureTime": v.ISO(departure)})
	apply("CarrierLocation", departure, M{"StarSystem": "Destination"})
	estimated := e.Carrier.JumpTiming(now).EstimatedCooldownUntil
	if estimated.IsZero() {
		t.Fatal("trusted departure location did not produce an estimated deadline")
	}
	// Opening after the timer expired must not generate a stale alert.
	if e.tickCarrierNotifications(now) {
		t.Fatal("an unobserved expired estimate emitted on startup")
	}
	e.tickCarrierNotifications(estimated.Add(-time.Second))
	if !e.tickCarrierNotifications(estimated) {
		t.Fatal("the active estimated timer did not notify at expiry")
	}
	events := e.section7.notifications.Snapshot(e.planWorkspace.key)["events"].([]notifications.Event)
	if len(events) != 1 || events[0].Kind != "carrierCooldown" || events[0].EnglishTitle != "Estimated carrier cooldown elapsed" {
		t.Fatalf("provisional recovery was misreported: %+v", events)
	}
	apply("CarrierJump", now, M{"CarrierID": nil, "MarketID": "42", "StarSystem": "Destination"})
	end := now.Add(carrier.FleetCarrierCooldown)
	e.tickCarrierNotifications(end.Add(-time.Second))
	if e.tickCarrierNotifications(end) {
		t.Fatal("arrival confirmation caused a second cooldown notification for the same jump")
	}
	if got := notificationKinds(e); !slices.Equal(got, []string{"carrierArrived", "carrierCooldown"}) {
		t.Fatalf("want one arrival and one cooldown notice, got %v", got)
	}
}
