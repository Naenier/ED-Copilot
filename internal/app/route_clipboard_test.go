package app

import (
	"github.com/Naenier/ED-Copilot/internal/carrier"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"testing"
)

func clipboardRoute() M {
	return M{"route": M{"createdAt": "2026-11-07T11:00:00Z", "activeIndex": 1, "stops": []M{{"name": "A", "id64": "1", "visited": true}, {"name": "B", "id64": "2"}, {"name": "C", "id64": "3"}}}, "summary": M{"totalJumps": 2, "completedJumps": 0}, "progress": M{"completedJumps": 0}}
}

func TestRouteClipboardOnlyLiveOwnArrivalAndLatestRequest(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	defer e.Close()
	e.Route = carrier.NewRoutePlanner(clipboardRoute())
	e.Apply(testEvent("CarrierStats", M{"CarrierID": "42", "Name": "Test carrier"}), M{"replay": true})
	e.Apply(testEvent("CarrierJump", M{"MarketID": "99", "StarSystem": "B", "SystemAddress": "2"}), M{})
	if e.PendingClipboard() != nil {
		t.Fatal("foreign carrier changed clipboard")
	}
	e.Apply(testEvent("FSDJump", M{"StarSystem": "B", "SystemAddress": "2"}), M{})
	if e.PendingClipboard() != nil {
		t.Fatal("ship jump changed clipboard")
	}
	e.Apply(testEvent("CarrierJump", M{"MarketID": "42", "StarSystem": "B", "SystemAddress": "2"}), M{})
	command := e.PendingClipboard()
	if v.Str(command["text"]) != "C" || v.Str(command["id"]) == "" {
		t.Fatal(command)
	}
	command["text"] = "tampered"
	if e.PendingClipboard()["text"] != "C" {
		t.Fatal("escaped mutable request")
	}
	e.AcknowledgeClipboard("stale")
	if e.PendingClipboard() == nil {
		t.Fatal("stale ack removed current request")
	}
	e.Apply(testEvent("CarrierJump", M{"MarketID": "42", "StarSystem": "B", "SystemAddress": "2"}), M{})
	if e.PendingClipboard()["id"] != command["id"] {
		t.Fatal("duplicate arrival queued again")
	}
	e.Apply(testEvent("CarrierJump", M{"MarketID": "42", "StarSystem": "C", "SystemAddress": "3"}), M{})
	if e.PendingClipboard() != nil {
		t.Fatal("finished route left a stale request")
	}
}

func TestRouteClipboardReplayAndOptOutDoNotWrite(t *testing.T) {
	for _, mode := range []string{"replay", "side", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			e, _, _ := fixtureEngine(t, nil)
			defer e.Close()
			e.Route = carrier.NewRoutePlanner(clipboardRoute())
			e.Apply(testEvent("CarrierStats", M{"CarrierID": "42"}), M{"replay": true})
			meta := M{}
			if mode == "replay" {
				meta["replay"] = true
			}
			if mode == "side" {
				meta["side"] = true
			}
			if mode == "disabled" {
				if _, err := e.Do("/api/settings", M{"routeAutoCopyNext": false}); err != nil {
					t.Fatal(err)
				}
			}
			e.Apply(testEvent("CarrierJump", M{"MarketID": "42", "StarSystem": "B", "SystemAddress": "2"}), meta)
			if e.PendingClipboard() != nil {
				t.Fatal("non-live or disabled write", e.PendingClipboard())
			}
		})
	}
}

func TestRouteClipboardAcknowledgmentAndDisabling(t *testing.T) {
	e, _, _ := fixtureEngine(t, nil)
	defer e.Close()
	e.Route = carrier.NewRoutePlanner(clipboardRoute())
	e.Apply(testEvent("CarrierStats", M{"CarrierID": "42"}), M{"replay": true})
	e.Apply(testEvent("CarrierLocation", M{"CarrierID": "42", "StarSystem": "B", "SystemAddress": "2"}), M{})
	request := e.PendingClipboard()
	if request == nil {
		t.Fatal("arrival not queued")
	}
	e.AcknowledgeClipboard(v.Str(request["id"]))
	if e.PendingClipboard() != nil {
		t.Fatal("ack not cleared")
	}
	e.Route = carrier.NewRoutePlanner(clipboardRoute())
	e.Apply(testEvent("CarrierJump", M{"MarketID": "42", "StarSystem": "B", "SystemAddress": "2"}), M{})
	if _, err := e.Do("/api/settings", M{"routeAutoCopyNext": false}); err != nil {
		t.Fatal(err)
	}
	if e.PendingClipboard() != nil {
		t.Fatal("opt-out did not cancel pending write")
	}
}
