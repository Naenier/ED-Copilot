package reference

import (
	"fmt"
	"slices"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func odysseyBlueprintID(recipe v.M) string {
	return "blueprint/odyssey-" + slug(v.Text(recipe["english"], recipe["name"]))
}

func odysseyRecipeVariant(recipe v.M) (string, string) {
	if recipe["moduleKey"] == "suit" {
		return "Костюмы", "Suits"
	}
	switch recipe["damageType"] {
	case "kinetic":
		return "Кинетическое оружие", "Kinetic weapons"
	case "laser":
		return "Лазерное оружие", "Laser weapons"
	case "plasma":
		return "Плазменное оружие", "Plasma weapons"
	default:
		return "Личное оружие", "On-foot weapons"
	}
}

func (s *Store) buildOdysseyRecipes() {
	catalogue := data.Object("engineering-roadmap.json")
	materials := map[string]v.M{}
	for _, raw := range v.Rows(catalogue["odysseyMaterials"]) {
		materials[v.Str(raw["key"])] = raw
	}
	s.groupNames["odyssey-blueprints"] = "Модификации Odyssey"
	workshops := map[string][]Item{}
	for _, recipe := range v.Rows(catalogue["odysseyRecipes"]) {
		id, key := odysseyBlueprintID(recipe), v.Str(recipe["key"])
		s.aliases["blueprint/"+key], s.aliases["blueprints/"+key] = id, id
		title, englishTitle := odysseyRecipeVariant(recipe)
		record := s.records[id]
		if record == nil {
			description := odysseyRecipeDescriptions[v.Str(recipe["english"])]
			record = s.add(Entry{ID: id, Category: "blueprints", Group: "odyssey-blueprints",
				Title: v.Str(recipe["name"]), EnglishTitle: v.Str(recipe["english"]), Description: description.ru, EnglishDescription: description.en}, key)
			gearArticle := "article/onfoot-weapons"
			if recipe["moduleKey"] == "suit" {
				gearArticle = "article/suits"
			}
			record.relatedIDs = append(record.relatedIDs, gearArticle)
		}
		grade := Grade{Title: "Установка модификации", Effects: []Property{{Label: "Стоимость, кр.", Value: fmt.Sprint(v.Int(recipe["credits"]))}}}
		for _, ingredient := range v.Rows(recipe["materials"]) {
			materialKey := v.Str(ingredient["key"])
			materialID := "material/odyssey-" + materialKey
			grade.Materials = append(grade.Materials, Ingredient{Name: v.Text(materials[materialKey]["name"], materialKey), Count: v.Int(ingredient["count"]), EntryID: materialID})
			if material := s.records[materialID]; material != nil {
				material.relatedIDs = append(material.relatedIDs, id)
			}
		}
		record.entry.Sections = append(record.entry.Sections, Section{ID: "recipe-" + key, Title: title, EnglishTitle: englishTitle, Type: "grades", Grades: []Grade{grade}})
		for _, key := range stringArray(recipe["engineerKeys"]) {
			engineerID := "engineer/" + key
			engineer := s.records[engineerID]
			if engineer == nil || slices.ContainsFunc(workshops[id], func(item Item) bool { return item.EntryID == engineerID }) {
				continue
			}
			workshops[id] = append(workshops[id], Item{Title: engineer.entry.Title, EnglishTitle: engineer.entry.Subtitle, EntryID: engineerID})
		}
	}
	for id, items := range workshops {
		s.records[id].entry.Sections = append(s.records[id].entry.Sections, Section{ID: "workshops", Title: "Инженеры", EnglishTitle: "Engineers", Type: "prose", Items: items})
	}
}

var odysseyRecipeDescriptions = map[string]struct{ ru, en string }{
	"Added Melee Damage": {
		"Умножает урон удара оружием в ближнем бою на 2,5. Модификация действует только на ближнюю атаку.",
		"Multiplies melee strike damage by 2.5. The modification applies to melee attacks.",
	},
	"Combat Movement Speed": {
		"Увеличивает скорость движения с поднятым прицелом. Полезно, если нужно смещаться относительно цели, продолжая вести прицельный огонь.",
		"Increases movement speed while aiming down sights, letting you reposition without lowering the weapon.",
	},
	"Damage Resistance": {
		"Броня костюма получает на 10% меньше кинетического, термического, плазменного и взрывного урона.",
		"The suit's armour takes 10% less kinetic, thermal, plasma and explosive damage.",
	},
	"Enhanced Tracking": {
		"Удваивает дальность определения личности цели при прямой видимости и ускоряет сканирование. Пригодится, когда нужно быстро распознать охранника до сближения.",
		"Doubles line-of-sight identification range and speeds up the scan. Useful for identifying a guard before approaching.",
	},
	"Extra Ammo Capacity": {
		"Увеличивает носимый запас боеприпасов на 50%. Размер магазина оружия меняется отдельной модификацией.",
		"Increases carried reserve ammunition by 50%. A separate weapon modification increases magazine size.",
	},
	"Extra Backpack Capacity": {
		"Удваивает отдельные лимиты рюкзака для товаров, компонентов и данных. Позволяет реже возвращаться к кораблю или ТРП при сборе на поселениях.",
		"Doubles the backpack limits for goods, assets and data, reducing trips back to the ship or SRV while scavenging settlements.",
	},
	"Faster Shield Regen": {
		"Ускоряет восстановление щита костюма на 25%. Позволяет раньше выйти из укрытия после размена выстрелами.",
		"Speeds up suit shield regeneration by 25%, shortening the time you need to spend in cover between engagements.",
	},
	"Improved Battery Capacity": {
		"Увеличивает ёмкость батареи костюма на 50%. Общий запас питания пригодится для щита, инструментов и длительных вылазок вдали от зарядки.",
		"Increases suit battery capacity by 50%. The larger shared reserve supports shields, tools and longer trips away from chargers.",
	},
	"Improved Jump Assist": {
		"Продлевает работу ранца за один прыжок. Позволяет проще добраться до крыш или пересечь препятствие. Высота и дистанция зависят от гравитации.",
		"Extends the powered portion of a jump. Use it to reach roofs or cross obstacles. Local gravity affects height and distance.",
	},
	"Increased Air Reserves": {
		"Увеличивает время работы аварийного запаса кислорода в пять раз. Даёт больше времени дойти до корабля, ТРП или помещения с атмосферой.",
		"Multiplies emergency oxygen time by five, giving you longer to reach a ship, an SRV or a pressurised area.",
	},
	"Increased Sprint Duration": {
		"Удваивает продолжительность спринта. Удобно при обходе крупных поселений и поиске образцов на поверхности.",
		"Doubles sprint duration for covering large settlements or searching for samples on the surface.",
	},
	"Night Vision": {
		"Добавляет режим ночного видения с подсветкой контуров. Помогает ориентироваться на тёмной стороне планеты и в обесточенных поселениях.",
		"Adds a night-vision mode that outlines terrain and objects on dark planetary surfaces and in unpowered settlements.",
	},
	"Quieter Footsteps": {
		"Уменьшает дальность слышимости шагов вдвое. Помогает пройти мимо охраны, когда она не видит вас напрямую.",
		"Halves the distance at which footsteps can be heard, making it easier to pass guards outside their line of sight.",
	},
	"Reduced Tool Battery Consumption": {
		"Вдвое снижает расход батареи инструментами костюма. Полезно при вскрытии нескольких панелей или продолжительной работе без зарядки.",
		"Halves the battery drain from suit tools, useful when cutting several panels or working away from a charger.",
	},
	"Audio Masking": {
		"Сокращает дальность слышимости выстрела в разгерметизированной зоне до 5% от обычной. Для герметичных помещений предусмотрен «Глушитель».",
		"Cuts the audible range of gunfire in unpressurised areas to 5% of normal. Use Noise Suppressor for pressurised rooms.",
	},
	"Faster Handling": {
		"Ускоряет извлечение и убирание оружия и переход к прицелу. Полезно при частой смене оружия между снятием щита и атакой по броне.",
		"Speeds up drawing and stowing the weapon and aiming down sights. Useful when switching weapons between shield and armour targets.",
	},
	"Greater Range": {
		"Увеличивает эффективную дальность оружия на 50%. Сверьте тип оружия перед закупкой, поскольку для кинетики, лазеров и плазмы нужны разные материалы.",
		"Increases effective weapon range by 50%. Check the weapon type before collecting materials because kinetic, laser and plasma recipes differ.",
	},
	"Headshot Damage": {
		"Увеличивает множитель урона при попадании в голову на 50%. Для кинетического, лазерного и плазменного оружия применяются разные рецепты.",
		"Increases the headshot damage multiplier by 50%. Kinetic, laser and plasma weapons use different recipes.",
	},
	"Improved Hip Fire Accuracy": {
		"Уменьшает разброс при стрельбе от бедра. Пригодится в ближнем бою, когда важнее быстро открыть огонь, чем поднять прицел.",
		"Reduces hip-fire spread, useful at close range when taking time to aim down sights would slow the first shot.",
	},
	"Magazine Size": {
		"Увеличивает магазин на 50% с округлением вверх. Магазин на три патрона после установки вмещает пять.",
		"Increases magazine size by 50%, rounded up. A three-round magazine becomes a five-round magazine.",
	},
	"Noise Suppressor": {
		"Сокращает дальность слышимости выстрела в герметичной зоне до 5% от обычной. Для разгерметизированных зон предусмотрена звуковая маскировка.",
		"Cuts the audible range of gunfire in pressurised areas to 5% of normal. Use Audio Masking in unpressurised areas.",
	},
	"Reload Speed": {
		"Сокращает время ручной перезарядки. Пригодится оружию с небольшим магазином, если его нужно держать наготове между сериями выстрелов.",
		"Shortens manual reloads. Useful for small magazines when you want to keep the same weapon ready between bursts.",
	},
	"Scope": {
		"Добавляет оптику с увеличением при прицеливании. Упрощает выцеливание головы и точные выстрелы по дальним целям.",
		"Adds magnified optics when aiming down sights, making headshots and distant targets easier to line up.",
	},
	"Stability": {
		"Уменьшает отдачу оружия. Помогает удерживать прицел на цели во время очереди.",
		"Reduces weapon recoil so you can keep the sights on target during a burst.",
	},
	"Stowed reloading": {
		"Оружие автоматически пополняет магазин из запаса, пока убрано. Можно стрелять из второго оружия, пока первое перезаряжается.",
		"Refills the magazine from reserve ammunition while the weapon is stowed, letting you use a second weapon in the meantime.",
	},
}
