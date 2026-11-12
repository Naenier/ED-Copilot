package mining

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func miningRunID(event, context M, sequence int) string {
	identity := v.Str(context["file"]) + "\x00" + v.Str(event["timestamp"]) + "\x00" + strconv.Itoa(sequence)
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:16])
}

func miningDuration(from, to any) any {
	start, end := v.Time(from), v.Time(to)
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return nil
	}
	return end.Sub(start).Seconds()
}

func (s *Service) finishTrip(boundary, timestamp string) {
	switch boundary {
	case "Docked":
		if s.tripStartedAt != "" {
			for _, run := range s.History {
				if run["tripStartedAt"] == s.tripStartedAt {
					run["tripEndedAt"] = timestamp
				}
			}
		}
		s.tripStartedAt = ""
	case "Died", "Shutdown", "LoadGame":
		s.tripStartedAt = ""
	}
}

func changedMiningContext(run, commander M) bool {
	for _, field := range []string{"system", "body", "ship", "vehicle"} {
		previous, now := v.Str(run[field]), v.Str(commander[field])
		if previous != "" && previous != "unknown" && now != "" && now != "unknown" && previous != now {
			return true
		}
	}
	return false
}

func (s *Service) recordMiningMethod(run, commander M, event string, raw bool) {
	for _, field := range []string{"system", "body", "ship", "vehicle"} {
		if run[field] == "" || run[field] == "unknown" {
			run[field] = v.Text(commander[field], run[field])
		}
	}
	switch {
	case event == "AsteroidCracked":
		run["method"], run["methodEvidence"] = "core", "journal"
	case event == "MiningRefined" && trading.Vessel(commander["vehicle"]) == "srv":
		run["method"], run["methodEvidence"] = "surface-planet", "journal"
		run["site"] = v.Text(s.Surface["site"], run["site"])
	case raw && run["method"] == nil && v.Num(run["prospectScans"]) == 0:
		run["method"], run["methodEvidence"] = "raw", "journal"
	case (event == "ProspectedAsteroid" || event == "MiningRefined") && (run["method"] == nil || run["method"] == "raw"):
		delete(run, "method")
		delete(run, "methodEvidence")
		if trading.Vessel(commander["vehicle"]) == "ship" && v.Bool(s.Loadout["known"]) {
			if method := onlyEquippedMiningMethod(s.Loadout); method != "" {
				run["method"], run["methodEvidence"] = method, "loadout"
			}
		}
	}
}

func onlyEquippedMiningMethod(loadout M) string {
	method := ""
	for _, module := range v.Rows(loadout["modules"]) {
		for _, tag := range v.Arr(module["tags"]) {
			candidate := v.Str(tag)
			if candidate == "abrasion" {
				candidate = "surface"
			}
			switch candidate {
			case "laser", "core", "subsurface", "surface":
				if method != "" && candidate != method {
					return ""
				}
				method = candidate
			}
		}
	}
	return method
}

// RefinedRunID identifies an event already accepted by Apply, so the trade
// ledger can carry its provenance through the ship and carrier inventories.
func (s *Service) RefinedRunID(event, context M) string {
	if event["event"] != "MiningRefined" || context["countInPeriod"] == false || v.Time(event["timestamp"]).IsZero() ||
		s.Current["lastMiningAt"] != event["timestamp"] {
		return ""
	}
	return v.Str(s.Current["id"])
}

func (s *Service) RunIDs() map[string]bool {
	ids := make(map[string]bool, len(s.History)+1)
	if id := v.Str(s.Current["id"]); id != "" {
		ids[id] = true
	}
	for _, run := range s.History {
		ids[v.Str(run["id"])] = true
	}
	return ids
}

// ApplyTradeResult handles transactions after the ledger has verified their
// movement and, where possible, associated external sales with mining runs.
func (s *Service) ApplyTradeResult(result, context M) {
	if revenue := v.Num(result["minedRevenue"]); revenue > 0 && context["countInPeriod"] != false {
		increment(s.StatsTotal, "miningRevenue", revenue)
	}
	attribution, ok := result["miningAttribution"].(trading.MiningTradeResult)
	if !ok {
		return
	}
	runs := make(map[string]M, len(s.History)+1)
	if s.Current != nil {
		runs[v.Str(s.Current["id"])] = s.Current
	}
	for _, run := range s.History {
		runs[v.Str(run["id"])] = run
	}
	for id := range attribution.AmbiguousRuns {
		if run := runs[id]; run != nil {
			run["originUncertain"] = true
		}
	}
	for _, sale := range attribution.Sales {
		if run := runs[sale.RunID]; run != nil {
			increment(run, "soldTons", sale.Tons)
			if sale.Priced {
				increment(run, "confirmedRevenue", sale.Credits)
			} else {
				increment(run, "unpricedSoldTons", sale.Tons)
			}
		}
	}
}

func (s *Service) enrichComparison(current M, history []M, stock map[string]map[string]trading.MiningStock, resources []M) {
	prices := make(map[string]float64, len(resources))
	for _, resource := range resources {
		if price := v.Num(resource["price"]); price > 0 {
			prices[v.Str(resource["key"])] = price
		}
	}
	if price := v.Num(s.Options["sellPrice"]); price > 0 {
		prices[v.Str(s.Options["resourceKey"])] = price
	}
	for _, run := range append([]M{current}, history...) {
		if run == nil {
			continue
		}
		var ship, carrier, priced, value float64
		for key, item := range stock[v.Str(run["id"])] {
			ship += item.ShipTons
			carrier += item.CarrierTons
			tons := item.ShipTons + item.CarrierTons
			if price := prices[key]; price > 0 {
				priced += tons
				value += tons * price
			}
		}
		run["shipRemainingTons"], run["carrierRemainingTons"] = ship, carrier
		run["remainingTons"] = ship + carrier
		if priced > 0 {
			run["estimatedRemainingValue"] = value
			run["pricedRemainingTons"] = priced
		}
		run["activeSeconds"] = miningDuration(run["startedAt"], run["lastMiningAt"])
		run["tripSeconds"] = miningDuration(run["tripStartedAt"], run["tripEndedAt"])
		for _, pair := range [][2]string{{"activeSeconds", "activeTonsPerHour"}, {"tripSeconds", "tripTonsPerHour"}} {
			if seconds := v.Num(run[pair[0]]); seconds >= time.Minute.Seconds() {
				run[pair[1]] = v.Num(run["refinedTons"]) * 3600 / seconds
			}
		}
	}
}
