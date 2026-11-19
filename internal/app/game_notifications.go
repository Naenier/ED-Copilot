package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	"github.com/Naenier/ED-Copilot/internal/exobiology"
	"github.com/Naenier/ED-Copilot/internal/notifications"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type gameNotificationChanges struct {
	live                               bool
	carrier                            carrier.JumpTiming
	engineerStatus, constructionStatus string
	constructionObservedAt             time.Time
	riches                             *notifications.Event
}

func (e *Engine) TakeNotificationSound() int {
	return e.section7.notifications.TakeSound()
}

func freshNotification(at, now time.Time) bool {
	return !at.IsZero() && now.Sub(at) <= 2*time.Minute && at.Sub(now) <= time.Minute
}

func (e *Engine) beforeGameNotifications(owner string, event, meta M) gameNotificationChanges {
	out := gameNotificationChanges{}
	if owner == "" || owner != e.planWorkspace.key || v.Bool(meta["replay"]) || e.journalStarting || e.planReplay ||
		!freshNotification(v.Time(event["timestamp"]), time.Now()) {
		return out
	}
	kind := v.Str(event["event"])
	if v.Bool(meta["side"]) && kind != "Status" {
		return out
	}
	out.live = true
	switch kind {
	case "CarrierJumpRequest", "CarrierJumpCancelled", "CarrierJump", "CarrierLocation":
		out.carrier = e.Carrier.JumpTiming(time.Now())
	case "EngineerProgress":
		if event["Engineers"] == nil && oneOf(v.Str(event["Progress"]), "Invited", "Unlocked") {
			_, _, _, out.engineerStatus = e.Engineering.EngineerProgressNotice(event)
		}
	case "ColonisationConstructionDepot":
		if event["ResourcesRequired"] != nil && (v.Bool(event["ConstructionComplete"]) != v.Bool(event["ConstructionFailed"])) {
			_, out.constructionStatus, out.constructionObservedAt = e.Colonies.ConstructionNoticeState(event["MarketID"])
		}
	}
	return out
}

func (e *Engine) observeGameNotifications(owner string, event, meta M, changes gameNotificationChanges) {
	s := &e.section7
	kind, at := v.Str(event["event"]), v.Time(event["timestamp"])
	notice := notifications.Event{Scope: owner, System: v.Str(v.At(meta, "commander", "system")), ObservedAt: at}
	if changes.riches != nil {
		s.notifications.Emit(*changes.riches)
	}
	switch kind {
	case "Status", "FuelScoop", "FSDJump", "ReservoirReplenished", "RefuelAll", "RefuelPartial":
		if !oneOf(v.Str(v.At(meta, "commander", "vehicle")), "srv", "foot") &&
			!v.Bool(event["Taxi"]) && !v.Bool(event["Multicrew"]) &&
			!v.Bool(v.At(meta, "game", "multicrew")) {
			e.shipFuelNotice(notice, !v.Bool(v.At(meta, "commander", "docked")))
		}
	case "MissionCompleted", "MissionFailed":
		if id := v.Str(event["MissionID"]); id != "" && id != "0" {
			notice.ID, notice.Link, notice.Activity = id, "overview", "flight"
			notice.Detail = v.Text(event["LocalisedName"], event["Name_Localised"], event["NameLocalised"])
			notice.EnglishDetail = notice.Detail
			notice.Kind, notice.Title, notice.EnglishTitle = "task", "Миссия завершена", "Mission completed"
			if kind == "MissionFailed" {
				notice.Kind, notice.Title, notice.EnglishTitle = "missionFailed", "Миссия провалена", "Mission failed"
			}
			s.notifications.Emit(notice)
		}
	case "CarrierJumpRequest", "CarrierJumpCancelled", "CarrierJump", "CarrierLocation":
		e.carrierJournalNotice(notice, event, changes.carrier)
	case "EngineerProgress":
		e.engineerStatusNotice(notice, event, changes.engineerStatus)
	case "ColonisationConstructionDepot":
		e.constructionNotice(notice, event, changes.constructionStatus, changes.constructionObservedAt)
	case "ScanOrganic", "SellOrganicData":
		e.bioNotice(notice, event)
	}
	if oneOf(kind, "Materials", "MaterialCollected", "MaterialDiscarded", "MaterialTrade", "EngineerCraft", "TechnologyBroker", "Synthesis", "MissionCompleted") {
		notice.Activity = "engineering"
		notice.Detail, notice.EnglishDetail = "", ""
		e.engineeringReadyNotice(notice)
	}
}

