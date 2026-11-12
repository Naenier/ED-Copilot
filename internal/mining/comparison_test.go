package mining

import (
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type comparisonFixture struct {
	mining *Service
	ledger *trading.Service
	start  time.Time
	ctx    M
}

func newComparisonFixture() comparisonFixture {
	return comparisonFixture{
		mining: New(M{"options": M{"resourceKey": "platinum", "sellPrice": 30000}}),
		ledger: trading.New(nil),
		start:  time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC),
		ctx: M{"file": "Journal.synthetic.01.log", "carrierId": "321", "commander": M{
			"vehicle": "ship", "system": "Synthetic", "body": "Synthetic 1 A Ring", "ship": "Python",
		}},
	}
}

func (f comparisonFixture) dispatch(offset time.Duration, kind string, fields M) {
	event := v.Merge(M{"event": kind, "timestamp": v.ISO(f.start.Add(offset))}, fields)
	ctx := v.Copy(f.ctx)
	if kind == "MiningRefined" {
		f.mining.Apply(event, ctx)
		ctx["miningRunId"] = f.mining.RefinedRunID(event, ctx)
	}
	result := f.ledger.Apply(event, ctx)
	ctx["tradeResult"] = result
	if kind != "MiningRefined" {
		f.mining.Apply(event, ctx)
	}
	f.mining.ApplyTradeResult(result, ctx)
}

func (f comparisonFixture) snapshot() M {
	ctx := v.Copy(f.ctx)
	ctx["miningCargo"] = f.ledger.MiningInventory(f.mining.RunIDs())
	return f.mining.SnapshotForUI(ctx, false)
}

func TestMiningComparisonSeparatesConfirmedSalesEstimatesAndCarrierCargo(t *testing.T) {
	f := newComparisonFixture()
	f.dispatch(0, "Loadout", M{"Modules": []M{{"Item": "hpt_mininglaser_fixed_small"}}})
	f.dispatch(time.Minute, "Undocked", nil)
	for i := 0; i < 10; i++ {
		var entries []M
		if i == 1 {
			entries = []M{{"Name": "Platinum", "Proportion": 20}}
		} else if i == 6 {
			entries = []M{{"Name": "Platinum", "Proportion": 30}}
		}
		f.dispatch(5*time.Minute+time.Duration(i)*time.Second, "ProspectedAsteroid", M{"Materials": entries})
	}
	f.dispatch(25*time.Minute, "MiningRefined", M{"Type": "Platinum", "Count": 80})
	f.dispatch(25*time.Minute+time.Second, "Cargo", M{"Vessel": "Ship", "Count": 80})
	active := v.Obj(f.snapshot()["current"])
	mn(t, active["remainingTons"], 80)
	mn(t, active["estimatedRemainingValue"], 2400000)
	mn(t, active["confirmedRevenue"], 0)
	if active["tripSeconds"] != nil || active["ship"] != "Python" || active["body"] != "Synthetic 1 A Ring" ||
		active["method"] != "laser" || active["methodEvidence"] != "loadout" {
		t.Fatalf("incorrect current run context: %v", active)
	}
	mn(t, active["activeSeconds"], 1200)
	mn(t, active["activeTonsPerHour"], 240)
	found := v.Rows(active["composition"])[0]
	mn(t, found["occurrencePercent"], 20)
	mn(t, found["averagePercent"], 25)
	mn(t, found["averageAllScansPercent"], 5)

	f.dispatch(26*time.Minute, "SupercruiseEntry", nil)
	f.dispatch(41*time.Minute, "Docked", nil)
	f.dispatch(42*time.Minute, "CargoTransfer", M{"Transfers": []M{{"Type": "Platinum", "Count": 50, "Direction": "ToCarrier"}}})
	archived := v.Rows(f.snapshot()["history"])[0]
	if archived["id"] != active["id"] {
		t.Fatal("completed run changed its identity")
	}
	mn(t, archived["tripSeconds"], 2400)
	mn(t, archived["tripTonsPerHour"], 120)
	mn(t, archived["carrierRemainingTons"], 50)
	mn(t, archived["remainingTons"], 80)
	mn(t, archived["confirmedRevenue"], 0)

	f.dispatch(43*time.Minute, "CargoTransfer", M{"Transfers": []M{{"Type": "Platinum", "Count": 50, "Direction": "ToShip"}}})
	f.dispatch(44*time.Minute, "MarketSell", M{"Type": "Platinum", "Count": 50, "SellPrice": 28000})
	archived = v.Rows(f.snapshot()["history"])[0]
	mn(t, archived["soldTons"], 50)
	mn(t, archived["confirmedRevenue"], 1400000)
	mn(t, archived["remainingTons"], 30)
	mn(t, archived["estimatedRemainingValue"], 900000)
}

