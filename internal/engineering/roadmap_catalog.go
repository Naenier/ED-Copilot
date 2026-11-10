package engineering

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type roadmapCapability struct {
	module, blueprint string
	maxGrade          int
}

type roadmapCatalogue struct {
	engineers, modules, personalRecipes, personalMaterials map[string]M
	engineerIDs, blueprintKeys, effectKeys                 map[string]string
	moduleSymbols                                          map[string]string
	effectExclusions                                       map[string]M
	capabilities                                           map[string][]roadmapCapability
	moduleOrder, engineerOrder                             []string
	personalGrades                                         []M
	personalEquipment                                      []M
	specialShipRecipes                                     []M
	verifiedAt                                             any
}

var roadmapData = newRoadmapCatalogue()

func newRoadmapCatalogue() roadmapCatalogue {
	r := roadmapCatalogue{
		engineers: map[string]M{}, modules: map[string]M{}, personalRecipes: map[string]M{}, personalMaterials: map[string]M{},
		engineerIDs: map[string]string{}, blueprintKeys: map[string]string{}, effectKeys: map[string]string{},
		capabilities: map[string][]roadmapCapability{}, moduleSymbols: map[string]string{}, effectExclusions: map[string]M{},
	}
	// The supplementary dataset enriches the existing ship catalogue without
	// changing the historical keys used by saved material plans.
	for _, raw := range v.Rows(catalog["engineers"]) {
		key := v.Str(raw["key"])
		r.engineers[key] = v.Merge(v.Copy(raw), M{"kind": "ship"})
	}
	raw, err := data.Load("engineering-roadmap.json")
	if err == nil {
		extension := v.Obj(raw)
		r.verifiedAt = extension["verifiedAt"]
		for _, row := range v.Rows(extension["engineers"]) {
			key := v.Str(row["key"])
			r.engineers[key] = v.Merge(v.Copy(r.engineers[key]), row)
		}
		r.personalMaterials = byKey(v.Rows(extension["odysseyMaterials"]))
		r.personalRecipes = byKey(v.Rows(extension["odysseyRecipes"]))
		r.personalGrades = v.Rows(extension["odysseyGradeRecipes"])
		r.personalEquipment = v.Rows(extension["odysseyEquipment"])
		r.specialShipRecipes = v.Rows(extension["specialShipRecipes"])
	}
	for key, row := range r.engineers {
		for _, alias := range append(v.Arr(row["aliases"]), key, row["journalId"], row["english"], row["name"]) {
			if normalized := v.Key(alias); normalized != "" {
				r.engineerIDs[normalized] = key
			}
		}
		r.engineerOrder = append(r.engineerOrder, key)
	}
	sort.Slice(r.engineerOrder, func(i, j int) bool {
		a, b := r.engineers[r.engineerOrder[i]], r.engineers[r.engineerOrder[j]]
		if a["kind"] != b["kind"] {
			return a["kind"] == "ship"
		}
		return v.Str(a["english"]) < v.Str(b["english"])
	})
	for key := range blueprints {
		r.blueprintKeys[v.Key(key)] = key
	}
	for key := range effects {
		r.effectKeys[v.Key(key)] = key
	}
	for symbol, exclusions := range v.Obj(catalog["journalEffectExclusions"]) {
		keys := M{}
		for _, key := range v.Arr(exclusions) {
			keys[v.Str(key)] = true
		}
		r.effectExclusions[symbolText(symbol)] = keys
	}
	for _, raw := range v.Rows(catalog["modules"]) {
		key := v.Str(raw["key"])
		english := v.Text(raw["english"], localization.T("en", v.Str(raw["name"])))
		r.modules[key] = M{"key": key, "name": raw["name"], "english": english, "kind": "ship"}
		for _, symbol := range v.Arr(raw["journalSymbols"]) {
			r.moduleSymbols[symbolText(symbol)] = key
		}
		for blueprint, grades := range v.Obj(raw["engineers"]) {
			maxGrades := map[string]int{}
			for grade, names := range v.Obj(grades) {
				for _, name := range v.Arr(names) {
					if engineer := r.engineerIDs[v.Key(name)]; engineer != "" {
						maxGrades[engineer] = max(maxGrades[engineer], v.Int(grade))
					}
				}
			}
			for engineer, grade := range maxGrades {
				r.capabilities[engineer] = append(r.capabilities[engineer], roadmapCapability{key, blueprint, grade})
			}
		}
	}
	if len(r.personalRecipes) > 0 {
		r.modules["suit"] = M{"key": "suit", "name": "Костюмы", "english": "Suits", "kind": "odyssey"}
		r.modules["weapon"] = M{"key": "weapon", "name": "Личное оружие", "english": "Personal weapons", "kind": "odyssey"}
	}
	for key, row := range r.personalRecipes {
		for _, engineer := range v.Arr(row["engineerKeys"]) {
			id := v.Str(engineer)
			r.capabilities[id] = append(r.capabilities[id], roadmapCapability{v.Str(row["moduleKey"]), key, 1})
		}
	}
	for key := range r.modules {
		r.moduleOrder = append(r.moduleOrder, key)
	}
	sort.Slice(r.moduleOrder, func(i, j int) bool {
		return v.Str(r.modules[r.moduleOrder[i]]["english"]) < v.Str(r.modules[r.moduleOrder[j]]["english"])
	})
	for _, caps := range r.capabilities {
		sort.Slice(caps, func(i, j int) bool {
			if caps[i].module != caps[j].module {
				return v.Str(r.modules[caps[i].module]["english"]) < v.Str(r.modules[caps[j].module]["english"])
			}
			a, b := r.blueprint(caps[i].blueprint), r.blueprint(caps[j].blueprint)
			return v.Str(a["english"])+caps[i].blueprint < v.Str(b["english"])+caps[j].blueprint
		})
	}
	return r
}

