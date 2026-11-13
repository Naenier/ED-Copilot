package engineering

import (
	"errors"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/journal"
	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type shipWorkshop struct {
	key      string
	maxGrade int
}

var shipWorkshops = func() map[string]map[string][]shipWorkshop {
	index := map[string]map[string][]shipWorkshop{}
	for _, key := range roadmapData.engineerOrder {
		for _, cap := range roadmapData.capabilities[key] {
			if roadmapData.modules[cap.module]["kind"] != "ship" || cap.maxGrade == 0 {
				continue
			}
			if index[cap.module] == nil {
				index[cap.module] = map[string][]shipWorkshop{}
			}
			index[cap.module][cap.blueprint] = append(index[cap.module][cap.blueprint], shipWorkshop{key, cap.maxGrade})
		}
	}
	return index
}()

// CurrentShipEngineering uses a complete current Loadout, keeping equal items
// in different slots independent. Recipes are calculated only for the selected slot.
func (p *Planner) CurrentShipEngineering(loadout, input, observations M) (M, error) {
	q, err := parseRoadmapQuery(input)
	if err != nil {
		return nil, err
	}
	slot, slotOK := input["slot"].(string)
	if input["slot"] != nil && (!slotOK || len(slot) > 120) {
		return nil, errors.New("Некорректный слот модуля")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	known := v.Arr(loadout["Modules"]) != nil
	rows := []M{}
	summary := M{"complete": 0, "inProgress": 0, "available": 0, "locked": 0, "unknown": 0}
	result := M{"loadoutKnown": known, "ship": ships.DisplayShip(v.Str(loadout["Ship"]), v.Str(loadout["Ship_Localised"])),
		"shipName": v.Str(loadout["ShipName"]), "shipID": loadout["ShipID"], "observedAt": loadout["timestamp"], "modules": rows, "summary": summary}
	if !known {
		return result, nil
	}
	for _, installed := range p.currentShipModules(loadout, observations) {
		status := v.Str(installed.row["status"])
		if status == "in-progress" {
			status = "inProgress"
		}
		summary[status] = v.Int(summary[status]) + 1
		rows = append(rows, installed.row)
		if installed.row["slot"] == slot {
			detail, err := p.currentShipDetail(installed.row, installed.choices, installed.preferred, q, input, observations)
			if err != nil {
				return nil, err
			}
			result["detail"] = detail
		}
	}
	result["modules"] = rows
	return result, nil
}

type currentShipModule struct {
	row       M
	choices   []M
	preferred M
}

func (p *Planner) currentShipModules(loadout, observations M) []currentShipModule {
	legacy := journal.GameMode(v.Str(v.At(observations, "game", "version"))) == "legacy"
	rows := []currentShipModule{}
	seen := map[string]bool{}
	choicesByModule := map[string][]M{}
	for _, raw := range v.Rows(loadout["Modules"]) {
		currentSlot, symbol := v.Str(raw["Slot"]), symbolText(raw["Item"])
		if currentSlot == "" || len(currentSlot) > 120 || symbol == "" || len(symbol) > 160 || seen[currentSlot] {
			continue
		}
		seen[currentSlot] = true
		module := shipModule(symbol)
		installed := v.Obj(raw["Engineering"])
		choices, cached := choicesByModule[module]
		if !cached {
			choices = p.shipBlueprintChoices(module, legacy)
			choicesByModule[module] = choices
		}
		if len(choices) == 0 && len(installed) == 0 {
			continue
		}
		row := M{"id": currentSlot, "slot": currentSlot, "item": raw["Item"], "moduleKey": module, "symbol": symbol,
			"name":        v.Text(modules[module]["name"], raw["Item_Localised"], raw["Item"]),
			"english":     v.Text(roadmapData.modules[module]["english"], raw["Item_Localised"], raw["Item"]),
			"moduleClass": shipModuleClass(symbol), "effectKnown": true, "grade": 0}
		applyShipUpgrade(row, installed)
		bp, effect := v.Str(row["blueprintKey"]), v.Str(row["experimentalKey"])
		row["blueprintName"], row["blueprintEnglish"] = blueprints[bp]["name"], blueprints[bp]["english"]
		row["experimentalName"] = v.First(effects[effect]["name"], installed["ExperimentalEffect_Localised"])
		row["experimentalEnglish"] = v.First(effects[effect]["english"], installed["ExperimentalEffect_Localised"])
		current := shipBlueprintChoice(choices, bp)
		if len(installed) > 0 && current == nil {
			row["blueprintName"] = installed["BlueprintName"]
			row["blueprintEnglish"] = installed["BlueprintName"]
			row["unmappedBlueprint"] = v.Text(installed["BlueprintName"], installed["EngineerModifications"], "unknown")
		}
		if rawEffect := v.Str(installed["ExperimentalEffect"]); rawEffect != "" && effect == "" {
			row["experimentalName"] = v.Text(row["experimentalName"], rawEffect)
			row["experimentalEnglish"] = v.Text(row["experimentalEnglish"], rawEffect)
		}
		choice := current
		if choice == nil && len(choices) > 0 {
			choice = choices[0]
		}
		row["maxGrade"], row["accessibleGrade"] = choice["maxGrade"], choice["accessibleGrade"]
		row["status"] = currentShipStatus(row, installed, choices, current)
		rows = append(rows, currentShipModule{row, choices, choice})
	}
	return rows
}

func currentShipStatus(row, installed M, choices []M, current M) string {
	if len(installed) != 0 {
		grade, highest := v.Int(row["grade"]), v.Int(current["maxGrade"])
		quality, known := v.Number(row["quality"])
		switch {
		case current == nil || grade == 0 || grade > highest || grade == highest && !known:
			return "unknown"
		case grade == highest && quality == 1:
			return "complete"
		default:
			return "in-progress"
		}
	}
	unknown := false
	for _, choice := range choices {
		for _, engineer := range v.Rows(choice["engineers"]) {
			if engineer["status"] == "unlocked" {
				return "available"
			}
			unknown = unknown || engineer["status"] == "unknown"
		}
	}
	if unknown || len(choices) == 0 {
		return "unknown"
	}
	return "locked"
}

func (p *Planner) shipBlueprintChoices(module string, legacy bool) []M {
	rows := make([]M, 0, len(shipWorkshops[module]))
	for bp, workshops := range shipWorkshops[module] {
		blueprint := blueprints[bp]
		if legacy && v.Bool(blueprint["liveOnly"]) {
			continue
		}
		row := roadLabel(blueprint)
		engineers, maxGrade, accessible := []M{}, 0, 0
		unknownAccess, unlocked := false, false
		for _, workshop := range workshops {
			engineer := p.roadmapEngineerState(workshop.key)
			maxGrade = max(maxGrade, workshop.maxGrade)
			engineer["maxGrade"] = workshop.maxGrade
			switch {
			case engineer["status"] == "unlocked" && engineer["rank"] != nil:
				grade := min(v.Int(engineer["rank"]), workshop.maxGrade)
				engineer["accessibleGrade"] = grade
				accessible = max(accessible, grade)
				unlocked = true
			case engineer["status"] == "unlocked":
				unlocked, unknownAccess = true, true
			case engineer["status"] == "unknown":
				unknownAccess = true
			default:
				engineer["accessibleGrade"] = 0
			}
			engineers = append(engineers, engineer)
		}
		sort.SliceStable(engineers, func(i, j int) bool {
			return shipWorkshopScore(engineers[i]) > shipWorkshopScore(engineers[j])
		})
		row["engineers"], row["maxGrade"], row["unlocked"] = engineers, maxGrade, unlocked
		if accessible > 0 || !unknownAccess {
			row["accessibleGrade"] = accessible
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if v.Int(a["accessibleGrade"]) != v.Int(b["accessibleGrade"]) {
			return v.Int(a["accessibleGrade"]) > v.Int(b["accessibleGrade"])
		}
		if a["unlocked"] != b["unlocked"] {
			return v.Bool(a["unlocked"])
		}
		if a["maxGrade"] != b["maxGrade"] {
			return v.Int(a["maxGrade"]) > v.Int(b["maxGrade"])
		}
		return v.Str(a["english"]) < v.Str(b["english"])
	})
	return rows
}

func shipBlueprintChoice(choices []M, key string) M {
	for _, choice := range choices {
		if choice["key"] == key {
			return choice
		}
	}
	return nil
}

func shipWorkshopScore(engineer M) int {
	return recipeAccessScore(engineer)*100 + v.Int(engineer["accessibleGrade"])*10 + v.Int(engineer["maxGrade"])
}

func shipModuleClass(symbol string) string {
	_, size, found := strings.Cut(symbol, "_size")
	if !found {
		return ""
	}
	size, class, found := strings.Cut(size, "_class")
	if !found || len(size) != 1 || size[0] < '0' || size[0] > '9' || len(class) == 0 || class[0] < '1' || class[0] > '5' {
		return ""
	}
	return size + string('F'-class[0]+'0')
}
