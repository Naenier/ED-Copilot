package presentation

import v "github.com/Naenier/ED-Copilot/internal/value"

func goalActions(state, selected M, hasRow bool) []Action {
	owner := v.Str(v.At(state, "planWorkspace", "key"))
	fields := func(goal M, fallback string) []Field {
		out := []Field{
			{Key: "title", Label: "Что нужно сделать", Kind: "text", Required: true, Default: goal["title"]},
			selectField("kind", "Занятие", v.First(goal["kind"], "travel"), ChoicePairs("travel", "Перелёт", "trade", "Торговля", "mining", "Добыча", "engineering", "Инженерия", "exploration", "Исследования", "combat", "Бой", "mission", "Поручение", "carrier", "Флотоносец", "colonization", "Колонизация", "odyssey", "Пешие задачи", "other", "Другое")),
			textField("system", "Система", goal["system"]),
			textField("station", "Станция", goal["station"]),
			textField("body", "Тело", goal["body"]),
			selectField("priority", "Приоритет", v.First(goal["priority"], "normal"), ChoicePairs("normal", "Обычный", "high", "Высокий")),
			{Key: "notes", Label: "Заметки", Kind: "multiline", Default: goal["notes"]},
		}
		if owner != "" {
			out = append(out, selectField("scope", "Владелец плана", v.Text(goal["scope"], fallback), ChoicePairs("personal", "Текущий командир", "shared", "Все командиры")))
		}
		return out
	}
	payload := func(action string) M {
		out := M{"action": action, "expectedPlanWorkspace": owner}
		if action != "add" {
			out["id"] = selected["id"]
		}
		return out
	}
	action := func(id, title, verb string, fields ...Field) Action {
		return Action{ID: "goal." + id, Title: title, Path: "/api/objectives", Payload: payload(verb), Fields: fields}
	}
	actions := []Action{
		action("add", "Новая цель", "add", fields(nil, "personal")...),
		action("edit", "Изменить цель", "update", fields(selected, "shared")...),
		action("complete", "Выполнено", "complete"),
		action("reopen", "Вернуть в работу", "reopen"),
		{ID: "goal.delete", Title: "Удалить цель", Path: "/api/objectives", Payload: payload("remove"), Confirm: "Удалить выбранную цель?"},
	}
	isGoal := hasRow && selected["title"] != nil && selected["kind"] != nil
	for i := 1; i < len(actions); i++ {
		actions[i].Disabled = !isGoal
	}
	return actions
}

func goalOwnerLabel(goal M) string {
	if goal["scope"] == "personal" {
		return v.Text(goal["commanderName"], "Текущий командир")
	}
	return "Все командиры"
}

func planWorkspaceTitle(state M) string {
	workspace := v.Obj(state["planWorkspace"])
	name := v.Str(workspace["name"])
	if name == "" {
		return ""
	}
	switch workspace["branch"] {
	case "legacy":
		return name + " · Legacy"
	case "live":
		return name + " · Live"
	default:
		return name
	}
}