func (e *Engine) shipFuelNotice(notice notifications.Event, flying bool) {
	s := &e.section7
	if !s.notifications.Enabled("fuel") {
		return
	}
	fuel := v.Obj(e.Pilot.Snapshot()["fuel"])
	current, known := v.Number(fuel["mainTons"])
	capacity := v.Num(fuel["mainCapacityTons"])
	if !known || capacity <= 0 {
		return
	}
	if current/capacity > .15 && s.lowFuel {
		s.lowFuel = false
		s.notifications.Dismiss("fuel")
	}
	if flying && current/capacity <= .10 && !s.lowFuel {
		s.lowFuel = true
		notice.ID, notice.Kind, notice.Link, notice.Activity = v.ISO(notice.ObservedAt), "fuel", "ship-route", "flight"
		notice.Title, notice.EnglishTitle = "Мало топлива", "Low fuel"
		notice.Detail = fmt.Sprintf("В основном баке %.1f т", current)
		notice.EnglishDetail = fmt.Sprintf("%.1f t in the main tank", current)
		s.notifications.Emit(notice)
	}
}

func (e *Engine) carrierJournalNotice(notice notifications.Event, event M, before carrier.JumpTiming) {
	s := &e.section7
	now := time.Now()
	after := e.Carrier.JumpTiming(now)
	if after.CarrierID == "" || after.CarrierID != v.Text(event["CarrierID"], event["MarketID"]) {
		return
	}
	notice.Link = "carrier"
	switch event["event"] {
	case "CarrierJumpRequest":
		if !after.RequestedAt.Equal(notice.ObservedAt) || after.RequestedAt.Equal(before.RequestedAt) {
			return
		}
		s.notifications.Dismiss("carrier")
		s.notifications.Dismiss("carrierScheduled")
		s.notifications.Dismiss("carrierCooldown")
		notice.ID, notice.Kind = after.CarrierID+"/"+v.ISO(after.RequestedAt), "carrierScheduled"
		notice.Title, notice.EnglishTitle = "Прыжок флотоносца назначен", "Fleet carrier jump scheduled"
		notice.System, notice.Detail, notice.EnglishDetail = after.Destination, after.Destination, after.Destination
		if !after.Departure.IsZero() {
			notice.Detail += " · " + after.Departure.Local().Format("15:04")
			notice.EnglishDetail = notice.Detail
		}
	case "CarrierJumpCancelled":
		if before.RequestedAt.IsZero() || !after.RequestedAt.IsZero() || notice.ObservedAt.Before(before.RequestedAt) {
			return
		}
		s.notifications.Dismiss("carrier")
		s.notifications.Dismiss("carrierScheduled")
		notice.ID, notice.Kind = before.CarrierID+"/"+v.ISO(before.RequestedAt), "carrierCancelled"
		notice.Title, notice.EnglishTitle = "Прыжок флотоносца отменён", "Fleet carrier jump cancelled"
		notice.System, notice.Detail, notice.EnglishDetail = before.Destination, before.Destination, before.Destination
	case "CarrierJump", "CarrierLocation":
		if !after.Arrival.Equal(notice.ObservedAt) || after.Arrival.Equal(before.Arrival) {
			return
		}
		s.notifications.Dismiss("carrier")
		s.notifications.Dismiss("carrierScheduled")
		notice.ID, notice.Kind = after.CarrierID+"/"+v.ISO(after.Arrival), "carrierArrived"
		notice.Title, notice.EnglishTitle = "Флотоносец прибыл", "Fleet carrier arrived"
		notice.System, notice.Detail, notice.EnglishDetail = after.ArrivalSystem, after.ArrivalSystem, after.ArrivalSystem
	}
	s.notifications.Emit(notice)
	e.tickCarrierNotifications(now)
}

