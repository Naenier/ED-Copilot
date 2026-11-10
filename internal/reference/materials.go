package reference

import (
	"fmt"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/engineering"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func materialEntry(m v.M) (Entry, string) {
	category := v.Str(m["category"])
	categoryTitle := map[string]string{"raw": "Сырьё", "manufactured": "Произведённые материалы", "encoded": "Данные", "currency": "Валюта"}[category]
	e := Entry{
		ID: "material/" + v.Str(m["key"]), Category: "materials", Group: "material-" + category,
		Title: v.Str(m["name"]), EnglishTitle: v.Str(m["english"]), Subtitle: v.Str(m["english"]),
		Properties: []Property{{Label: "Категория", Value: categoryTitle}},
	}
	if grade := v.Int(m["grade"]); grade > 0 {
		label := fmt.Sprintf("G%d", grade)
		e.Properties = append(e.Properties, Property{Label: "Класс", Value: label})
		e.Badges = []Badge{{Label: label}}
	}
	if capacity := v.Int(m["capacity"]); capacity > 0 && category != "currency" {
		e.Properties = append(e.Properties, Property{Label: "Лимит хранения", Value: fmt.Sprint(capacity)})
	}
	exchange := "Недоступен"
	if v.Bool(m["tradeable"]) {
		exchange = "Доступен"
	}
	e.Properties = append(e.Properties, Property{Label: "Обмен у торговца", Value: exchange})
	return e, categoryTitle
}

type materialFamily struct {
	category, name string
}

func (s *Store) addShipMaterialAcquisition() {
	rows := v.Rows(data.Object("engineering-data.json")["materials"])
	highest := map[materialFamily]v.M{}
	byKey := map[string]v.M{}
	for _, row := range rows {
		byKey[v.Str(row["key"])] = row
		if !v.Bool(row["tradeable"]) {
			continue
		}
		family := familyOf(row)
		if v.Int(row["grade"]) > v.Int(highest[family]["grade"]) {
			highest[family] = row
		}
	}
	for _, row := range rows {
		key := v.Str(row["key"])
		rec := s.records["material/"+key]
		if rec == nil {
			continue
		}
		e := &rec.entry
		if v.Bool(row["tradeable"]) {
			top := highest[familyOf(row)]
			if top["key"] != row["key"] {
				s.addMaterialExchange(e, row, top, false)
			} else if alternative := materialExchangeFallback[key]; alternative != "" {
				s.addMaterialExchange(e, row, byKey[alternative], true)
			}
		}
		addMaterialCollection(e, s.shipMaterialSources(key))
	}
}

func familyOf(row v.M) materialFamily {
	return materialFamily{v.Str(row["category"]), fmt.Sprint(row["family"])}
}

func (s *Store) addMaterialExchange(e *Entry, target, payment v.M, otherFamily bool) {
	ratio := engineering.TradeRatio(payment, target)
	if ratio == nil {
		return
	}
	id := "material/" + v.Str(payment["key"])
	rec := s.records[id]
	if rec == nil {
		return
	}
	kind := v.Str(target["category"])
	trader := map[string]string{"raw": "у торговца сырьём", "manufactured": "у торговца произведёнными материалами", "encoded": "у торговца данными"}[kind]
	traderEN := map[string]string{"raw": "at a Raw Material Trader", "manufactured": "at a Manufactured Material Trader", "encoded": "at an Encoded Material Trader"}[kind]
	cost, gain := v.Int(ratio["cost"]), v.Int(ratio["gain"])
	section := Section{ID: "trade", Type: "links", Title: "Обмен внутри семейства", EnglishTitle: "Same-family exchange"}
	if otherFamily {
		section.Title, section.EnglishTitle = "Обмен из другого семейства", "Cross-family exchange"
	}
	section.Items = []Item{{
		Title:        fmt.Sprintf("%s · G%d", rec.entry.Title, v.Int(payment["grade"])),
		EnglishTitle: fmt.Sprintf("%s · G%d", rec.entry.EnglishTitle, v.Int(payment["grade"])),
		EntryID:      id,
		Text:         fmt.Sprintf("Отдайте %d и получите %d %s.", cost, gain, trader),
		EnglishText:  fmt.Sprintf("Trade %d for %d %s.", cost, gain, traderEN),
	}}
	e.Sections = append(e.Sections, section)
}

func addMaterialCollection(e *Entry, items []Item) {
	if len(items) > 0 {
		e.Sections = append(e.Sections, Section{
			ID: "collection", Title: "Где собрать", EnglishTitle: "Where to collect", Type: "route", Items: items,
		})
	}
}

