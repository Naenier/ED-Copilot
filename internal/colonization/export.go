package colonization

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
)

// ExportCSV exports only the requested colony, in its independent supply order.
// Empty projectID selects the colony currently driving the overlay.
func (p *Planner) ExportCSV(projectID string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	project := p.projectLocked(projectID)
	if project == nil {
		return "", errors.New("Колония не найдена.")
	}
	snapshot := p.projectSnapshot(project, nil, false, nil)
	var b strings.Builder
	b.WriteString("\uFEFF")
	writer := csv.NewWriter(&b)
	writer.Comma = ';'
	if err := writer.Write([]string{"Колония", "Система", "Этап", "Приоритет снабжения", "Товар", "Нужно, т", "Принято, т", "Осталось, т", "Основание"}); err != nil {
		return "", err
	}
	for i, stage := range rows(snapshot["supplyQueue"]) {
		requirements := rows(stage["requirements"])
		if len(requirements) == 0 {
			if err := writer.Write([]string{str(project["name"]), str(project["targetSystem"]), str(stage["name"]), formatNumber(i + 1), "Неизвестно", "", "", "", "Нет списка ресурсов"}); err != nil {
				return "", err
			}
			continue
		}
		for _, r := range requirements {
			source := "Ручная оценка"
			if str(r["source"]) == "journal" || str(r["source"]) == "journal+contribution" {
				source = "Журнал игры"
			}
			if err := writer.Write([]string{str(project["name"]), str(project["targetSystem"]), str(stage["name"]), formatNumber(i + 1), str(r["name"]), formatNumber(r["required"]), formatNumber(r["delivered"]), formatNumber(r["missing"]), source}); err != nil {
				return "", err
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}
func (p *Planner) ExportPlan(projectID string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	project := p.projectLocked(projectID)
	if project == nil {
		return nil, errors.New("Колония не найдена.")
	}
	return json.MarshalIndent(p.projectSnapshot(project, nil, false, nil), "", "  ")
}
