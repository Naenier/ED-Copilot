package app

import (
	"github.com/Naenier/ED-Copilot/internal/theme"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// During replay the model is rebuilt event by event. The overview and window
// keep the last complete observation until the journal and side files agree.
type overviewObservations struct {
	commander, pilot, carrier, riches, colonies M
	workspace                                   planWorkspace
}

// SnapshotForWindow provides the state required to show the native window.
func (e *Engine) SnapshotForWindow() M {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	return e.snapshotWindowLocked(e.windowConnection(nil))
}

// SnapshotForNativeRefresh includes gameplay data only while the overlay
// can be visible. The desktop chrome and overlay selector stay up to date.
func (e *Engine) SnapshotForNativeRefresh() M {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	connection := e.windowConnection(nil)
	pending := e.journalObservationPending(connection)
	e.mu.RLock()
	showHUD := v.Bool(e.settings["overlayEnabled"]) && e.gameRunning != false && !pending
	e.mu.RUnlock()
	if showHUD {
		return e.snapshotScopedLocked("hud")
	}
	state := e.snapshotWindowLocked(connection)
	if v.At(state, "settings", "activity") == "colonization" {
		if pending {
			state["colonization"] = v.Copy(e.overviewBeforeReplay.colonies)
		} else {
			state["colonization"] = e.Colonies.SelectorSnapshot()
		}
	}
	return state
}

// SnapshotForOverview returns the cards on the landing page. The cargo,
// mission and journal views still use the regular overview projection.
func (e *Engine) SnapshotForOverview() M {
	e.domainMu.Lock()
	defer e.domainMu.Unlock()
	return e.snapshotOverviewLocked(e.windowConnection(nil), false)
}

// journalObservationPending must be evaluated within the domain lock.
// planReplay remains set if the reader stopped after a partially applied scan.
func (e *Engine) journalObservationPending(connection M) bool {
	return v.Bool(connection["scanning"]) || e.planReplay
}

func (e *Engine) snapshotWindowLocked(connection M) M {
	e.mu.RLock()
	defer e.mu.RUnlock()
	commander, running := e.commander, e.gameRunning
	workspace := e.planWorkspace
	pending := e.journalObservationPending(connection)
	if pending {
		commander, running = e.overviewBeforeReplay.commander, false
		workspace = e.overviewBeforeReplay.workspace
	}
	connection["gameRunning"] = running
	state := e.windowStateLocked()
	if !pending {
		state["shipBuildRevision"] = e.engineeringRoadmapRevision
	}
	state["commander"], state["connection"] = v.Copy(commander), connection
	state["planWorkspace"] = workspace.snapshot()
	return state
}

func (e *Engine) snapshotOverviewLocked(connection M, detailed bool) M {
	e.mu.RLock()
	state := e.windowStateLocked()
	var observed overviewObservations
	var running any = false
	if e.journalObservationPending(connection) {
		frozen := e.overviewBeforeReplay
		observed = overviewObservations{
			commander: v.Copy(frozen.commander), pilot: v.Copy(frozen.pilot),
			carrier: v.Copy(frozen.carrier), riches: v.Copy(frozen.riches),
			colonies: v.Copy(frozen.colonies), workspace: frozen.workspace,
		}
	} else {
		observed, running = e.overviewObservationsLocked(detailed), e.gameRunning
	}
	connection["gameRunning"] = running
	state["connection"] = connection
	state["navigation"] = M{"tabs": append([]string(nil), e.navigation...)}
	state["commander"], state["pilot"] = observed.commander, observed.pilot
	state["carrier"], state["riches"] = observed.carrier, observed.riches
	if v.At(state, "settings", "activity") == "colonization" {
		state["coloniesSelectors"] = observed.colonies
	}
	state["objectives"] = goalsForWorkspace(e.objectives, observed.workspace)
	state["planWorkspace"] = observed.workspace.snapshot()
	appearance := v.Copy(e.appearance)
	e.mu.RUnlock()
	state["updates"] = e.updateSnapshot()
	palette, _ := theme.Palette(appearance)
	state["theme"] = M{"palette": palette}
	return state
}

// overviewObservationsLocked is called with the domain and engine locks held.
func (e *Engine) overviewObservationsLocked(detailed bool) overviewObservations {
	o := overviewObservations{
		commander: v.Copy(e.commander),
		riches:    e.richesSnapshotLocked(false),
		workspace: e.planWorkspace,
	}
	o.commander["cargoTons"] = e.Trading.CargoTons(v.Str(o.commander["vehicle"]))
	if detailed {
		o.pilot = e.Pilot.Snapshot()
		o.commander["cargo"] = e.Trading.Snapshot(o.commander)["inventory"]
		o.commander["cargoUpdatedAt"] = e.cargoAt
	} else {
		o.pilot = e.Pilot.OverviewSnapshot()
	}
	if c := e.Carrier.Current(); c != nil {
		o.carrier = M{"owned": true, "name": c["name"], "callsign": c["callsign"], "location": M{"system": v.At(c, "location", "system")}}
	}
	if detailed || e.settings["activity"] == "colonization" {
		o.colonies = e.Colonies.SelectorSnapshot()
	}
	return o
}

func (e *Engine) windowStateLocked() M {
	return M{
		"revision": e.revision, "settings": v.Copy(e.settings), "desktop": v.Copy(e.desktop),
		"app": M{"name": "ED Copilot", "version": Version, "language": e.settings["language"], "profileDir": e.opts.DataDir, "runtime": "Go · native Windows"},
	}
}

func (e *Engine) windowConnection(running any) M {
	connection := M{}
	if e.Reader != nil {
		connection = e.Reader.State()
	}
	connection["gameRunning"] = running
	return connection
}
