package presentation

import (
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudShipWaypoint(doc *HUDDocument, state M) {
	stop := v.Obj(state["shipRoute"])
	system := v.Str(stop["system"])
	if system == "" {
		return
	}
	lines := []HUDLine{hudText(system)}
	if station := v.Str(stop["station"]); station != "" {
		lines = append(lines, hudText(station))
	}
	if action := v.Str(stop["action"]); action != "" {
		lines = append(lines, hudText(action))
	}
	hudCard(doc, "navigation", "Следующая точка", Number(stop["next"])+" / "+Number(stop["total"]), 0, lines...)
}

func hudEngineeringStop(doc *HUDDocument, state M) bool {
	stop := v.Obj(state["engineeringStop"])
	system := v.Str(stop["system"])
	if system == "" {
		return false
	}
	label := map[string]string{"trade": "Обменять материалы", "unlock": "Открыть инженера", "upgrade": "Улучшить модули"}[v.Str(stop["action"])]
	hudCard(doc, "navigation", label, system, 0, hudText(v.Text(stop["station"], system)))
	return true
}

func hudJournalNotice(doc *HUDDocument, state M, now time.Time) {
	event := v.Obj(state["notificationHUD"])
	if !now.Before(v.Time(event["displayUntil"])) {
		return
	}
	title, detail := v.Str(event["title"]), v.Str(event["detail"])
	if Language(state) == "en" {
		title, detail = v.Text(event["englishTitle"], title), v.Text(event["englishDetail"], detail)
	}
	priority := v.Int(event["priority"])
	if detail == "" {
		// The HUD requires a line, so use the event title as its only text.
		detail, title = title, ""
	}
	kind, line := "navigation", hudText(detail)
	if priority == 3 {
		kind, line = "warn", hudWarn(detail)
	}
	hudCard(doc, kind, title, "", 0, line)
	if priority == 3 && len(doc.Sections) > 1 {
		last := len(doc.Sections) - 1
		urgent := doc.Sections[last]
		copy(doc.Sections[1:], doc.Sections[:last])
		doc.Sections[0] = urgent
	}
}