func (r roadmapCatalogue) blueprint(key string) M {
	if row := blueprints[key]; row != nil {
		return row
	}
	return r.personalRecipes[key]
}

func roadmapEngineerKey(id, name any) string {
	if key := roadmapData.engineerIDs[v.Key(id)]; key != "" {
		return key
	}
	return roadmapData.engineerIDs[v.Key(name)]
}

func journalUpgradeKey(module, field, normalized string, catalogue map[string]M, fallback map[string]string) string {
	for _, raw := range v.Arr(modules[module][field]) {
		key := v.Str(raw)
		if v.Key(key) == normalized || v.Key(catalogue[key]["journalSymbol"]) == normalized {
			return key
		}
	}
	return fallback[normalized]
}

type roadmapQuery struct {
	engineers                            map[string]bool
	module, blueprint, effect, equipment string
	grade                                int
}

func parseRoadmapQuery(body M) (roadmapQuery, error) {
	q := roadmapQuery{engineers: map[string]bool{}}
	if raw, exists := body["engineerIDs"]; exists {
		values := v.Arr(raw)
		if values == nil || len(values) > 64 {
			return q, fmt.Errorf("Некорректный список инженеров")
		}
		for _, value := range values {
			key, ok := value.(string)
			if !ok || roadmapData.engineers[key] == nil {
				return q, fmt.Errorf("Инженер не найден")
			}
			q.engineers[key] = true
		}
	}
	fields := []struct {
		key string
		dst *string
	}{{"moduleKey", &q.module}, {"blueprintKey", &q.blueprint}, {"experimentalKey", &q.effect}, {"equipmentKey", &q.equipment}}
	for _, field := range fields {
		if raw, exists := body[field.key]; exists {
			value, ok := raw.(string)
			if !ok || len(value) > 200 {
				return q, fmt.Errorf("Некорректный фильтр инженерии")
			}
			*field.dst = value
		}
	}
	if q.module != "" && roadmapData.modules[q.module] == nil {
		return q, fmt.Errorf("Модуль не найден")
	}
	if q.blueprint != "" && roadmapData.blueprint(q.blueprint) == nil {
		return q, fmt.Errorf("Чертёж не найден")
	}
	if q.effect != "" && effects[q.effect] == nil {
		return q, fmt.Errorf("Экспериментальный эффект не найден")
	}
	if raw, exists := body["grade"]; exists {
		grade, err := integer(raw, "Класс улучшения", 0, 5)
		if err != nil {
			return q, err
		}
		q.grade = grade
	}
	return q, nil
}

func (q roadmapQuery) capabilityMatches(cap roadmapCapability) bool {
	return (q.module == "" || cap.module == q.module) &&
		(q.blueprint == "" || cap.blueprint == q.blueprint) &&
		(q.grade == 0 || cap.maxGrade >= q.grade) &&
		(q.effect == "" || contains(modules[cap.module]["experimentals"], q.effect))
}

func roadLabel(row M) M {
	return M{"key": row["key"], "name": row["name"], "english": v.Text(row["english"], row["name"])}
}

func symbolText(raw any) string {
	return strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v.Str(raw)), "$")), ";"), "_name")
}
