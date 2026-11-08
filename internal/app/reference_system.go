package app

import (
	"net/url"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// ReferenceSystem projects a single observed system. Opening its page never
// copies or evaluates the whole exploration history.
func (e *Engine) ReferenceSystem(name string) M {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 150 {
		return nil
	}
	e.domainMu.Lock()
	var observed M
	for _, system := range e.Mining.World {
		if strings.EqualFold(v.Str(system["name"]), name) {
			observed = v.Copy(system)
			break
		}
	}
	e.mu.RLock()
	current := v.Copy(e.commander)
	e.mu.RUnlock()
	e.domainMu.Unlock()
	properties := []M{{"label": "Название", "value": name}}
	sections := []M{}
	description := "В локальном журнале пока нет подробных наблюдений этой системы. Данные появятся после посещения или поиска мест добычи."
	if observed != nil {
		description = "Наблюдения этой системы, сохранённые в вашем игровом журнале. Состав тел и колец отражает известные сканирования."
		if address := v.Str(observed["address"]); address != "" {
			properties = append(properties, M{"label": "Адрес системы", "value": address})
		}
		if distance := trading.Distance(observed["coordinates"], current["coordinates"]); distance >= 0 && distance < 1e10 {
			properties = append(properties, M{"label": "От текущей системы", "value": presentation.Number(distance) + " св. л."})
		}
		bodies := []M{}
		for _, raw := range v.Obj(observed["bodies"]) {
			bodies = append(bodies, v.Obj(raw))
		}
		sort.Slice(bodies, func(i, j int) bool { return v.Str(bodies[i]["name"]) < v.Str(bodies[j]["name"]) })
		properties = append(properties, M{"label": "Известно тел", "value": presentation.Number(len(bodies))})
		rows := [][]string{}
		for _, body := range bodies[:min(128, len(bodies))] {
			rings := v.Rows(body["rings"])
			landable := "Нет"
			if v.Bool(body["landable"]) {
				landable = "Да"
			}
			rows = append(rows, []string{v.Str(body["name"]), v.Text(body["planetClass"], body["starType"], "Не определён"), landable, presentation.Number(len(rings))})
		}
		if len(rows) > 0 {
			sections = append(sections, M{"id": "bodies", "title": "Исследованные тела", "type": "table", "columns": []string{"Тело", "Тип", "Посадка", "Колец"}, "rows": rows})
		}
		if len(bodies) > 128 {
			sections = append(sections, M{"id": "body-limit", "title": "Большая система", "type": "prose", "paragraphs": []string{"Показаны первые 128 тел. Остальные наблюдения сохранены в истории приложения."}})
		}
	}
	sections = append(sections, M{"id": "next", "title": "Что можно сделать", "type": "prose", "paragraphs": []string{"Скопируйте название для поиска в карте галактики. Рынки, месторождения и задачи снабжения выбирайте в соответствующих инструментах с этой системой в качестве отправной точки."}})
	return M{"id": "system/" + url.PathEscape(name), "category": "systems", "title": name, "subtitle": "Звёздная система", "description": description, "properties": properties, "sections": sections, "related": []M{}, "actions": []M{{"type": "copy-system", "label": "Копировать систему", "system": name}}}
}
