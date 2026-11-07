package colonization

import (
	"embed"
	"encoding/json"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/value"
	"strconv"
)

//go:embed advice.json
var adviceAssets embed.FS
var adviceData M

func init() {
	b, _ := adviceAssets.ReadFile("advice.json")
	if e := json.Unmarshal(b, &adviceData); e != nil {
		panic(e)
	}
}
func economyPlan(goal string, buildings []M) M {
	selected := M{}
	for _, e := range economies {
		if str(e["id"]) == goal {
			selected = e
		}
	}
	support := []M{}
	for _, b := range buildings {
		if str(b["economy"]) == goal {
			support = append(support, M{"id": b["id"], "name": b["name"], "body": b["body"]})
		}
	}
	guidance := "Выберите направление экономики для плана развития."
	if len(selected) > 0 {
		guidance = "Размещайте профильные объекты с учётом связей с портом и свойств небесного тела. Проверяйте итоговую экономику по данным игры."
	}
	if goal == "refinery" {
		guidance = "Перерабатывающий узел — профильная поддержка. Проверяйте связи с портом на том же небесном теле и итоговые пропорции экономик в игре."
	}
	return M{"goal": str(selected["id"]), "name": firstText(selected["name"], "Не задана"), "supportingBuildings": support, "guidance": guidance, "exactSimulation": false, "warning": "Направление объектов не предсказывает точные доли экономик, производство, население или доход. Необходимые слоты и зависимости подтверждаются на карте системы."}
}
func buildAdvice(project M, buildings []M) []M {
	primary := value.Obj(adviceData["primary"])
	for _, b := range buildings {
		if value.Bool(b["isPrimary"]) && str(b["status"]) == "complete" {
			primary = merge(copyM(value.Obj(adviceData["primaryComplete"])), M{"typeIds": []string{str(b["typeId"])}})
			break
		}
	}
	out := []M{copyM(primary), copyM(value.Obj(adviceData["tiers"]))}
	directions := value.Obj(adviceData["directions"])
	d := value.Obj(directions[str(project["goalEconomy"])])
	if len(d) == 0 {
		d = value.Obj(directions[""])
	}
	out = append(out, copyM(d))
	deficits := 0
	for _, b := range buildings {
		tier := num(b["tier"])
		key := "tier2"
		if tier == 3 {
			key = "tier3"
		}
		before := value.Obj(b["pointsBefore"])
		if str(b["status"]) != "planned" || tier != 2 && tier != 3 || num(b["pointCost"]) <= 0 || before[key] == nil || num(before[key]) >= num(b["pointCost"]) {
			continue
		}
		deficits++
		if deficits > 3 {
			continue
		}
		missing := num(b["pointCost"]) - num(before[key])
		txt := fmt.Sprintf("По текущей очереди перед этим этапом не хватает %.0f очк. T2. Поставьте раньше подходящие объекты T1 и завершите их. Затем обновите текущий баланс по экрану архитектора. Не расходуйте одни и те же очки на параллельные стройки.", missing)
		ids := []string{"settlement-medium", "outpost-civilian"}
		if tier == 3 {
			txt = fmt.Sprintf("По текущей очереди перед этим этапом не хватает %.0f очк. T3. Поставьте раньше нужные узлы или большие поселения T2 и завершите их; сначала обеспечьте их очками T2. После этого обновите баланс по экрану архитектора.", missing)
			ids = []string{"settlement-large", "hub-industrial", "hub-refinery"}
		}
		out = append(out, M{"title": "Подготовьте очки перед «" + str(b["name"]) + "»", "text": txt, "typeIds": ids})
	}
	if deficits > 3 {
		out = append(out, M{"title": "Проверьте оставшуюся очередь", "text": fmt.Sprintf("Ещё %d этапов имеют расчётный дефицит очков. Начните с первого предупреждения в очереди и пересчитайте план после изменения порядка.", deficits-3)})
	}
	points := value.Obj(project["points"])
	if deficits == 0 && (points["tier2"] == nil || points["tier3"] == nil) {
		out = append(out, M{"title": "Уточните исходный баланс очков", "text": "Введите доступные очки T2 и T3 по экрану архитектора. Это текущий остаток, уже учитывающий существующие здания и начатые стройки. Пока баланс неизвестен, возможность начать следующий этап не подтверждена."})
	}
	return out
}
func nextTask(project M, buildings []M, deadline any) M {
	if has([]string{"complete", "cancelled"}, str(project["status"])) {
		return nil
	}
	if str(project["status"]) == "planned" {
		return M{"kind": "claim", "text": "Проверьте заявку на " + str(project["targetSystem"]) + " у контакта колонизации.", "system": project["originSystem"]}
	}
	var current M
	for _, b := range buildings {
		if has([]string{"building", "planned"}, str(b["status"])) {
			current = b
			break
		}
	}
	if current == nil {
		return M{"kind": "plan", "text": "Добавьте следующий объект в план строительства.", "system": project["targetSystem"]}
	}
	var needed M
	for _, r := range rows(current["requirements"]) {
		if num(r["missing"]) > 0 && (needed == nil || num(r["missing"]) > num(needed["missing"])) {
			needed = r
		}
	}
	kind, txt := "inspect-depot", str(current["name"])+": посетите депо для точного списка товаров."
	if needed != nil {
		kind = "delivery"
		txt = str(current["name"]) + ": доставить " + str(needed["name"]) + " — " + formatNumber(needed["missing"]) + " т"
	} else if len(rows(current["requirements"])) > 0 {
		kind = "complete"
		txt = str(current["name"]) + ": проверьте завершение строительства в игре."
	}
	var market any
	if str(current["marketId"]) != "" {
		market = current["marketId"]
	}
	return M{"kind": kind, "text": txt, "system": project["targetSystem"], "buildingId": current["id"], "marketId": market, "deadline": deadline, "commodity": needed["commodity"], "count": needed["missing"], "authoritative": value.Bool(needed["authoritative"])}
}
func strconvFormat(n float64) string { return strconv.FormatFloat(n, 'f', 0, 64) }
