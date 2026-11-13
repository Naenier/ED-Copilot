package presentation

import (
	"strings"
	"time"

	"github.com/Naenier/ED-Copilot/internal/colonization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudText(text string) HUDLine  { return HUDLine{Text: text, Role: "text"} }
func hudMuted(text string) HUDLine { return HUDLine{Text: text, Role: "muted"} }
func hudWarn(text string) HUDLine  { return HUDLine{Text: text, Role: "warn"} }
func hudKnown(raw any) bool        { n, known := v.Number(raw); return known && n >= 0 }
func hudEqual(a, b any) bool {
	return v.Str(a) != "" && strings.EqualFold(strings.TrimSpace(v.Str(a)), strings.TrimSpace(v.Str(b)))
}
func hudContextMatches(record, commander M) bool {
	for _, field := range []string{"system", "body"} {
		if v.Str(record[field]) != "" && v.Str(commander[field]) != "" && !hudEqual(record[field], commander[field]) {
			return false
		}
	}
	return true
}

func buildHUDContent(s M, now time.Time) HUDDocument {
	activity := v.Text(v.At(s, "settings", "activity"), "flight")
	doc := HUDDocument{Activity: activity}
	switch activity {
	case "cartography":
		hudCartography(&doc, s)
	case "exobiology":
		hudExobiology(&doc, s)
	case "asteroid":
		hudAsteroid(&doc, s, now)
	case "surface":
		hudSurface(&doc, s)
	case "raw":
		hudMaterials(&doc, s, true)
		hudTargets(&doc, s, "raw")
	case "trade":
		hudDestination(&doc, s, now)
		hudTradePlan(&doc, s)
	case "carrier":
		hudCarrier(&doc, s, now)
	case "engineering":
		if !hudEngineeringStop(&doc, s) {
			hudMaterials(&doc, s, false)
			hudExchange(&doc, s)
		}
	case "odyssey":
		hudOdyssey(&doc, s, now)
	case "colonization":
		hudColony(&doc, s, now)
	default:
		hudSurfaceNavigation(&doc, s)
		hudShipWaypoint(&doc, s)
		hudFlight(&doc, s)
	}
	hudJournalNotice(&doc, s, now)
	hudCompactContext(&doc, v.Obj(s["commander"]))
	return doc
}

// Only flight uses the pilot's current location as context. Activity cards
// already identify the destination, worksite, carrier or subject they need.
func hudCompactContext(doc *HUDDocument, cmd M) {
	if doc.Activity != "flight" || len(doc.Sections) == 0 {
		return
	}
	if doc.Sections[0].Kind == "surface-navigation" {
		return
	}
	location := v.Str(cmd["system"])
	var point string
	if v.Bool(cmd["docked"]) {
		point = v.Str(cmd["station"])
	} else if cmd["vehicle"] == "foot" || cmd["vehicle"] == "srv" {
		point = v.Str(cmd["body"])
	}
	if point != "" && !hudEqual(location, point) {
		if location != "" {
			location += " · "
		}
		location += point
	}
	doc.Sections[0].Subtitle = location
}

func hudColony(doc *HUDDocument, s M, now time.Time) {
	colony := v.Obj(s["colonization"])
	project := v.Obj(colony["activeProject"])
	switch project["status"] {
	case nil, "complete", "cancelled", "failed":
		return
	}
	label := v.Text(project["targetSystem"], project["name"])
	if name := v.Str(project["name"]); name != "" && !hudEqual(name, label) {
		label = name + " · " + label
	}
	actions, notes := hudColonyActions(colony, project, v.Obj(s["carrier"]), v.Obj(s["commander"]), now)
	hudAddColonyActions(doc, actions, label)
	task := v.Obj(project["nextTask"])
	switch task["kind"] {
	case "claim", "plan", "inspect-depot":
		notes = append(notes, hudText(v.Str(task["text"])))
	}
	if n := v.Int(project["unknownBills"]); n > 0 && task["kind"] != "inspect-depot" {
		notes = append(notes, hudWarn("Сметы неизвестны: "+Number(n)))
	}
	if deadline := v.Time(project["deadline"]); !colonization.PrimaryPortComplete(project) && project["deadlineActive"] != false && !deadline.IsZero() {
		if deadline.Before(now) {
			notes = append(notes, hudWarn("Расчётный срок первичного порта истёк"))
		} else {
			notes = append(notes, hudMuted("Срок первичного порта: "+Date(project["deadline"])))
		}
	}
	if len(doc.Sections) > 0 {
		label = ""
	}
	hudCard(doc, "context", "КОЛОНИЯ", label, 0, notes...)
}
