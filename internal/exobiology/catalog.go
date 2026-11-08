// Package exobiology tracks biological exploration from journal observations.
package exobiology

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

type Species struct {
	ID, GenusID, Genus, Name, EnglishName, Description, EnglishDescription string
	MinSampleDistanceM, BaseCredits                                        any
	Conditions, Habitat, Colours                                           M
}

type Database struct {
	species      []*Species
	byID         map[string]*Species
	aliases      map[string]*Species
	genera       []M
	genusByID    map[string]M
	genusAliases map[string]string
	models       M
}

type Query struct {
	Query, Genus, Group, Category string
	Offset, Limit                 int
}

var catalogOnce sync.Once
var catalogData *Database

func Catalog() *Database {
	catalogOnce.Do(func() {
		raw := data.Object("exobiology-catalog.json")
		d := &Database{byID: map[string]*Species{}, aliases: map[string]*Species{}, genusByID: map[string]M{}, genusAliases: map[string]string{}, models: v.Obj(raw["models"])}
		for _, g := range v.Rows(raw["genera"]) {
			d.genera = append(d.genera, g)
			d.genusByID[v.Str(g["id"])] = g
		}
		for _, row := range v.Rows(raw["species"]) {
			s := &Species{ID: v.Str(row["id"]), GenusID: v.Str(row["genusId"]), Genus: v.Str(row["genus"]), Name: v.Str(row["name_ru"]), EnglishName: v.Str(row["name_en"]), Description: v.Str(row["description"]), EnglishDescription: v.Str(row["description_en"]), MinSampleDistanceM: row["minSampleDistanceM"], BaseCredits: row["baseCredits"], Conditions: v.Obj(row["conditions"]), Habitat: v.Obj(row["habitat"]), Colours: v.Obj(row["colours"])}
			d.species = append(d.species, s)
			d.byID[s.ID] = s
			for _, alias := range []string{s.ID, s.Name, s.EnglishName, strings.ReplaceAll(s.EnglishName, "Bacterium", "Bacteria")} {
				d.aliases[normal(alias)] = s
			}
		}
		sort.Slice(d.species, func(i, j int) bool { return d.species[i].EnglishName < d.species[j].EnglishName })
		for _, raw := range v.Arr(d.models["codexTypes"]) {
			row := v.Arr(raw)
			if len(row) < 2 {
				continue
			}
			sp := d.resolve(row[1])
			if sp == nil {
				continue
			}
			token := v.Str(row[0])
			d.aliases[normal(token)] = sp
			parts := strings.Split(token, "_")
			if len(parts) >= 5 {
				base := strings.Join(parts[:len(parts)-1], "_")
				d.aliases[normal(base)] = sp
				d.genusAliases[normal(parts[2])] = sp.GenusID
			}
		}
		catalogData = d
	})
	return catalogData
}

