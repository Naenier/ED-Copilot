package presentation

import v "github.com/Naenier/ED-Copilot/internal/value"

func hudCartographyTargets(progress M) []M {
	if targets, available := progress["targets"]; available {
		return v.Rows(targets)
	}
	if target := v.Obj(progress["target"]); len(target) > 0 {
		return []M{target}
	}
	return nil
}

func hudCartography(doc *HUDDocument, state M) {
	progress := v.Obj(v.At(state, "riches", "progress"))
	targets := hudCartographyTargets(progress)
	next := v.Str(progress["nextSystem"])
	if len(targets) == 0 && next == "" {
		return
	}

	rewardKey, totalKey := "mappingCredits", "totalMappingCredits"
	if useMapping, present := progress["useMappingValue"]; present && !v.Bool(useMapping) {
		rewardKey, totalKey = "scanCredits", "totalScanCredits"
	}
	lines := make([]HUDLine, 0, len(targets))
	for _, target := range targets {
		body := v.Text(target["designation"], target["name"])
		if body == "" {
			continue
		}
		role := "good"
		if v.Bool(target["completed"]) {
			body = "✓ " + body
			role = "muted"
		}
		if distance := target["distanceLs"]; hudKnown(distance) {
			body += " · " + Number(distance) + " св. с."
		}
		var reward string
		if credits := target[rewardKey]; hudKnown(credits) {
			reward = "≈ " + Number(credits) + " кр."
		}
		lines = append(lines, hudValue(body, reward, role))
	}
	title := "КАРТОГРАФИЯ"
	if len(lines) > 0 {
		system := v.Text(progress["currentSystem"], v.At(state, "commander", "system"))
		hudCard(doc, "cartography-targets", title, system, 0, lines...)
		title = "МАРШРУТ КАРТОГРАФИИ"
	}
	if v.Bool(progress["finished"]) {
		return
	}

	lines = make([]HUDLine, 0, 3)
	if next != "" {
		lines = append(lines, hudValue("Следующая система", next, "text"))
	}
	if total := v.First(progress["totalRewardCredits"], progress[totalKey]); hudKnown(total) {
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
	hudCard(doc, "navigation", title, "", 0, lines...)
}