func (e *Engine) tickCarrierNotifications(now time.Time) bool {
	s := &e.section7
	owner := s.observedOwner
	if owner == "" || owner != e.planWorkspace.key || e.planReplay || e.journalStarting {
		return false
	}
	timing := e.Carrier.JumpTiming(now)
	if timing.CarrierID == "" {
		return false
	}
	notice := notifications.Event{Scope: owner, ObservedAt: now, Link: "carrier"}
	emitted := false
	if timing.Phase == "countdown" && timing.Departure.After(now) && timing.Departure.Sub(now) <= 5*time.Minute {
		id := timing.CarrierID + "/" + v.ISO(timing.Departure)
		if s.carrierDepartureSeen != id {
			s.carrierDepartureSeen = id
			departure := notice
			departure.ID, departure.Kind, departure.System = id, "carrier", timing.Destination
			departure.Title, departure.EnglishTitle = "Скоро прыжок флотоносца", "Fleet carrier departure soon"
			departure.Detail = timing.Destination + " · " + timing.Departure.Local().Format("15:04")
			departure.EnglishDetail = departure.Detail
			emitted = s.notifications.Emit(departure)
		}
	}

	return e.carrierCooldownNotice(notice, timing, now) || emitted
}

func (e *Engine) carrierCooldownNotice(notice notifications.Event, timing carrier.JumpTiming, now time.Time) bool {
	s := &e.section7
	request, deadline, system := timing.ConfirmedRequestAt, timing.CooldownUntil, timing.ArrivalSystem
	if request.IsZero() {
		request = timing.Arrival
	}
	estimated := !timing.EstimatedCooldownUntil.IsZero()
	if estimated {
		request, deadline, system = timing.RequestedAt, timing.EstimatedCooldownUntil, timing.Destination
	} else if !timing.RequestedAt.IsZero() {
		// A new request supersedes recovery from the previous jump.
		s.armedCarrierCooldown = ""
		return false
	}
	if request.IsZero() || deadline.IsZero() {
		s.armedCarrierCooldown = ""
		return false
	}
	id := timing.CarrierID + "/" + v.ISO(request)
	if id == s.carrierCooldownSeen {
		return false
	}
	if now.Before(deadline) {
		s.armedCarrierCooldown = id
		return false
	}
	if s.armedCarrierCooldown != id {
		return false
	}
	s.armedCarrierCooldown = ""
	if now.Sub(deadline) > 20*time.Second {
		return false
	}
	s.carrierCooldownSeen = id
	notice.ID, notice.Kind, notice.System = id, "carrierCooldown", system
	notice.Title, notice.EnglishTitle = "Охлаждение флотоносца завершено", "Fleet carrier cooldown elapsed"
	notice.Detail, notice.EnglishDetail = "По расчёту от времени подтверждённого прибытия.", "Estimated from the confirmed arrival time."
	if estimated {
		notice.Title, notice.EnglishTitle = "Расчётное время охлаждения истекло", "Estimated carrier cooldown elapsed"
		notice.Detail, notice.EnglishDetail = "По времени начала прыжка. Ожидается подтверждение прибытия.", "Based on jump start. Awaiting arrival confirmation."
	}
	return s.notifications.Emit(notice)
}

func (e *Engine) notifyShipRoute(owner string, before, after M, at time.Time) {
	if after != nil && before["next"] == after["next"] {
		return
	}
	notice := notifications.Event{
		ID: v.ISO(at) + "/" + v.Str(before["next"]), Kind: "shipRouteStop", Scope: owner, Activity: "flight",
		Title: "Точка маршрута пройдена", EnglishTitle: "Route waypoint reached",
		System: v.Str(before["system"]), Link: "ship-route", ObservedAt: at,
	}
	notice.Detail = routeLocation(notice.System, v.Str(before["station"]))
	notice.EnglishDetail = notice.Detail
	if after == nil {
		notice.Kind, notice.Title, notice.EnglishTitle = "shipRouteComplete", "Маршрут корабля завершён", "Ship route complete"
	}
	e.section7.notifications.Emit(notice)
}

func routeLocation(system, station string) string {
	if system == "" || station == "" {
		return system + station
	}
	return system + " · " + station
}

