package presentation

import "testing"

func TestTradeHUDShowsDockedOperationsAndMeasuredRate(t *testing.T) {
	run := M{
		"atStation": M{
			"system": "Beta", "station": "Beta Dock",
			"buy":  M{"name": "Сталь", "key": "steel", "quantity": 80},
			"sell": M{"name": "Золото", "key": "gold", "quantity": 100},
		},
		"target":         M{"system": "Alpha", "station": "Alpha Port"},
		"profitPerRound": 32000, "completedRounds": 0,
		"legs": []M{{}, {}},
	}
	state := M{"settings": M{"activity": "trade"}, "tradeRun": run}
	card := hudSection(BuildHUD(state, hudFixtureNow), "ТОРГОВЫЙ РЕЙС")
	if card == nil || card.Subtitle != "Beta · Beta Dock" || len(card.Lines) != 4 ||
		card.Lines[0].Text != "Продать" || card.Lines[0].Value != "Золото · 100 т" ||
		card.Lines[1].Text != "Купить" || card.Lines[2].Text != "Далее · Alpha · Alpha Port" ||
		card.Lines[3].Text != "За круг по ценам рынка" {
		t.Fatalf("incorrect docked trade instructions: %+v", card)
	}
	run["completedRounds"], run["actualProfitPerHour"] = 1, 235000
	state["settings"] = M{"activity": "trade", "language": "en"}
	card = hudSection(BuildHUD(state, hudFixtureNow), "TRADE ROUTE")
	if card == nil || len(card.Lines) != 5 || card.Lines[4].Text != "Actual profit per hour" {
		t.Fatalf("incorrect observed hourly profit: %+v", card)
	}
}
