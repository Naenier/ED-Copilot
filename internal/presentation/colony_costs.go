package presentation

import (
	"github.com/Naenier/ED-Copilot/internal/freshness"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func colonyPriceQuality(row M) freshness.Observation {
	if quality, ok := row["priceQuality"].(freshness.Observation); ok {
		return quality
	}
	quality := v.Obj(row["priceQuality"])
	return freshness.Observation{
		State:      freshness.State(v.Str(quality["state"])),
		ObservedAt: v.Str(quality["observedAt"]), FetchedAt: v.Str(quality["fetchedAt"]),
	}
}

func colonyEstimatePrice(row M) string {
	price := Number(row["galacticAveragePrice"])
	switch colonyPriceQuality(row).State {
	case freshness.Stale:
		return price + " · Устаревшая оценка"
	case freshness.Undated:
		return price + " · Возраст цены неизвестен"
	default:
		return price
	}
}

func colonyProcurement(exact, upper any) string {
	if _, ok := v.Number(exact); ok {
		return Number(exact)
	}
	if _, ok := v.Number(upper); ok {
		return "≤" + Number(upper)
	}
	return "—"
}

func colonyCostDetails(r M) string {
	quality := colonyPriceQuality(r)
	priceDates := fields(M{"observed": quality.ObservedAt, "fetched": quality.FetchedAt}, "observed", "Цена наблюдалась", "fetched", "Цена получена")
	if priceDates == "" {
		priceDates = fields(r, "galacticAveragePriceAt", "Цена получена")
	}
	if priceDates != "" {
		priceDates += "\r\n"
	}
	return "Остаток по строительному складу уже включает принятые поставки. Груз на борту ещё не доставлен.\r\n" +
		"Оценка по среднегалактической цене игры, а не фактические расходы или предложение рынка.\r\n" +
		priceDates + fields(r, "updatedAt", "Наблюдение", "sourceNote", "Источник оценки", "galacticAveragePrice", "Средняя по галактике, кр./т", "requiredCost", "Полная стоимость, кр.", "deliveredCost", "Стоимость принятого, кр.", "missingCost", "Стоимость остатка, кр.", "cargoCoveredCost", "Покрыто грузом корабля, кр.", "carrierCoveredCost", "Покрыто складом флотоносца, кр.", "toBuyCost", "Закупить, кр.", "toBuyAtMostCost", "Закупить не более, кр.")
}
