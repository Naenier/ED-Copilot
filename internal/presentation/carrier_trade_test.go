package presentation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestCarrierTradePresentationModesAndUnknownInputs(t *testing.T) {
	p := carrier.NewTradePlanner(nil)
	state := M{"carrierTrade": p.Snapshot()}
	before := v.Copy(state)
	for _, mode := range []string{"Рейс", "Расходы", "Проверки"} {
		page := CarrierTradePage(mode, "", state)
		if len(page.Rows) == 0 || len(page.Columns) == 0 || len(page.Modes) != 3 || page.Title != "Торговля флотоносцем" {
			t.Fatal("incomplete page", mode, page)
		}
		for _, row := range page.Rows {
			if row.ID == "" || len(row.Cells) != len(page.Columns) || row.Details == "" {
				t.Fatal("invalid row", mode, row)
			}
			if row.ID == "profit" && row.Cells[1] != "—" {
				t.Fatal("unknown result shown as free", row)
			}
		}
	}
	if !reflect.DeepEqual(before, v.Copy(state)) {
		t.Fatal("presentation mutated planner state")
	}
	if len(CarrierTradePage("Расходы", "безубыточная", state).Rows) != 1 {
		t.Fatal("Russian cost search did not narrow the rows")
	}
	if len(CarrierTradePage("Рейс", "definitely no match", state).Rows) != 0 {
		t.Fatal("unmatched query returned unrelated legs")
	}
}

func TestCarrierTradeActionsProduceIndependentDomainCommands(t *testing.T) {
	p := carrier.NewTradePlanner(nil)
	for _, mode := range []string{"Рейс", "Расходы", "Проверки"} {
		state := M{"carrierTrade": p.Snapshot(), "commander": M{"system": "Synthetic Origin"}}
		for _, action := range CarrierTradeActions(mode, state, M{"id": "profit"}) {
			if action.ID == "carrier-trade.source" {
				if action.Kind != "url" || !strings.HasPrefix(action.Path, "https://pilotstradenetwork.com/") {
					t.Fatal("invalid primary-source action", action)
				}
				continue
			}
			if action.Path != "/api/carrier/trade" {
				t.Fatal("wrong endpoint", action.ID)
			}
			values := M{}
			for _, field := range action.Fields {
				parsed, err := ParseField(field, field.Default)
				if err != nil {
					t.Fatal(action.ID, field.Key, err)
				}
				values[field.Key] = parsed
			}
			body, err := action.Body(values)
			if err != nil {
				t.Fatal(action.ID, err)
			}
			switch action.ID {
			case "carrier-trade.import-carrier":
				if body["action"] != "import-carrier" {
					t.Fatal("import command lost", body)
				}
				continue
			case "carrier-trade.import-route":
				if body["leg"] != "outbound" {
					t.Fatal("cost row became an invalid leg default", body)
				}
				continue
			case "carrier-trade.options":
				if v.At(body, "options", "tritiumPrice") != nil || v.At(body, "options", "durationHours") != nil {
					t.Fatal("optional unknown values replaced with zero", body)
				}
			case "carrier-trade.reset":
				if action.Confirm == "" {
					t.Fatal("destructive reset lacks confirmation")
				}
			}
			if err := p.Update(body); err != nil {
				t.Fatal(action.ID, body, err)
			}
		}
	}
	state := M{"carrierTrade": p.Snapshot()}
	for _, a := range CarrierTradeActions("Рейс", state, M{"id": "return"}) {
		if a.ID == "carrier-trade.import-route" && a.Fields[0].Default != "return" {
			t.Fatal("selected return leg ignored")
		}
		if a.ID == "carrier-trade.return" {
			body, err := a.Body(M{"quantity": 400})
			if err != nil || v.At(body, "legs", "return", "quantity") != 400 || v.At(body, "legs", "outbound") != nil {
				t.Fatal("return editor targets the wrong leg", body, err)
			}
		}
	}
}