func (e *Engine) notifyTradeRound(owner string, run, cmd M, at time.Time) {
	round, seconds := v.Int(run["completedRounds"]), v.Num(run["lastRoundSeconds"])
	if seconds <= 0 {
		return
	}
	elapsed := (time.Duration(seconds * float64(time.Second))).Round(time.Second)
	notice := notifications.Event{
		ID: v.Str(run["routeId"]) + "/" + strconv.Itoa(round) + "/" + v.ISO(at), Kind: "tradeRound",
		Scope: owner, Activity: "trade", System: v.Str(cmd["system"]), Link: "deals", ObservedAt: at,
		Title: "Торговый круг завершён", EnglishTitle: "Trade round complete",
		Detail: fmt.Sprintf("Круг %d · %s", round, elapsed), EnglishDetail: fmt.Sprintf("Round %d · %s", round, elapsed),
	}
	e.section7.notifications.Emit(notice)
}

func (e *Engine) engineerStatusNotice(notice notifications.Event, event M, before string) {
	progress := v.Str(event["Progress"])
	if event["Engineers"] != nil || !oneOf(progress, "Invited", "Unlocked") {
		return
	}
	key, name, english, after := e.Engineering.EngineerProgressNotice(event)
	// Unlocked can accompany routine rank updates. Require a known prior
	// status before reporting an unlock.
	if key == "" || before == after || after != progress || progress == "Unlocked" && before == "" {
		return
	}
	notice.ID, notice.Link, notice.Activity = key+"/"+progress, "engineering", "engineering"
	notice.Kind, notice.Title, notice.EnglishTitle = "engineerInvited", "Получено приглашение инженера", "Engineer invitation received"
	if progress == "Unlocked" {
		notice.Kind, notice.Title, notice.EnglishTitle = "engineerUnlocked", "Инженер открыт", "Engineer unlocked"
	}
	notice.Detail, notice.EnglishDetail = name, english
	e.section7.notifications.Emit(notice)
}

func (e *Engine) constructionNotice(notice notifications.Event, event M, before string, beforeAt time.Time) {
	complete, failed := v.Bool(event["ConstructionComplete"]), v.Bool(event["ConstructionFailed"])
	if complete == failed || event["ResourcesRequired"] == nil {
		return
	}
	name, after, observed := e.Colonies.ConstructionNoticeState(event["MarketID"])
	if before == after || !observed.Equal(notice.ObservedAt) || !observed.After(beforeAt) {
		return
	}
	notice.ID, notice.Link, notice.Activity = v.Str(event["MarketID"]), "colonization", "colonization"
	notice.Detail, notice.EnglishDetail = name, name
	switch {
	case complete && after == "complete":
		notice.Kind, notice.Title, notice.EnglishTitle = "colonyComplete", "Строительство завершено", "Construction complete"
	case failed && after == "failed":
		notice.Kind, notice.Title, notice.EnglishTitle = "colonyFailed", "Строительство не удалось", "Construction failed"
	default:
		return
	}
	e.section7.notifications.Emit(notice)
}

func (e *Engine) bioNotice(notice notifications.Event, event M) {
	s := e.section7
	notice.Link, notice.Activity = "exobiology", "exobiology"
	switch event["event"] {
	case "ScanOrganic":
		if !strings.EqualFold(v.Str(event["ScanType"]), "Analyse") || v.Str(event["SystemAddress"]) == "" || event["Body"] == nil {
			return
		}
		speciesKey := v.Text(event["Species"], event["Variant"])
		if speciesKey == "" {
			return
		}
		notice.ID = v.ISO(notice.ObservedAt) + "/" + v.Str(event["SystemAddress"]) + "/" + v.Str(event["Body"]) + "/" + speciesKey
		notice.Kind, notice.Title, notice.EnglishTitle = "bioComplete", "Биологический образец исследован", "Organic sample complete"
		notice.Detail = v.Text(event["Species_Localised"], event["Variant_Localised"])
		notice.EnglishDetail = notice.Detail
		if species := exobiology.Catalog().ResolveOrganic(event); species != nil {
			notice.Detail, notice.EnglishDetail = v.Text(notice.Detail, species.Name), species.EnglishName
		}
	case "SellOrganicData":
		rows := v.Rows(event["BioData"])
		if len(rows) == 0 {
			return
		}
		notice.ID, notice.Kind = v.ISO(notice.ObservedAt), "bioSold"
		notice.Title, notice.EnglishTitle = "Биологические данные проданы", "Organic data sold"
		notice.Detail = fmt.Sprintf("Записей в продаже: %d", len(rows))
		notice.EnglishDetail = fmt.Sprintf("Entries sold: %d", len(rows))
	}
	s.notifications.Emit(notice)
}
