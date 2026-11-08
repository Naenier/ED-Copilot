package presentation

import v "github.com/Naenier/ED-Copilot/internal/value"

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
	return "Остаток по строительному складу уже включает принятые поставки. Груз на борту ещё не доставлен.\r\n" +
		"Оценка по среднегалактической цене игры, а не фактические расходы или предложение рынка.\r\n" +
		fields(r, "updatedAt", "Наблюдение", "sourceNote", "Источник оценки", "galacticAveragePriceAt", "Цена получена", "galacticAveragePrice", "Средняя по галактике, кр./т", "requiredCost", "Полная стоимость, кр.", "deliveredCost", "Стоимость принятого, кр.", "missingCost", "Стоимость остатка, кр.", "cargoCoveredCost", "Покрыто грузом корабля, кр.", "carrierCoveredCost", "Покрыто складом флотоносца, кр.", "toBuyCost", "Закупить, кр.", "toBuyAtMostCost", "Закупить не более, кр.")
}