func normal(raw any) string {
	s := strings.ToLower(v.Str(raw))
	s = strings.ReplaceAll(s, "ё", "е")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func (d *Database) resolve(raw any) *Species {
	name := strings.TrimPrefix(v.Str(raw), "exo/")
	if s := d.byID[name]; s != nil {
		return s
	}
	if s := d.aliases[normal(name)]; s != nil {
		return s
	}
	if s := d.aliases[normal(strings.TrimSuffix(strings.TrimSuffix(name, ";"), "_Name"))]; s != nil {
		return s
	}
	for _, sep := range []string{" - ", " — ", " – "} {
		if i := strings.LastIndex(name, sep); i >= 0 {
			if s := d.aliases[normal(name[:i])]; s != nil {
				return s
			}
		}
	}
	return nil
}

func (d *Database) ResolveOrganic(event M) *Species {
	for _, key := range []string{"speciesId", "Species_Localised", "Variant_Localised", "name", "Species", "Variant"} {
		if s := d.resolve(event[key]); s != nil {
			return s
		}
	}
	return nil
}

func (s *Species) summary() M {
	return M{"id": "exo/" + s.ID, "speciesId": s.ID, "category": "exobiology", "group": s.GenusID, "genus": s.Genus, "genus_en": s.Genus, "genus_ru": Catalog().genusByID[s.GenusID]["name"], "title": s.Name, "name": s.Name, "name_ru": s.Name, "name_en": s.EnglishName, "subtitle": s.EnglishName, "summary": s.Description, "baseCredits": s.BaseCredits, "minSampleDistanceM": s.MinSampleDistanceM, "meta": []M{{"label": "Базовая выплата, кр.", "value": s.BaseCredits}, {"label": "Разнообразие образцов, м", "value": s.MinSampleDistanceM}}}
}

func (d *Database) List(q Query) M {
	limit := q.Limit
	if limit <= 0 {
		limit = 24
	}
	limit = min(limit, 48)
	offset := max(0, q.Offset)
	group := v.Text(q.Genus, q.Group)
	terms := strings.Fields(strings.ToLower(q.Query))
	rows := []M{}
	counts := map[string]int{}
	for _, s := range d.species {
		search := strings.ToLower(s.Name + " " + s.EnglishName + " " + s.Genus + " " + s.Description)
		match := true
		for _, term := range terms {
			if !strings.Contains(search, term) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		counts[s.GenusID]++
		if group != "" && group != "all" && group != s.GenusID && normal(group) != normal(s.Genus) {
			continue
		}
		rows = append(rows, s.summary())
	}
	groups := []M{}
	for _, g := range d.genera {
		if n := counts[v.Str(g["id"])]; n > 0 {
			groups = append(groups, M{"id": g["id"], "title": g["name"], "name_en": g["name_en"], "count": n})
		}
	}
	total := len(rows)
	offset = min(offset, total)
	return M{"categories": []M{{"id": "exobiology", "title": "Экзобиология", "count": len(d.species)}}, "groups": groups, "entries": rows[offset:min(offset+limit, total)], "total": total, "offset": offset, "limit": limit}
}

func (d *Database) Entry(id string) (M, bool) {
	s := d.resolve(id)
	if s == nil {
		return nil, false
	}
	out := s.summary()
	out["image"] = SpeciesImage(s.ID)
	out["description"], out["description_en"] = s.Description, s.EnglishDescription
	out["properties"] = []M{{"label": "Род", "value": d.genusByID[s.GenusID]["name"]}, {"label": "Базовая выплата, кр.", "value": s.BaseCredits}, {"label": "С первым открытием, до кр.", "value": multiply(s.BaseCredits, 5)}, {"label": "Разнообразие образцов, м", "value": s.MinSampleDistanceM}, {"label": "Наблюдений в профиле", "value": s.Habitat["sampleCount"]}}
	conditions := v.Copy(s.Conditions)
	displayConditions := v.Copy(conditions)
	gate := gateForSpecies(s.ID)
	if gate != nil {
		delete(displayConditions, "parent_star")
		delete(displayConditions, "parent_star_types")
	}
	items := conditionProperties(displayConditions)
	if gate != nil {
		scope := "Родительские звёзды"
		if gate.scope == "any" {
			scope = "Любая звезда системы"
		}
		if gate.scope == "main" {
			scope = "Главная или родительская звезда"
		}
		items = append(items, M{"label": "Наблюдаемые классы звёзд", "value": gate.allowed}, M{"label": "Проверка звёзд", "value": scope})
		for _, class := range sortedKeys(gate.luminosity) {
			allowed := gate.luminosity[class]
			value := strings.Join(allowed, ", ")
			if len(allowed) == 0 {
				value = "Не подтверждено"
			}
			items = append(items, M{"label": "Класс светимости: " + class, "value": value})
		}
	}
	sections := []M{{"id": "habitat", "title": "Условия обитания", "type": "facts", "items": items}}
	if notes := speciesNotes(s); len(notes) > 0 {
		sections = append(sections, M{"id": "condition-notes", "title": "Уточнение условий", "type": "prose", "paragraphs": notes})
	}
	if len(s.Habitat) > 0 {
		rows := [][]any{}
		for _, key := range sortedKeys(v.Obj(s.Habitat["numerics"])) {
			if strings.Contains(key, ".rings[") {
				continue
			}
			n := v.Obj(v.Obj(s.Habitat["numerics"])[key])
			scale := 1.0
			if key == "body.axialTilt" {
				scale = 180 / math.Pi
			}
			rows = append(rows, []any{parameterName(key), multiply(n["min"], scale), multiply(n["max"], scale), multiply(n["mode"], scale), n["count"]})
		}
		sections = append(sections, M{"id": "measured", "title": "Измеренная среда обитания", "type": "table", "columns": []string{"Параметр", "Минимум", "Максимум", "Типичное", "Наблюдений"}, "rows": rows})
	}
	variants := d.variants(s)
	if len(variants) > 0 {
		sections = append(sections, M{"id": "variants", "title": "Цветовые варианты", "type": "facts", "items": variants})
	}
	sections = append(sections, M{"id": "valuation", "title": "Выплата и подтверждение", "type": "prose", "paragraphs": []string{"Базовая выплата относится к полному набору генетических образцов. Бонус первого открытия зависит от состояния тела и продажи; прогноз не является гарантированной выплатой. Точная выручка берётся из события продажи Vista Genomics."}})
	out["sections"], out["conditions"], out["colourRules"] = sections, conditions, v.Copy(s.Colours)
	out["habitat"] = v.Copy(s.Habitat)
	related := []M{}
	for _, other := range d.species {
		if other.GenusID == s.GenusID && other.ID != s.ID {
			related = append(related, M{"id": "exo/" + other.ID, "category": "exobiology", "title": other.Name, "name_en": other.EnglishName})
			if len(related) >= 12 {
				break
			}
		}
	}
	out["related"] = related
	return out, true
}

func stringsOf(raw any) []string {
	if s, ok := raw.(string); ok && s != "" {
		return []string{s}
	}
	out := []string{}
	for _, x := range v.Arr(raw) {
		if s := v.Str(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}
func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func multiply(raw any, factor float64) any {
	if n, ok := v.Number(raw); ok {
		return n * factor
	}
	return nil
}

func conditionProperties(c M) []M {
	labels := map[string]string{"atmosphere": "Атмосфера", "planet_types": "Типы тел", "max_gravity": "Максимальная гравитация, g", "temperature_K": "Температура, K", "volcanism": "Вулканизм", "parent_star": "Родительская звезда", "parent_star_types": "Типы родительских звёзд", "atmospherePressureCategory": "Класс атмосферы", "required_atmosphere_type": "Необходимый газ", "atmosphere_gas_share_pct": "Доли газов, %", "gravity_constraints": "Ограничения гравитации", "location_requirement": "Расположение", "system_requirements": "Другие тела системы", "requires_system_bodies": "Условия системы", "min_sample_distance_m": "Разнообразие образцов, м", "known_systems": "Известные системы", "distance_from_star": "Расстояние от родительской звезды", "distance_from_arrival": "Расстояние от точки прибытия", "minPressure": "Минимальное давление, атм", "volcanismActiveRequired": "Нужен активный вулканизм", "all_conditions_required": "Требуется подтверждение всех условий", "presence_any_of": "Альтернативные условия", "soft_no_volcanism": "Преимущественно без вулканизма"}
	out := []M{}
	for _, k := range sortedKeys(c) {
		if label := labels[k]; label != "" {
			if c[k] != nil && k != "location_requirement" {
				out = append(out, M{"label": label, "value": readableRule(c[k])})
			}
		}
	}
	return out
}

func parameterName(key string) string {
	if label := map[string]string{"body.argOfPeriapsis": "Аргумент периастра, °", "body.ascendingNode": "Долгота восходящего узла, °", "body.meanAnomaly": "Средняя аномалия, °", "body.rotationalPeriod": "Вращение, сут."}[key]; label != "" {
		return label
	}
	if strings.HasPrefix(key, "body.materials.") {
		name := strings.TrimPrefix(key, "body.materials.")
		for _, material := range v.Rows(data.Object("engineering-data.json")["materials"]) {
			if strings.EqualFold(v.Str(material["english"]), name) {
				name = v.Str(material["name"])
				break
			}
		}
		return "Материал: " + name + ", %"
	}
	if strings.HasPrefix(key, "body.atmosphereComposition.") {
		return "Газ атмосферы: " + readableRule(strings.TrimPrefix(key, "body.atmosphereComposition.")) + ", %"
	}
	if strings.HasPrefix(key, "body.solidComposition.") {
		name := strings.TrimPrefix(key, "body.solidComposition.")
		name = mapToken(name, map[string]string{"Rock": "Камень", "Ice": "Лёд", "Metal": "Металл"})
		return "Состав поверхности: " + name + ", %"
	}
	labels := map[string]string{"body.surfaceTemperature": "Температура, K", "body.gravity": "Гравитация, g", "body.surfacePressure": "Давление, атм", "body.radius": "Радиус, км", "body.distanceToArrival": "От точки прибытия, св. с.", "body.earthMasses": "Масса, земных", "body.rotationalPeriod": "Вращение, ч", "body.orbitalPeriod": "Орбитальный период, сут.", "body.semiMajorAxis": "Большая полуось, а.е.", "body.axialTilt": "Наклон оси, °", "body.orbitalEccentricity": "Эксцентриситет", "body.orbitalInclination": "Наклон орбиты, °"}
	if label := labels[key]; label != "" {
		return label
	}
	return strings.TrimPrefix(key, "body.")
}

func (d *Database) variants(s *Species) []M {
	out := []M{}
	mapping := v.Obj(v.Obj(v.Obj(d.models["colourVariants"])["bySpecies"])[strings.ToLower(s.EnglishName)])
	basis := v.Str(mapping["source"])
	mapping = v.Obj(mapping["map"])
	if len(mapping) == 0 {
		mapping = v.Obj(s.Colours["mapping"])
	}
	if len(mapping) == 0 {
		mapping = v.Obj(v.Obj(d.genusByID[s.GenusID]["colours"])["mapping"])
	}
	for _, k := range sortedKeys(mapping) {
		if mapping[k] != nil {
			out = append(out, M{"label": readableRule(k), "value": readableRule(mapping[k]), "basis": basis})
		}
	}
	return out
}

func readableRule(raw any) string {
	if raw == nil {
		return "—"
	}
	if b, ok := raw.(bool); ok {
		if b {
			return "Да"
		}
		return "Нет"
	}
	if a := v.Arr(raw); a != nil {
		if len(a) == 2 {
			if _, ok := v.Number(a[0]); ok {
				return readableRange(a[0], a[1])
			}
		}
		parts := []string{}
		for _, x := range a {
			parts = append(parts, readableRule(x))
		}
		return strings.Join(parts, ", ")
	}
	if object := v.Obj(raw); len(object) > 0 {
		if object["min"] != nil || object["max"] != nil {
			return readableRange(object["min"], object["max"])
		}
		parts := []string{}
		for _, key := range sortedKeys(object) {
			if strings.Contains(strings.ToLower(key), "note") || key == "possible_types_unconfirmed" || key == "soft_below" {
				continue
			}
			label := map[string]string{"min_ls": "Минимум, св. с.", "max_ls": "Максимум, св. с.", "approx_AU": "Примерно, а.е.", "required_types": "Подтверждённые типы", "volcanismActiveRequired": "Активный вулканизм", "temperature_K": "Температура, K"}[key]
			if label == "" {
				label = readableRule(key)
			}
			parts = append(parts, label+": "+readableRule(object[key]))
		}
		return strings.Join(parts, "; ")
	}
	text := v.Str(raw)
	labels := map[string]string{"Rocky": "Каменистое", "Rocky body": "Каменистое тело", "High Metal Content": "С высоким содержанием металлов", "High metal content body": "Тело с высоким содержанием металлов", "Icy": "Ледяное", "Rocky Ice": "Каменисто-ледяное", "Metal Rich": "Богатое металлами", "None": "Отсутствует", "thin": "Тонкая", "Carbon Dioxide": "Диоксид углерода", "Sulphur Dioxide": "Диоксид серы", "Ammonia": "Аммиак", "Water": "Вода", "Oxygen": "Кислород", "Argon": "Аргон", "Argon-rich": "Богатая аргоном", "Neon": "Неон", "Nitrogen": "Азот", "Methane": "Метан", "Any thin atmosphere": "Любая тонкая атмосфера", "Near nebulae": "Вблизи туманностей", "A or B, any star in the system": "Звезда A или B в системе", "A, F, G, K, M, S or a brown dwarf (hostStarGates.ts)": "A, F, G, K, M, S или коричневый карлик"}
	if label := labels[text]; label != "" {
		return label
	}
	if strings.HasSuffix(text, " (thin)") {
		return readableRule(strings.TrimSuffix(text, " (thin)")) + " (тонкая)"
	}
	return text
}
func readableRange(lo, hi any) string {
	lower, lok := v.Number(lo)
	upper, hok := v.Number(hi)
	if upper == 999 {
		hok = false
	}
	if lok && hok {
		return fmt.Sprintf("%g–%g", lower, upper)
	}
	if lok {
		return fmt.Sprintf("≥ %g", lower)
	}
	if hok {
		return fmt.Sprintf("≤ %g", upper)
	}
	return "Не определено"
}
func speciesNotes(s *Species) []string {
	out := []string{"Условия и измеренные диапазоны помогают выбрать место поиска. Совпадение параметров не подтверждает присутствие вида; неизвестный параметр требует дополнительного сканирования."}
	c := s.Conditions
	if len(v.Rows(c["presence_any_of"])) > 0 {
		parts := []string{}
		for _, r := range v.Rows(c["presence_any_of"]) {
			parts = append(parts, readableRule(r))
		}
		out = append(out, "Достаточно одного альтернативного условия: "+strings.Join(parts, " ИЛИ ")+".")
	}
	if c["whenAtmosphereLinkedMinTempK"] != nil || c["whenAtmosphereLinkedMaxTempK"] != nil {
		gases := v.First(c["whenAtmosphereLinkedAtmosphereAnyOf"], []string{"Carbon Dioxide"})
		out = append(out, "Для атмосферы «"+readableRule(gases)+"» температурный диапазон: "+readableRange(c["whenAtmosphereLinkedMinTempK"], c["whenAtmosphereLinkedMaxTempK"])+" K; это ограничение не распространяется на остальные разрешённые атмосферы.")
	}
	if c["volcanic_only_atmospheres"] != nil {
		out = append(out, "В атмосферах «"+readableRule(c["volcanic_only_atmospheres"])+"» требуется активный вулканизм.")
	}
	if c["off_list_atmosphere_needs_volcanism"] == true {
		out = append(out, "Наблюдения вне основного списка атмосфер допускаются при активном вулканизме и отмечаются как менее определённые.")
	}
	switch s.GenusID {
	case "bark_mounds":
		out = append(out, "Поиск сосредоточен вблизи туманностей: основной диапазон до 150 св. лет, расширенный до 300 св. лет. Более далёкие места отмечаются как маловероятные.")
	case "brain_trees":
		out = append(out, "Поиск связан с известными областями Стражей; ориентир — до 700 св. лет от известных мест, расширенный диапазон — до 1000 св. лет. Каталог известных мест не исчерпывает Галактику.")
	case "sinuous_tuber":
		out = append(out, "Основная область поиска — до 21 000 св. лет от Sagittarius A*. За пределами области прогноз снижается, а не считается доказательством отсутствия.")
	case "crystalline_shards":
		out = append(out, "Известные скопления преимущественно находятся не ближе 19 500 св. лет от Sagittarius A*. Проверяйте также расстояние от точки прибытия и тип родительской звезды.")
	}
	return out
}
