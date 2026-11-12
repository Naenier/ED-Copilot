package presentation

import (
	"sort"
	"time"

	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudOdyssey(doc *HUDDocument, state M, now time.Time) {
	if v.At(state, "commander", "vehicle") != "foot" {
		return
	}
	plan := v.Obj(state["odyssey"])
	active := v.Str(v.At(plan, "activeLoadout", "name"))
	if active == "" {
		return
	}
	language := Language(state)
	name := func(row M) string {
		if language == "en" {
			return v.Text(row["english"], row["name"], row["key"])
		}
		return v.Text(row["name"], row["english"], row["key"])
	}
	text := func(label string) string { return localization.T(language, label) }
	needs := []M{}
	materials := map[string]M{}
	for _, row := range v.Rows(plan["portfolioMaterials"]) {
		materials[v.Key(row["key"])] = row
	}
	for _, row := range v.Rows(plan["materials"]) {
		materials[v.Key(row["key"])] = row
		if v.Num(row["count"]) > 0 && (!hudKnown(row["missing"]) || v.Num(row["missing"]) > 0) {
			needs = append(needs, row)
		}
	}
	priority := func(row M) float64 { return v.Num(v.First(row["missing"], row["missingAtMost"], row["count"])) }
	sort.SliceStable(needs, func(i, j int) bool { return priority(needs[i]) > priority(needs[j]) })
	shown := min(len(needs), 5)
	lines := make([]HUDLine, 0, shown)
	for _, row := range needs[:shown] {
		label, role := text("Не хватает"), "warn"
		amount := Number(priority(row))
		if !hudKnown(row["missing"]) {
			label, role = text("Нужно"), "muted"
			amount = Number(row["count"])
			if v.Num(row["missingAtMost"]) > 0 {
				label = text("Не хватает")
				amount = "≤ " + Number(row["missingAtMost"])
			}
		}
		lines = append(lines, hudValue(name(row), label+" "+amount, role))
	}
	hudCard(doc, "engineering", "МАТЕРИАЛЫ ODYSSEY", active, len(needs)-shown, lines...)

	finds := []HUDLine{}
	for _, pickup := range v.Rows(plan["recentPickups"]) {
		row := materials[v.Key(pickup["key"])]
		at := v.Time(pickup["observedAt"])
		if row == nil || at.IsZero() || now.Sub(at) > 45*time.Second || at.Sub(now) > 2*time.Second || v.Num(pickup["count"]) <= 0 {
			continue
		}
		quantity := "+" + Number(pickup["count"])
		status, role := "", "good"
		switch {
		case v.Num(row["spare"]) > 0:
			status, role = text("Запас")+" "+Number(row["spare"]), "muted"
		case v.Num(row["count"]) == 0 && (v.Num(row["sharedCount"]) > 0 || v.Num(row["reservedOther"]) > 0):
			status = text("Для другого комплекта")
		case v.Num(row["missing"]) > 0:
			status = text("Не хватает") + " " + Number(row["missing"])
		}
		finds = append(finds, hudValue(name(row)+" "+quantity, status, role))
	}
	hudCard(doc, "engineering", "В РЮКЗАК ДОБАВЛЕНО", "", 0, finds...)
}