func TestMiningComparisonNeverAssignsMixedPartialSalesByFIFO(t *testing.T) {
	f := newComparisonFixture()
	f.dispatch(0, "MiningRefined", M{"Type": "Platinum", "Count": 20})
	f.dispatch(time.Minute, "SupercruiseEntry", nil)
	f.dispatch(2*time.Minute, "MiningRefined", M{"Type": "Platinum", "Count": 30})
	f.dispatch(3*time.Minute, "Docked", nil)
	f.dispatch(4*time.Minute, "MarketSell", M{"Type": "Platinum", "Count": 10, "TotalSale": 280000})
	runs := v.Rows(f.snapshot()["history"])
	if len(runs) != 2 {
		t.Fatal("expected two runs", runs)
	}
	for _, run := range runs {
		if run["originUncertain"] != true || run["confirmedRevenue"] != nil || run["soldTons"] != nil || run["estimatedRemainingValue"] != nil {
			t.Fatalf("mixed partial sale was presented as a known run: %v", run)
		}
	}
}

func TestMiningComparisonLeavesUnknownConcentrationsUnestimated(t *testing.T) {
	f := newComparisonFixture()
	f.dispatch(0, "ProspectedAsteroid", M{"MotherlodeMaterial": "Platinum"})
	f.dispatch(time.Minute, "ProspectedAsteroid", M{"Materials": []M{{"Name": "Platinum", "Proportion": 30}}})
	compound := v.Rows(v.At(f.snapshot(), "current", "composition"))[0]
	mn(t, compound["occurrencePercent"], 100)
	mn(t, compound["averagePercent"], 30)
	if compound["averageAllScansPercent"] != nil {
		t.Fatalf("a missing concentration was treated as zero: %v", compound)
	}
}

func TestMiningComparisonDoesNotAttachOutOfPeriodCargoToOpenRun(t *testing.T) {
	f := newComparisonFixture()
	f.dispatch(0, "MiningRefined", M{"Type": "Platinum", "Count": 20})
	// Even the same timestamp cannot attach an event omitted by the period.
	f.ctx["countInPeriod"] = false
	f.dispatch(0, "MiningRefined", M{"Type": "Platinum", "Count": 30})
	f.ctx["countInPeriod"] = true
	active := v.Obj(f.snapshot()["current"])
	mn(t, active["refinedTons"], 20)
	mn(t, active["remainingTons"], 20)
	f.dispatch(time.Minute, "Docked", nil)
	f.dispatch(2*time.Minute, "MarketSell", M{"Type": "Platinum", "Count": 10, "TotalSale": 280000})
	run := v.Rows(f.snapshot()["history"])[0]
	if run["originUncertain"] != true || run["confirmedRevenue"] != nil {
		t.Fatalf("sale was attributed to the visible run after mixing with excluded cargo: %v", run)
	}
}

func TestMiningComparisonTracksPlanetaryCargoFromSRVToShip(t *testing.T) {
	f := newComparisonFixture()
	delete(f.ctx, "carrierId")
	commander := v.Obj(f.ctx["commander"])
	commander["vehicle"], commander["body"] = "srv", "Synthetic A 1"
	f.dispatch(0, "MiningRefined", M{"Type": "Platinum", "Count": 8})
	commander["vehicle"] = "ship"
	f.dispatch(time.Minute, "CargoTransfer", M{"Transfers": []M{{"Type": "Platinum", "Count": 8, "Direction": "toship"}}})
	f.dispatch(2*time.Minute, "Docked", nil)
	f.dispatch(3*time.Minute, "MarketSell", M{"Type": "Platinum", "Count": 5, "TotalSale": 140000})
	run := v.Rows(f.snapshot()["history"])[0]
	if run["method"] != "surface-planet" || run["originUncertain"] == true {
		t.Fatalf("known planetary origin was lost: %v", run)
	}
	mn(t, run["confirmedRevenue"], 140000)
	mn(t, run["remainingTons"], 3)
}

func TestMiningComparisonAvoidsAggregateMiningRevenueFromUnclearSales(t *testing.T) {
	for _, test := range []struct {
		label        string
		bought, sold int
	}{{"mixed cargo", 20, 5}, {"unobserved cargo", 0, 25}} {
		t.Run(test.label, func(t *testing.T) {
			f := newComparisonFixture()
			f.dispatch(0, "MiningRefined", M{"Type": "Platinum", "Count": 20})
			if test.bought > 0 {
				f.dispatch(time.Minute, "MarketBuy", M{"Type": "Platinum", "Count": test.bought, "TotalCost": 400000})
			}
			f.dispatch(2*time.Minute, "MarketSell", M{"Type": "Platinum", "Count": test.sold, "SellPrice": 28000})
			snapshot := f.snapshot()
			run := v.Obj(snapshot["current"])
			if run["originUncertain"] != true || run["confirmedRevenue"] != nil {
				t.Fatalf("unproven revenue was attributed: %v", run)
			}
			mn(t, v.At(snapshot, "stats", "total", "miningRevenue"), 0)
			mn(t, f.ledger.Totals["miningRevenue"], 0)
		})
	}
}
