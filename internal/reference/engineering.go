package reference

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var moduleAliases = map[string]string{"bsg": "sg", "psg": "sg", "advmc": "mc", "amr": "mr"}
var engineerImages = map[string]string{"300000": "didi_vatermann", "300010": "bill_turner", "300030": "broo_tarquin", "300040": "the_sarge", "300050": "zacariah_nemo", "300080": "liz_ryder2", "300090": "hera_tani", "300100": "felicity_farseer", "300110": "ram_tah", "300120": "lei_cheung", "300130": "petra_olmanova", "300140": "bris_dekker", "300150": "marsha_hicks", "300160": "elvira_martuuk", "300180": "the_dweller", "300200": "marco_qwent", "300210": "selene_jean", "300220": "prof_palin", "300230": "lori_jameson", "300250": "juri_ishmaak", "300260": "tod_mcquinn", "300270": "tiana_fortune", "300280": "mel_brandon", "300290": "etienne_dorn", "300300": "chloe_sedesi"}

func canonicalModule(key string) string {
	if canonical := moduleAliases[key]; canonical != "" {
		return canonical
	}
	return key
}

func blueprintID(raw v.M) string { return "blueprint/" + slug(v.Text(raw["english"], raw["name"])) }
func experimentalID(raw v.M) string {
	return "experimental/" + slug(v.Text(raw["english"], raw["name"]))
}

func (s *Store) buildEngineering() {
	c := data.Object("engineering-data.json")
	stats := data.Object("encyclopedia-engineering-stats.json")
	effectStats := data.Object("encyclopedia-experimental-stats.json")
	mods := v.Rows(c["modules"])
	engineers := v.Rows(c["engineers"])
	materials := map[string]v.M{}
	bps, effects := map[string]v.M{}, map[string]v.M{}
	for _, m := range v.Rows(c["materials"]) {
		materials[v.Str(m["key"])] = m
		s.addMaterial(m)
	}
	for _, m := range v.Rows(c["blueprints"]) {
		bps[v.Str(m["key"])] = m
	}
	for _, m := range v.Rows(c["experimentals"]) {
		effects[v.Str(m["key"])] = m
	}
	s.buildModules(mods, engineers, bps, effects)
	for _, raw := range v.Rows(c["blueprints"]) {
		id := blueprintID(raw)
		s.aliases["blueprint/"+v.Str(raw["key"])], s.aliases["blueprints/"+v.Str(raw["key"])] = id, id
		r := s.records[id]
		if r == nil {
			r = s.add(Entry{ID: id, Category: "blueprints", Group: "ship-blueprints", Title: v.Str(raw["name"]), Subtitle: v.Str(raw["english"]), Description: blueprintDescription(v.Str(raw["english"]))}, v.Str(raw["english"]))
		}
		moduleNames := []string{}
		for _, m := range mods {
			if hasKey(m["blueprints"], v.Str(raw["key"])) {
				moduleNames = append(moduleNames, v.Str(m["name"]))
				r.relatedIDs = append(r.relatedIDs, "module/"+canonicalModule(v.Str(m["key"])))
			}
		}
		grades := []Grade{}
		for grade := 1; grade <= 5; grade++ {
			recipe := v.Rows(v.Obj(raw["grades"])[fmt.Sprint(grade)])
			if len(recipe) == 0 {
				continue
			}
			ingredients := recipeIngredients(recipe, materials)
			modifiers := blueprintModifiers(stats, v.Str(raw["key"]), grade)
			if len(modifiers) == 0 {
				modifiers = catalogueModifiers(raw, grade)
			}
			grades = append(grades, Grade{Grade: grade, Title: fmt.Sprintf("Класс G%d: один ролл", grade), Materials: ingredients, Effects: modifiers})
			for _, item := range ingredients {
				if mat := s.records[item.EntryID]; mat != nil {
					mat.relatedIDs = append(mat.relatedIDs, id)
				}
				r.relatedIDs = append(r.relatedIDs, item.EntryID)
			}
		}
		r.entry.Sections = append(r.entry.Sections, Section{ID: "recipe-" + v.Str(raw["key"]), Title: strings.Join(unique(moduleNames), " · "), Type: "grades", Grades: grades})
	}
	for _, raw := range v.Rows(c["experimentals"]) {
		id := experimentalID(raw)
		s.aliases["experimental/"+v.Str(raw["key"])], s.aliases["experimentals/"+v.Str(raw["key"])] = id, id
		r := s.records[id]
		if r == nil {
			r = s.add(Entry{ID: id, Category: "experimentals", Group: "ship-effects", Title: v.Str(raw["name"]), Subtitle: v.Str(raw["english"]), Description: experimentalDescription(v.Str(raw["english"]))}, v.Str(raw["english"]))
		}
		names := []string{}
		for _, m := range mods {
			if hasKey(m["experimentals"], v.Str(raw["key"])) {
				names = append(names, v.Str(m["name"]))
				r.relatedIDs = append(r.relatedIDs, "module/"+canonicalModule(v.Str(m["key"])))
			}
		}
		ingredients := recipeIngredients(v.Rows(raw["materials"]), materials)
		r.entry.Sections = append(r.entry.Sections, Section{ID: "recipe-" + v.Str(raw["key"]), Title: strings.Join(unique(names), " · "), Type: "grades", Grades: []Grade{{Title: "Установка эффекта", Materials: ingredients, Effects: experimentalModifiers(effectStats, v.Str(raw["key"]))}}})
		for _, item := range ingredients {
			r.relatedIDs = append(r.relatedIDs, item.EntryID)
			if mat := s.records[item.EntryID]; mat != nil {
				mat.relatedIDs = append(mat.relatedIDs, id)
			}
		}
	}
	s.buildEngineers(engineers, mods, bps)
	s.buildRecipeWorkshops(mods, engineers, bps, effects)
	s.groupNames["ship-blueprints"], s.groupNames["ship-effects"] = "Корабельные чертежи", "Корабельные эффекты"
}

