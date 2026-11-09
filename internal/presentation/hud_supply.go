package presentation

import (
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const hudCarrierLocationMaxAge = 15 * time.Minute

type hudColonyWarehouse struct {
	label string
	items map[string]float64
	local bool
}

func hudCarrierInColony(warehouse, carrier, colony M, now time.Time) bool {
	location := v.Obj(warehouse["location"])
	fresh := func(at time.Time) bool {
		age := now.Sub(at)
		return !at.IsZero() && age >= 0 && age <= hudCarrierLocationMaxAge
	}
	observedAt := v.Time(location["updatedAt"])
	unknownDeparture := v.Bool(location["jumpPlanned"])
	trusted := fresh(observedAt) && !unknownDeparture
	if warehouse["kind"] == "fleet" && hudEqual(warehouse["id"], carrier["id"]) {
		pending := v.Obj(v.At(carrier, "jump", "pending"))
		journal := v.Obj(carrier["location"])
		at := v.Time(journal["updatedAt"])
		// An elapsed departure can make the old system wrong even when stock
		// is fresh. A pending destination is never an arrival observation.
		if len(pending) > 0 {
			departure := v.Time(pending["departureTime"])
			if departure.IsZero() || !now.Before(departure) {
				return false
			}
			// The owner's journal confirms the departure is still ahead.
			trusted = fresh(observedAt)
			unknownDeparture = false
		}
		if fresh(at) && v.Str(journal["system"]) != "" &&
			(!unknownDeparture || at.After(observedAt)) &&
			(!trusted || !at.Before(observedAt) || hudEqual(journal["system"], location["system"])) {
			location, trusted = journal, true
		}
	}
	if !trusted {
		return false
	}
	if address, target := v.Str(location["systemAddress"]), v.Str(colony["systemAddress"]); address != "" && target != "" {
		return address == target
	}
	return hudEqual(location["system"], colony["targetSystem"])
}

func hudColonyWarehouses(colony, project, carrier M, now time.Time) []hudColonyWarehouse {
	if !v.Bool(colony["includeCarrierWarehouse"]) {
		return nil
	}
	var result []hudColonyWarehouse
	seen := map[string]bool{}
	for _, warehouse := range v.Rows(colony["carrierWarehouses"]) {
		id := v.Str(warehouse["id"])
		if id == "" || seen[id] || !v.Bool(warehouse["accessible"]) || !v.Bool(warehouse["known"]) || warehouse["included"] == false {
			continue
		}
		seen[id] = true
		label := v.Str(warehouse["callsign"])
		if label == "" {
			label = "Личный флотоносец"
			if warehouse["kind"] == "squadron" {
				label = "Флотоносец эскадрильи"
			}
		}
		stock := hudColonyWarehouse{label: label, items: map[string]float64{}, local: hudCarrierInColony(warehouse, carrier, project, now)}
		for _, item := range v.Rows(warehouse["items"]) {
			key := v.Key(v.First(item["key"], item["commodity"]))
			if key != "" && hudKnown(item["count"]) {
				stock.items[key] += v.Num(item["count"])
			}
		}
		result = append(result, stock)
	}
	return result
}

type hudColonyAction struct {
	kind, title, subtitle string
	rows                  []HUDLine
}

func hudColonyAmount(name string, amount float64, bound, estimated bool) HUDLine {
	line := HUDLine{Text: name, Role: "text", Value: Number(amount) + " т"}
	if bound {
		line.Value = "≤" + line.Value
	}
	if estimated {
		line.Text += " (оценка)"
		line.Role = "warn"
	}
	return line
}

func hudColonyActions(colony, project, carrier M, now time.Time) ([]hudColonyAction, []HUDLine) {
	warehouses := hudColonyWarehouses(colony, project, carrier, now)
	delivery := hudColonyAction{kind: "delivery", title: "ДОСТАВИТЬ"}
	purchase := hudColonyAction{kind: "purchase", title: "КУПИТЬ"}
	unloads := make([]hudColonyAction, len(warehouses))
	for i, warehouse := range warehouses {
		unloads[i] = hudColonyAction{kind: "unload", title: "РАЗГРУЗИТЬ", subtitle: warehouse.label}
	}
	unknownHold, unknownStock, otherStockBound := false, false, false
	otherStock := float64(0)
	for _, row := range v.Rows(project["operationalSupplies"]) {
		missing := v.Num(row["missing"])
		name := v.Text(row["name"], row["commodity"])
		if missing <= 0 || name == "" {
			continue
		}
		holdKnown := hudKnown(row["cargoAvailable"])
		unknownHold = unknownHold || !holdKnown
		estimate := !v.Bool(row["authoritative"]) && strings.Contains(v.Str(row["source"]), "manual")
		onboard := float64(0)
		if holdKnown {
			onboard = min(v.Num(row["cargoAvailable"]), missing)
		}
		if onboard > 0 {
			delivery.rows = append(delivery.rows, hudColonyAmount(name, onboard, false, estimate))
		}
		left := missing - onboard
		key := v.Key(row["commodity"])
		for i, warehouse := range warehouses {
			if amount := min(warehouse.items[key], left); warehouse.local && amount > 0 {
				unloads[i].rows = append(unloads[i].rows, hudColonyAmount(name, amount, !holdKnown, estimate))
				left -= amount
			}
		}
		if left <= 0 {
			continue
		}
		if v.Bool(colony["includeCarrierWarehouse"]) {
			knownOther := float64(0)
			for _, warehouse := range warehouses {
				if !warehouse.local {
					knownOther += warehouse.items[key]
				}
			}
			// Older or scoped projections may only have aggregate stock.
			// It can reduce procurement, but does not identify a local carrier.
			if len(warehouses) == 0 && v.Bool(row["carrierIncluded"]) && hudKnown(row["carrierAvailable"]) {
				knownOther = v.Num(row["carrierAvailable"])
			}
			reserved := min(left, knownOther)
			otherStock += reserved
			otherStockBound = otherStockBound || reserved > 0 && !holdKnown
			left -= reserved
		}
		if left > 0 {
			stockIncomplete := v.Bool(colony["includeCarrierWarehouse"]) && !v.Bool(row["carrierComplete"])
			unknownStock = unknownStock || stockIncomplete
			purchase.rows = append(purchase.rows, hudColonyAmount(name, left, !holdKnown || stockIncomplete, estimate))
		}
	}
	actions := append([]hudColonyAction{delivery}, unloads...)
	actions = append(actions, purchase)
	var notes []HUDLine
	if unknownHold {
		notes = append(notes, hudWarn("Проверьте груз корабля"))
	}
	if otherStock > 0 {
		notes = append(notes, hudColonyAmount("Забрать с флотоносца", otherStock, otherStockBound, false))
	}
	if unknownStock {
		notes = append(notes, hudMuted("Сверьте запас на флотоносце"))
	}
	return actions, notes
}

// Give every nonempty group a preview and keep the total to ten commodity
// rows. Remaining positions stay in their own card's count.
func hudAddColonyActions(doc *HUDDocument, actions []hudColonyAction, subtitle string) {
	const limit = 10
	counts := make([]int, len(actions))
	for remaining := limit; remaining > 0; {
		added := false
		for i, action := range actions {
			maxRows := 5
			if action.kind == "purchase" {
				maxRows = 7
			}
			if counts[i] < min(len(action.rows), maxRows) {
				counts[i]++
				remaining--
				added = true
				if remaining == 0 {
					break
				}
			}
		}
		if !added {
			break
		}
	}
	for i, action := range actions {
		if counts[i] == 0 {
			continue
		}
		if subtitle != "" {
			if action.subtitle == "" {
				action.subtitle = subtitle
			} else {
				action.subtitle = subtitle + " · " + action.subtitle
			}
			subtitle = ""
		}
		hudCard(doc, action.kind, action.title, action.subtitle, len(action.rows)-counts[i], action.rows[:counts[i]]...)
	}
}
