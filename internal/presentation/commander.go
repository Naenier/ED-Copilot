package presentation

import (
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// CommanderPage is the compact native fallback for the custom commander view.
// Modes remain stable Russian tokens, matching the other presentation pages.
func CommanderPage(s M, selection ...string) Page {
	mode, query := "Сводка", ""
	if len(selection) > 0 && selection[0] != "" {
		mode = selection[0]
	}
	if len(selection) > 1 {
		query = selection[1]
	}
	text := func(ru, en string) string {
		if Language(s) == "en" {
			return en
		}
		return ru
	}
	p := Page{Title: text("Командир", "Commander"), Description: text("Подтверждённые сведения о командире, средствах, имуществе и игровых достижениях.", "Observed commander finances, possessions and game statistics."), Modes: []string{"Сводка", "Финансы", "Инвентарь", "Имущество", "Флот", "Статистика", "Журнал"}, Columns: []string{text("Параметр", "Parameter"), text("Значение", "Value")}, Rows: []Row{}}
	profile := v.Obj(s["commanderProfile"])
	cmd := v.Obj(s["commander"])
	add := func(id, title string, value any, data M) {
		formatted := Format(value)
		if data == nil {
			data = M{}
		} else {
			data = v.Copy(data)
		}
		data["key"] = id
		if contains(query, title, formatted, data["name"], data["shipName"], data["system"]) {
			p.Rows = append(p.Rows, row(data, []string{title, formatted}, ""))
		}
	}
	switch mode {
	case "Финансы":
		finance := v.Obj(profile["finance"])
		for _, field := range []struct{ key, ru, en string }{{"credits", "Баланс", "Balance"}, {"loan", "Долг", "Debt"}, {"netWorth", "Стоимость имущества", "Net worth"}, {"shipValue", "Стоимость текущего корабля", "Current ship value"}, {"hullValue", "Стоимость корпуса", "Hull value"}, {"modulesValue", "Стоимость модулей", "Module value"}, {"rebuy", "Страховой выкуп", "Rebuy cost"}} {
			add("finance."+field.key, text(field.ru, field.en), finance[field.key], nil)
		}
		add("finance.updatedAt", text("Время наблюдения", "Observed at"), finance["updatedAt"], nil)
	case "Инвентарь":
		for _, container := range []struct{ key, ru, en string }{{"materials", "Материалы", "Materials"}, {"shipLocker", "Шкаф корабля", "Ship locker"}, {"backpack", "Рюкзак", "Backpack"}} {
			groups := v.Obj(v.At(profile, "inventory", container.key))
			keys := []string{}
			for key := range groups {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				group := v.Obj(groups[key])
				title := text(container.ru, container.en) + " · " + commanderInventoryGroup(key, Language(s))
				add("inventory."+container.key+"."+key, title, group["count"], group)
				if !v.Bool(group["known"]) {
					continue
				}
				for _, item := range v.Rows(group["entries"]) {
					add(container.key+"."+v.Str(item["key"]), title+" · "+v.Str(item["name"]), item["count"], item)
				}
			}
		}
	case "Имущество":
		add("assets.colonies", text("Колонии в планах", "Colony plans"), len(v.Rows(v.At(s, "coloniesSelectors", "projects"))), nil)
		add("assets.carrier", text("Личный флотоносец", "Personal fleet carrier"), v.At(s, "carrier", "name"), v.Obj(s["carrier"]))
		modules := v.Obj(profile["storedModules"])
		add("assets.modules", text("Модулей на хранении", "Stored modules"), modules["count"], modules)
		if v.Bool(modules["known"]) {
			for _, module := range v.Rows(modules["entries"]) {
				add("module."+v.Str(module["id"]), v.Str(module["name"]), v.Text(module["station"], module["system"]), module)
			}
		}
	case "Флот":
		fleet := v.Obj(profile["fleet"])
		current := v.Obj(fleet["currentShip"])
		if v.Bool(current["known"]) {
			add("fleet.current", text("Текущий корабль", "Current ship"), Ship(current), current)
		} else {
			add("fleet.current", text("Текущий корабль", "Current ship"), nil, nil)
		}
		add("fleet.count", text("Кораблей в последнем списке", "Ships in last observed list"), fleet["count"], fleet)
		if v.Bool(fleet["known"]) {
			for _, ship := range v.Rows(fleet["entries"]) {
				add("ship."+v.Str(ship["shipId"]), Ship(ship), v.Text(ship["station"], ship["system"]), ship)
			}
		}
	case "Статистика":
		for _, section := range v.Rows(v.At(profile, "statistics", "sections")) {
			for _, entry := range v.Rows(section["entries"]) {
				add("statistics."+v.Str(section["id"])+"."+v.Str(entry["id"]), v.Str(section["label"])+" · "+v.Str(entry["label"]), entry["value"], entry)
			}
		}
	case "Репутация":
		for _, field := range []struct{ key, ru, en string }{{"federation", "Федерация", "Federation"}, {"empire", "Империя", "Empire"}, {"alliance", "Альянс", "Alliance"}, {"independent", "Независимые", "Independent"}} {
			add("reputation."+field.key, text(field.ru, field.en), v.At(profile, "reputation", field.key), nil)
		}
	case "Журнал":
		for _, entry := range v.Rows(v.At(s, "pilot", "recent")) {
			add(v.Str(entry["timestamp"])+"."+v.Str(entry["event"]), Date(entry["timestamp"])+" · "+v.Str(entry["label"]), v.Text(entry["detail"], entry["system"]), entry)
		}
	default:
		add("identity.name", text("Командир", "Commander"), cmd["name"], M{"commanderName": cmd["name"]})
		add("identity.system", text("Система", "System"), cmd["system"], cmd)
		add("identity.ship", text("Текущий корабль", "Current ship"), Ship(cmd), cmd)
		add("summary.credits", text("Баланс", "Balance"), v.At(profile, "finance", "credits"), nil)
		add("summary.netWorth", text("Стоимость имущества", "Net worth"), v.At(profile, "finance", "netWorth"), nil)
		for _, field := range []struct{ key, ru, en string }{{"federation", "Федерация", "Federation"}, {"empire", "Империя", "Empire"}, {"alliance", "Альянс", "Alliance"}, {"independent", "Независимые", "Independent"}} {
			if observed := v.At(profile, "reputation", field.key); observed != nil {
				add("reputation."+field.key, text(field.ru, field.en), observed, nil)
			}
		}
		for _, rank := range v.Rows(v.At(s, "ranks", "disciplines")) {
			add("rank."+v.Str(rank["id"]), v.Str(rank["title"]), rank["rankName"], rank)
		}
	}
	p.Summary = strings.TrimSpace(v.Str(cmd["name"]))
	return p
}

func commanderInventoryGroup(key, language string) string {
	labels := map[string][2]string{"raw": {"Сырьё", "Raw materials"}, "manufactured": {"Промышленные материалы", "Manufactured materials"}, "encoded": {"Данные", "Encoded materials"}, "items": {"Предметы", "Items"}, "components": {"Компоненты", "Components"}, "consumables": {"Расходники", "Consumables"}, "data": {"Данные", "Data"}}
	if names, ok := labels[key]; ok {
		if language == "en" {
			return names[1]
		}
		return names[0]
	}
	return key
}
