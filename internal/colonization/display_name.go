package colonization

import "strings"

// Display names never participate in market/system matching. Keep the game's
// original station name and MarketID in observations and saved plans.
func displayBuildingName(building M) string {
	name := str(building["name"])
	if building["journalDiscovered"] != true {
		// Older versions can drop the automatic-discovery flag. An exact
		// observed raw name is still safe to format; a user override is not.
		if raw := str(building["journalStationName"]); raw != "" && name == raw {
			return displayConstructionName(name)
		}
		return name
	}
	rawStation := firstText(building["journalStationName"], name)
	if name == rawStation {
		name = firstText(building["journalDisplayName"], name)
	}
	return displayConstructionName(name)
}

func displayConstructionName(raw string) string {
	name := strings.TrimSpace(raw)
	for _, pair := range [][2]string{
		{"$EXT_PANEL_ColonisationShip;", "Колонизационный корабль"},
		{"Planetary Construction Site:", "Планетарная стройплощадка"},
		{"Orbital Construction Site:", "Орбитальная стройплощадка"},
		{"Surface Construction Site:", "Поверхностная стройплощадка"},
		{"Construction Site:", "Стройплощадка"},
		{"Colonisation Ship:", "Колонизационный корабль"},
		{"Colonization Ship:", "Колонизационный корабль"},
	} {
		if len(name) >= len(pair[0]) && strings.EqualFold(name[:len(pair[0])], pair[0]) {
			identity := strings.TrimLeft(strings.TrimSpace(name[len(pair[0]):]), ":–—- ")
			if identity == "" {
				return pair[1]
			}
			return pair[1] + " · " + identity
		}
	}
	// Unknown journal localization markup has no useful player-facing meaning.
	// Drop only a leading token, keeping the station identity after it intact.
	if strings.HasPrefix(name, "$") {
		if end := strings.IndexByte(name, ';'); end >= 0 {
			name = strings.TrimSpace(name[end+1:])
		}
	}
	if name == "" {
		return "Стройплощадка из журнала"
	}
	return name
}