func recipeIngredients(rows []v.M, materials map[string]v.M) []Ingredient {
	out := []Ingredient{}
	for _, item := range rows {
		key := v.Str(item["key"])
		out = append(out, Ingredient{Name: v.Text(materials[key]["name"], key), Count: v.Int(item["count"]), EntryID: "material/" + key})
	}
	return out
}

func (s *Store) addMaterial(m v.M) {
	e, groupTitle := materialEntry(m)
	s.add(e, v.Str(m["symbol"]), e.EnglishTitle)
	s.groupNames[e.Group] = groupTitle
}

func moduleGroup(key string) string {
	if strings.Contains(" bl c fc mc mr ml nl pa pl rg tp ul guardian-gauss guardian-plasma guardian-shard ", " "+key+" ") {
		return "module-weapons"
	}
	if strings.Contains(" fsd t pp pd ls s bh guardian-pp guardian-pd ", " "+key+" ") {
		return "module-core"
	}
	if strings.Contains(" ch csl ec hs kw po sb ws cs ", " "+key+" ") {
		return "module-utility"
	}
	return "module-optional"
}

func (s *Store) groupTitle(group string) string {
	title := map[string]string{"module-weapons": "Вооружение", "module-core": "Основные модули", "module-utility": "Служебные устройства", "module-optional": "Дополнительные модули"}[group]
	s.groupNames[group] = title
	return title
}
func hasKey(values any, key string) bool {
	for _, v0 := range v.Arr(values) {
		if v.Str(v0) == key {
			return true
		}
	}
	return false
}
func unique(values []string) []string {
	set := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !set[value] {
			set[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func blueprintDescription(name string) string {
	if text := lookupDescription(blueprintDescriptions, name); text != "" {
		return text
	}
	if text := map[string]string{"Increased range": "Увеличивает дальность гиперпрыжка через параметры FSD. Конкретный класс улучшения меняет результат и сопутствующие характеристики.", "Lightweight": "Уменьшает массу совместимого модуля. Побочные изменения прочности и энергопотребления зависят от семейства модуля.", "Heavy duty": "Усиливает защитные характеристики совместимого модуля. Масса и итоговая защита рассчитываются для конкретного исполнения.", "Dirty": "Повышает рабочие возможности двигателей ценой сопутствующих ограничений и тепловой нагрузки.", "Clean": "Меняет характеристики двигателей с акцентом на более холодную работу. Результат зависит от массы корабля и выбранного класса.", "Overcharged": "Повышает основной выходной параметр: у оружия — урон, у силовой установки — выработку. Побочные эффекты и рецепты зависят от типа модуля.", "Long range": "Увеличивает рабочую дальность совместимого оборудования. Для оружия, сканеров и других устройств применяются разные рецепты и изменения характеристик.", "Expanded Probe Scanning Radius": "Увеличивает область покрытия зонда подробного сканера поверхности. Помогает картографировать тело меньшим количеством зондов."}[name]; text != "" {
		return text
	}
	return ""
}

func experimentalDescription(name string) string {
	if text := lookupDescription(effectDescriptions, name); text != "" {
		return text
	}
	if text := map[string]string{"Mass Manager": "Увеличивает оптимальную массу FSD и применяется как отдельный эффект к совместимому инженерному чертежу.", "Deep Charge": "Меняет допустимый расход топлива FSD за прыжок. Выгоду нужно оценивать вместе с классом двигателя и требуемым запасом топлива.", "Corrosive Shell": "Добавляет коррозионное воздействие при попадании совместимого оружия. Эффект и боезапас учитываются отдельно от основного чертежа.", "Thermal Vent": "Меняет тепловое поведение лучевого лазера при попадании и промахе. Работа эффекта зависит от удержания луча на цели.", "Stripped Down": "Дополнительно уменьшает массу совместимого модуля. Рецепт зависит от устройства, на которое устанавливается эффект."}[name]; text != "" {
		return text
	}
	return ""
}
