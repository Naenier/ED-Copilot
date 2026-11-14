package presentation

import (
	"strings"

	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudActiveTradeRoute(doc *HUDDocument, state M) bool {
	run := v.Obj(state["tradeRun"])
	target, station := v.Obj(run["target"]), v.Obj(run["atStation"])
	if len(target) == 0 && len(station) == 0 {
		return false
	}
	point := target
	if len(station) != 0 {
		point = station
	}
	language := Language(state)
	translate := func(text string) string { return localization.T(language, text) }
	number := func(value any) string {
		label := Number(value)
		if language == "en" {
			return localizedNumber(label)
		}
		return label
	}
	location := func(m M) string {
		parts := []string{}
		for _, key := range []string{"system", "station"} {
			if name := v.Str(m[key]); name != "" {
				parts = append(parts, name)
			}
		}
		return strings.Join(parts, " · ")
	}
	rows := []HUDLine{}
	for _, operation := range []struct{ key, label, role string }{
		{"sell", "Продать", "good"}, {"buy", "Купить", "text"},
	} {
		item := v.Obj(point[operation.key])
		if len(item) == 0 {
			continue
		}
		name := v.Text(item["name"], item["key"])
		if language == "en" {
			name = v.Text(item["english"], localization.Text("en", name))
		}
		if count, ok := v.Number(item["quantity"]); ok && count > 0 {
			name += " · " + number(count) + translate(" т")
		}
		rows = append(rows, hudValue(translate(operation.label), name, operation.role))
	}
	if len(station) != 0 && len(target) != 0 && location(station) != location(target) {
		rows = append(rows, hudMuted(translate("Далее")+" · "+location(target)))
	}
	if profit, known := v.Number(run["profitPerRound"]); known {
		rows = append(rows, hudValue(translate("За круг по ценам рынка"), number(profit)+translate(" кр."), "muted"))
	}
	if v.Int(run["completedRounds"]) > 0 {
		if rate, known := v.Number(v.First(run["actualProfitPerHour"], run["profitPerHour"])); known {
			label := "В час по времени круга"
			if run["actualProfitPerHour"] != nil {
				label = "Фактически в час"
			}
			rows = append(rows, hudValue(translate(label), number(rate)+translate(" кр."), "good"))
		}
	}
	if len(rows) == 0 {
		return false
	}
	hudCard(doc, "trade-route", "ТОРГОВЫЙ РЕЙС", location(point), 0, rows...)
	return true
}
