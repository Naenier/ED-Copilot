package engineering

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxOdysseyLoadouts = 40

var (
	odysseyEquipmentByKey = byKey(roadmapData.personalEquipment)
	odysseyGradeRecipes   = byKey(roadmapData.personalGrades)
	odysseyScopePattern   = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
	odysseyItemPattern    = regexp.MustCompile(`^(suit|weapon):[0-9]{1,20}$`)
)

type odysseyWorkspace struct {
	selected string
	loadouts []M
	progress map[string]M
}

func odysseyWorkspaceKey(context M) string {
	key := v.Str(context["planWorkspace"])
	if odysseyScopePattern.MatchString(key) {
		return key
	}
	return ""
}

func odysseyItemKey(symbol any) string {
	base, _, _ := strings.Cut(symbolText(symbol), "_class")
	return base
}

func odysseyWeaponSlot(item M) string {
	if item["moduleKey"] == "suit" {
		return "suit"
	}
	if strings.HasPrefix(v.Str(item["itemSymbol"]), "wpn_s_") {
		return "secondary"
	}
	return "primary"
}

func odysseyPrimarySlots(suit string) int {
	if suit == "tacticalsuit" {
		return 2
	}
	return 1
}

func odysseyModCompatible(model, mod M) bool {
	return mod != nil && model["moduleKey"] == mod["moduleKey"] &&
		(mod["damageType"] == nil || mod["damageType"] == model["damageType"])
}

func odysseySlots(model M, grade int) int {
	for g := grade; g >= 1; g-- {
		if row := v.Obj(v.At(model, "grades", strconv.Itoa(g))); len(row) > 0 {
			return v.Int(row["slots"])
		}
	}
	return 0
}

func validateOdysseyLoadout(raw M) (M, error) {
	name := strings.TrimSpace(v.Str(raw["name"]))
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return nil, errors.New("Укажите название комплекта длиной до 100 символов.")
	}
	rows := v.Rows(raw["items"])
	if len(rows) == 0 || len(rows) > 4 || len(rows) != len(v.Arr(raw["items"])) {
		return nil, errors.New("Добавьте от одного до четырёх предметов снаряжения.")
	}
	items := make([]M, 0, len(rows))
	bound := map[string]bool{}
	slots := map[string]int{}
	suit := ""
	for _, item := range rows {
		key := v.Str(item["itemKey"])
		model := odysseyEquipmentByKey[key]
		if model == nil {
			return nil, errors.New("Выберите модель костюма или оружия из справочника.")
		}
		equipmentKey := v.Str(item["equipmentKey"])
		if equipmentKey != "" && (!odysseyItemPattern.MatchString(equipmentKey) ||
			!strings.HasPrefix(equipmentKey, v.Str(model["moduleKey"])+":") || bound[equipmentKey]) {
			return nil, errors.New("Один экземпляр снаряжения можно включить в комплект только один раз.")
		}
		bound[equipmentKey] = true
		start, err := integer(v.First(item["startingGrade"], 1), "Исходный класс", 1, 5)
		if err != nil {
			return nil, err
		}
		target, err := integer(item["targetGrade"], "Целевой класс", 1, 5)
		if err != nil {
			return nil, err
		}
		if equipmentKey == "" && target < start {
			return nil, errors.New("Целевой класс ниже исходного.")
		}
		modKeys := []string{}
		mods := v.Arr(item["modKeys"])
		if len(mods) > odysseySlots(model, target) && equipmentKey == "" || len(mods) > 4 {
			return nil, errors.New("На целевом классе не хватает ячеек для выбранных модификаций.")
		}
		for _, rawMod := range mods {
			mod := v.Str(rawMod)
			if contains(modKeys, mod) || !odysseyModCompatible(model, roadmapData.personalRecipes[mod]) {
				return nil, errors.New("Выберите совместимые модификации без повторов.")
			}
			modKeys = append(modKeys, mod)
		}
		slot := odysseyWeaponSlot(model)
		slots[slot]++
		if slot == "suit" {
			suit = key
		}
		items = append(items, M{"itemKey": key, "equipmentKey": equipmentKey, "startingGrade": start, "targetGrade": target, "modKeys": modKeys})
	}
	primarySlots := 2
	if suit != "" {
		primarySlots = odysseyPrimarySlots(suit)
	}
	if slots["suit"] > 1 || slots["secondary"] > 1 || slots["primary"] > primarySlots {
		return nil, errors.New("В комплекте больше оружия, чем позволяет выбранный костюм.")
	}
	reserve := true
	if rawReserve, exists := raw["reserve"]; exists {
		var ok bool
		reserve, ok = rawReserve.(bool)
		if !ok {
			return nil, errors.New("Укажите, нужно ли резервировать материалы комплекта.")
		}
	}
	return M{"id": raw["id"], "name": name, "reserve": reserve, "items": items}, nil
}

// UpdateOdysseyLoadout edits the active commander's kit and returns the same
// projection as OdysseyLoadouts. Inventory snapshots never enter settings.
func (p *Planner) UpdateOdysseyLoadout(request, observations M) (M, error) {
	owner := odysseyWorkspaceKey(observations)
	if owner == "" {
		return nil, errors.New("Для комплекта Odyssey нужен командир из журнала игры.")
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	current := p.odysseyPlans[owner]
	if current == nil {
		if len(p.odysseyPlans) >= maxOdysseyWorkspaces {
			return nil, errors.New("Достигнут предел профилей с комплектами Odyssey.")
		}
		current = &odysseyWorkspace{loadouts: []M{}, progress: map[string]M{}}
	}
	id, index := v.Str(request["id"]), -1
	for i, kit := range current.loadouts {
		if kit["id"] == id {
			index = i
			break
		}
	}
	switch v.Str(request["action"]) {
	case "save":
		kit, err := validateOdysseyLoadout(request)
		if err != nil {
			return nil, err
		}
		if id != "" && index < 0 {
			return nil, errors.New("Комплект Odyssey не найден.")
		}
		if index < 0 && len(current.loadouts) >= maxOdysseyLoadouts {
			return nil, errors.New("Достигнут предел комплектов Odyssey.")
		}
		if err := p.checkOdysseyBindings(owner, current, kit, index); err != nil {
			return nil, err
		}
		if index < 0 {
			id = v.ID()
			kit["id"] = id
			current.loadouts = append(current.loadouts, kit)
		} else {
			current.loadouts[index] = kit
		}
		current.selected = id
		p.odysseyPlans[owner] = current
		p.refreshOdysseyProgress(owner, current)
	case "select":
		if index < 0 {
			return nil, errors.New("Комплект Odyssey не найден.")
		}
		current.selected = id
	case "remove":
		if index < 0 {
			return nil, errors.New("Комплект Odyssey не найден.")
		}
		current.loadouts = append(current.loadouts[:index], current.loadouts[index+1:]...)
		if current.selected == id {
			current.selected = ""
			if len(current.loadouts) > 0 {
				current.selected = v.Str(current.loadouts[min(index, len(current.loadouts)-1)]["id"])
			}
		}
		if len(current.loadouts) == 0 {
			delete(p.odysseyPlans, owner)
		} else {
			p.refreshOdysseyProgress(owner, current)
		}
	default:
		return nil, errors.New("Неизвестное действие с комплектом Odyssey.")
	}
	return p.odysseyProjection(observations, true), nil
}
