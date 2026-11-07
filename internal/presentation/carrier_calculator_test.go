package presentation

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/carrier"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func calculatorPresentationFixture(t *testing.T, kind string) (*carrier.Calculator, M) {
	t.Helper()
	c := carrier.NewCalculator(nil)
	if err := c.Update(M{"carrierType": kind, "jumpsPerWeek": 3}); err != nil {
		t.Fatal(err)
	}
	return c, M{"carrierCalculator": c.Snapshot()}
}

func TestCarrierCalculatorPageEveryTypeModeAndUnknownValues(t *testing.T) {
	for _, kind := range []string{"fleet", "squadron"} {
		_, state := calculatorPresentationFixture(t, kind)
		before := v.Copy(state)
		for _, mode := range []string{"Сводка", "Службы", "Сравнение"} {
			page := CarrierCalculatorPage(mode, "", state)
			if len(page.Columns) == 0 || len(page.Rows) == 0 || len(page.Modes) != 3 {
				t.Fatal(kind, mode, page)
			}
			for _, r := range page.Rows {
				if len(r.Cells) != len(page.Columns) || r.ID == "" || r.Details == "" {
					t.Fatal("incomplete native row", kind, mode, r)
				}
			}
		}
		if !reflect.DeepEqual(before, v.Copy(state)) {
			t.Fatal("presentation mutated calculator state")
		}
		summary := CarrierCalculatorPage("Сводка", "", state)
		foundUnknown, foundCalendar := false, false
		for _, r := range summary.Rows {
			if r.ID == "week-fuel" {
				foundUnknown = r.Cells[1] == "—"
			}
			if r.ID == "month" {
				foundCalendar = strings.Contains(r.Details, "30/7") && strings.Contains(r.Details, "пять")
			}
		}
		if !foundUnknown || !foundCalendar {
			t.Fatal("unknown fuel or billing-period distinction lost")
		}
		filtered := CarrierCalculatorPage("Службы", "боеприпасы", state)
		if len(filtered.Rows) != 1 || filtered.Rows[0].ID != "rearm" {
			t.Fatal("Russian service search", filtered)
		}
	}
	if len(CarrierCalculatorPage("Сводка", "", M{}).Columns) == 0 {
		t.Fatal("empty initial snapshot has no table schema")
	}
}

func TestCarrierCalculatorActionsParseAndApplyToDomain(t *testing.T) {
	for _, kind := range []string{"fleet", "squadron"} {
		c, state := calculatorPresentationFixture(t, kind)
		services := v.Rows(v.At(state, "carrierCalculator", "services"))
		for _, mode := range []string{"Сводка", "Службы", "Сравнение"} {
			selected := M{}
			if mode == "Службы" {
				selected = services[0]
			}
			if mode == "Сравнение" {
				selected = v.Rows(v.At(state, "carrierCalculator", "comparison"))[0]
			}
			for _, action := range CarrierCalculatorActions(mode, state, selected) {
				if action.Disabled || action.Kind != "" {
					continue
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
				if action.Path != "/api/carrier/calculator" {
					t.Fatal("wrong command", action.Path)
				}
				if err = c.Update(body); err != nil {
					t.Fatal(action.ID, body, err)
				}
			}
		}
	}
}

func TestCalculatorSuspensionDoesNotInstallNewServices(t *testing.T) {
	c, state := calculatorPresentationFixture(t, "fleet")
	if err := c.Update(M{"services": M{"repair": "active"}}); err != nil {
		t.Fatal(err)
	}
	state["carrierCalculator"] = c.Snapshot()
	for _, a := range CarrierCalculatorActions("Службы", state, M{}) {
		if a.ID != "carrier-calculator.suspend-all" {
			continue
		}
		body, err := a.Body(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.Update(body); err != nil {
			t.Fatal(err)
		}
	}
	options := v.Obj(c.Snapshot()["options"])
	if v.At(options, "services", "repair") != "suspended" || v.At(options, "services", "refuel") != "off" || v.At(options, "services", "shipyard") != "off" {
		t.Fatal("suspension installed optional services", options)
	}
	_, squadron := calculatorPresentationFixture(t, "squadron")
	for _, service := range v.Rows(v.At(squadron, "carrierCalculator", "services")) {
		if !v.Bool(service["mandatory"]) {
			continue
		}
		found := false
		for _, a := range CarrierCalculatorActions("Службы", squadron, service) {
			if a.ID == "carrier-calculator.service" {
				found = true
				if !a.Disabled {
					t.Fatal("mandatory shipyard editable")
				}
			}
		}
		if !found {
			t.Fatal("missing service action contract")
		}
	}
}
