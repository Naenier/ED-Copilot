package presentation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestHUDPurchaseShowsStockInsteadOfDemand(t *testing.T) {
	doc := &HUDDocument{}
	hudDestination(doc, M{"trading": M{"destination": M{"station": "Synthetic port", "system": "Synthetic system", "selectionMode": "buy", "quotes": []M{{"key": "gold", "name": "Золото", "price": 100, "supply": 200, "demand": 0}}}}}, time.Now())
	body, _ := json.Marshal(doc)
	text := string(body)
	if !strings.Contains(text, "МЕСТО ЗАКУПКИ") || !strings.Contains(text, "запас 200 т") || strings.Contains(text, "спрос") {
		t.Fatal("Purchase HUD describes a sale", text)
	}
}

func TestMarketSearchFormOnlyShowsRelevantAvailabilityControls(t *testing.T) {
	state := M{"commander": M{"system": "Synthetic"}}
	for _, test := range []struct{ mode, visible, hidden string }{{"Купить", "minSupply", "minDemand"}, {"Продать", "minDemand", "minSupply"}} {
		actions := CoreActions("sale", test.mode, state, nil, nil)
		found, hidden := false, false
		for _, a := range actions {
			if a.ID == "market.search" {
				for _, f := range a.Fields {
					found = found || f.Key == test.visible
					hidden = hidden || f.Key == test.hidden
				}
			}
		}
		if !found || hidden {
			t.Fatal("Irrelevant stock/demand control", test.mode)
		}
	}
}
