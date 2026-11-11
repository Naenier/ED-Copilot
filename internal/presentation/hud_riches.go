package presentation

import v "github.com/Naenier/ED-Copilot/internal/value"

func hudRiches(doc *HUDDocument, progress M) bool {
	target := v.Obj(progress["target"])
	body := v.Text(target["name"], target["designation"])
	next := v.Str(progress["nextSystem"])
	if v.Bool(progress["finished"]) || (body == "" && next == "") {
		return false
	}

	lines := make([]HUDLine, 0, 4)
	rewardKey := "mappingCredits"
	if useMapping, present := progress["useMappingValue"]; present && !v.Bool(useMapping) {
		rewardKey = "scanCredits"
	}
	if body != "" {
		var reward string
		if credits := target[rewardKey]; hudKnown(credits) {
			reward = "≈ " + Number(credits) + " кр."
		}
		lines = append(lines, hudValue(body, reward, "good"))
	}
	if next != "" {
		lines = append(lines, hudValue("Следующая система", next, "text"))
	}
	if total := v.First(progress["totalRewardCredits"], progress["totalMappingCredits"]); hudKnown(total) {
		lines = append(lines, hudValue("Оценка маршрута", "≈ "+Number(total)+" кр.", "text"))
	}
	done, remaining := progress["completedJumps"], progress["remainingJumps"]
	if hudKnown(done) && hudKnown(remaining) {
		left := Number(remaining)
		if v.Num(remaining) > 0 {
			left = "≈ " + left
		}
		lines = append(lines, hudValue("Прыжки · сделано / осталось", Number(done)+" / "+left, "muted"))
	}
	hudCard(doc, "navigation", "ДОРОГА К БОГАТСТВУ", "", 0, lines...)
	return true
}