func (s *Store) guideMaterialSteps(guide string, stepTitles ...string) []Item {
	rec := s.records["article/"+guide]
	if rec == nil {
		return nil
	}
	out := make([]Item, 0, len(stepTitles))
	for _, title := range stepTitles {
		for _, section := range rec.entry.Sections {
			for _, item := range section.Items {
				if item.EnglishTitle == title {
					out = append(out, item)
				}
			}
		}
	}
	return out
}

func (s *Store) addOdysseyMaterialAcquisition() {
	guides := []string{"farm-odyssey-assets", "farm-odyssey-data", "farm-odyssey-goods"}
	type route struct {
		step  Item
		index string
	}
	byName := map[string]route{}
	for _, guide := range guides {
		rec := s.records["article/"+guide]
		if rec == nil {
			continue
		}
		for _, section := range rec.entry.Sections {
			if !strings.HasSuffix(section.ID, "-index") {
				continue
			}
			for _, item := range section.Items {
				byName[item.EnglishTitle] = route{item, section.ID}
			}
		}
	}
	for id, rec := range s.records {
		if !strings.HasPrefix(id, "material/odyssey-") {
			continue
		}
		e := &rec.entry
		direct, ok := byName[e.EnglishTitle]
		if !ok {
			continue
		}
		direct.step.Title, direct.step.EnglishTitle = "Места сбора", "Loot locations"
		direct.step.Text = strings.TrimSuffix(direct.step.Text, " Если предмета нет в добыче, обменяйте излишки той же группы у бармена.")
		direct.step.EnglishText = strings.TrimSuffix(direct.step.EnglishText, " If the item does not drop, barter surplus from the same asset pool.")
		addMaterialCollection(e, []Item{direct.step})
		s.addOdysseyAssetTrade(e, direct.index)
	}
}

func (s *Store) addOdysseyAssetTrade(e *Entry, index string) {
	source, ok := odysseyBarterStock[index]
	if !ok {
		return
	}
	id := "material/odyssey-" + source.key
	rec := s.records[id]
	if rec == nil || e.ID == id {
		return
	}
	e.Sections = append(e.Sections, Section{
		ID: "trade", Title: "Обмен у бармена", EnglishTitle: "Bartender barter", Type: "links",
		Items: []Item{{
			Title: rec.entry.Title, EnglishTitle: rec.entry.EnglishTitle, EntryID: id,
			Text:        fmt.Sprintf("В разделе «%s» отдайте излишки. %s даёт %d очков за единицу. Стоимость нужного количества покажет бармен.", source.pool, rec.entry.Title, source.points),
			EnglishText: fmt.Sprintf("Use surplus in the %s pool. %s is worth %d points per unit. The bartender shows the cost for the amount you select.", source.poolEN, rec.entry.EnglishTitle, source.points),
		}},
	})
}

func (s *Store) addMaterialEngineerUnlocks() {
	unlocks := map[string][]Item{}
	for _, row := range v.Rows(data.Object("engineering-roadmap.json")["engineers"]) {
		key := v.Str(row["key"])
		id := "engineer/" + key
		rec := s.records[id]
		if rec == nil {
			continue
		}
		odyssey := strings.HasPrefix(key, "odyssey-")
		for _, requirement := range v.Rows(row["requirements"]) {
			for _, ingredient := range v.Rows(requirement["items"]) {
				materialID := farmingMaterialID(v.Str(ingredient["key"]), odyssey)
				if s.records[materialID] != nil {
					unlocks[materialID] = append(unlocks[materialID], Item{
						Title: rec.entry.Title, EnglishTitle: v.Text(rec.entry.EnglishTitle, row["english"]),
						Text: v.Str(requirement["text"]), EnglishText: v.Str(requirement["english"]), EntryID: id,
					})
				}
			}
		}
	}
	for id, items := range unlocks {
		rec := s.records[id]
		rec.entry.Sections = append(rec.entry.Sections, Section{
			ID: "engineer-unlocks", Title: "Открытие инженеров", EnglishTitle: "Engineer unlocks", Type: "links", Items: items,
		})
	}
}

var odysseyBarterStock = map[string]struct {
	key, pool, poolEN string
	points            int
}{
	"chemical-index": {"graphene", "Химикаты", "Chemicals", 13},
	"circuits-index": {"microelectrode", "Схемы", "Circuits", 9},
	"tech-index":     {"weaponcomponent", "Технологии", "Technology", 10},
}
