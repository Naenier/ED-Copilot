package app

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// ExportCSV uses the selected colony when the exported table is colony-specific.
func (e *Engine) ExportCSV(kind string) (string, error) { return e.ExportCSVFor(kind, "") }

// ExportCSVFor keeps an explicit colony selection even when a native save dialog
// remains open while the user or another window selects a different colony.
func (e *Engine) ExportCSVFor(kind, projectID string) (string, error) {
	switch kind {
	case "colonization":
		return e.Colonies.ExportCSV(projectID)
	case "route":
		return e.Route.ExportCSV()
	case "trading":
		e.domainMu.Lock()
		transactions := v.Rows(v.Clone(e.Trading.Transactions))
		e.domainMu.Unlock()
		rows := [][]string{{"Время", "Операция", "Товар", "Тонны", "Сумма", "Маржа", "Внутренний перевод", "Система", "Станция"}}
		for _, r := range transactions {
			rows = append(rows, []string{v.Str(r["timestamp"]), p.Label(r["kind"]), v.Str(r["name"]), p.Number(r["count"]), p.Number(r["value"]), p.Number(r["margin"]), p.Format(r["internal"]), v.Str(r["system"]), v.Str(r["station"])})
		}
		return exportRows(rows)
	case "engineering":
		state := e.Snapshot()
		out := [][]string{{"Материал", "Категория", "Уровень", "Есть", "Нужно", "Не хватает"}}
		for _, r := range rows(state, "engineering", "materials") {
			if v.Num(r["needed"]) > 0 {
				out = append(out, []string{v.Str(r["name"]), p.Label(r["category"]), v.Str(r["grade"]), p.Number(r["owned"]), p.Number(r["needed"]), p.Number(r["missing"])})
			}
		}
		return exportRows(out)
	case "statistics", "stats", "":
		state := e.Snapshot()
		out := [][]string{{"Ресурс", "Произведено, т", "Сырьё, ед.", "Продано, т", "Выручка, кр."}}
		for _, r := range rows(state, "stats", "byResource") {
			out = append(out, []string{v.Str(r["name"]), p.Number(r["refinedTons"]), p.Number(r["materialUnits"]), p.Number(r["soldTons"]), p.Number(r["revenue"])})
		}
		return exportRows(out)
	default:
		return "", errors.New("Неизвестный вид таблицы для экспорта")
	}
}

func (e *Engine) ExportPlan(kind string) ([]byte, error) { return e.ExportPlanFor(kind, "") }

func (e *Engine) ExportPlanFor(kind, projectID string) ([]byte, error) {
	switch kind {
	case "colonization":
		return e.Colonies.ExportPlan(projectID)
	case "engineering":
		return encodeJSON(M{"format": "ed-copilot-engineering", "version": 1, "items": e.Engineering.ExportSettings()["items"]})
	case "objectives", "":
		return encodeJSON(e.Snapshot()["objectives"])
	default:
		return nil, errors.New("Неизвестный вид плана для экспорта")
	}
}

func encodeJSON(value any) ([]byte, error) { return json.MarshalIndent(value, "", "  ") }

func exportRows(rows [][]string) (string, error) {
	var b strings.Builder
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	w.Comma = ';'
	for _, row := range rows {
		cells := append([]string(nil), row...)
		for i, s := range cells {
			trimmed := strings.TrimLeft(s, " \t\r\n")
			if trimmed != "" && strings.ContainsRune("=+@-", rune(trimmed[0])) {
				if _, numeric := v.Number(trimmed); !numeric {
					cells[i] = "'" + s
				}
			}
		}
		if err := w.Write(cells); err != nil {
			return "", err
		}
	}
	w.Flush()
	return b.String(), w.Error()
}
