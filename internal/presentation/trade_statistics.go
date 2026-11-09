package presentation

import (
	"fmt"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func tradeStatisticsPage(p Page, state M) Page {
	ledger := v.Obj(state["tradeLedger"])
	totals := v.Obj(ledger["totals"])
	p.Columns = []string{"Товар", "Куплено, т", "Продано, т", "Закупки, кр.", "Выручка, кр.", "Маржа, кр."}
	p.Rows = nil
	p.Summary = fmt.Sprintf("Покупок: %s · Продаж: %s · Выручка: %s кр.",
		Number(totals["buyCount"]), Number(totals["sellCount"]), Number(totals["revenue"]))
	for _, resource := range v.Rows(ledger["byResource"]) {
		bought, sold := v.Num(resource["boughtTons"]), v.Num(resource["soldTons"])
		if bought <= 0 && sold <= 0 {
			continue
		}
		margin := "—"
		if sold > v.Num(resource["unknownCostTons"]) {
			margin = Number(resource["grossMargin"])
		}
		p.Rows = append(p.Rows, row(resource, []string{v.Text(resource["name"], resource["key"]),
			Number(bought), Number(sold), Number(resource["purchaseCost"]), Number(resource["revenue"]), margin},
			fields(resource, "unknownCostTons", "Продано без себестоимости, т")))
	}
	return p
}
