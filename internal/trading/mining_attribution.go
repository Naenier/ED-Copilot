package trading

// MiningSale attributes a journal-confirmed market sale to a run only when
// the consumed commodity can be traced unambiguously to that run.
type MiningSale struct {
	RunID         string
	Tons, Credits float64
	Priced        bool
}

type MiningTradeResult struct {
	Sales         []MiningSale
	AmbiguousRuns map[string]bool
}

type MiningStock struct {
	ShipTons, CarrierTons float64
}

func takeWithMiningProvenance(st store, key string, count float64, result *MiningTradeResult) []lot {
	lots := st[key]
	if len(lots) > 0 && count > 0 {
		total, origin, mixed := 0.0, lots[0].MiningRun, false
		startsMined, mixedMiningOrigin := lots[0].Origin == "mined", false
		for _, item := range lots {
			total += item.Count
			mixed = mixed || item.MiningRun != origin
			mixedMiningOrigin = mixedMiningOrigin || (item.Origin == "mined") != startsMined
		}
		// A complete removal of the tracked inventory consumes each lot.
		// A partial removal of a mix is ambiguous, as is a transaction
		// exceeding our entire inventory because some cargo went untracked.
		missingCargo := count > total
		if missingCargo || count < total && (mixed || mixedMiningOrigin) {
			for i := range lots {
				if run := lots[i].MiningRun; (mixed || missingCargo) && run != "" {
					if result.AmbiguousRuns == nil {
						result.AmbiguousRuns = make(map[string]bool)
					}
					result.AmbiguousRuns[run] = true
					lots[i].MiningRun = ""
				}
				if (mixedMiningOrigin || missingCargo) && lots[i].Origin == "mined" {
					lots[i].Origin = "unknown"
				}
			}
		}
	}
	return take(st, key, count)
}

// MiningInventory returns identifiable remaining mining cargo in known holds
// and carrier warehouses, limited to the runs shown in the comparison.
func (s *Service) MiningInventory(runIDs map[string]bool) map[string]map[string]MiningStock {
	inventory := make(map[string]map[string]MiningStock)
	if len(runIDs) == 0 {
		return inventory
	}
	addStore := func(st store, carrier bool) {
		for commodity, lots := range st {
			for _, item := range lots {
				if !runIDs[item.MiningRun] {
					continue
				}
				stock := inventory[item.MiningRun]
				if stock == nil {
					stock = make(map[string]MiningStock)
					inventory[item.MiningRun] = stock
				}
				total := stock[commodity]
				if carrier {
					total.CarrierTons += item.Count
				} else {
					total.ShipTons += item.Count
				}
				stock[commodity] = total
			}
		}
	}
	for _, st := range s.Holds {
		addStore(st, false)
	}
	for _, st := range s.Warehouses {
		addStore(st, true)
	}
	return inventory
}
