package presentation

import v "github.com/Naenier/ED-Copilot/internal/value"

// MigrateNavigation upgrades only the unmodified old default, preserving a
// pilot's explicitly customized order and visibility.
func MigrateNavigation(raw any, version int) []string {
	previousDefault := []string{"overview", "ranks", "politics", "targets", "deals", "sale", "mining", "exobiology", "learning", "encyclopedia", "statistics", "settings"}
	if version < 4 && sameNavigation(raw, previousDefault) {
		return append([]string(nil), DefaultTabs...)
	}
	previous := []string{"overview", "targets", "sale", "mining", "exobiology", "learning", "encyclopedia", "statistics", "settings"}
	if version < 3 && sameNavigation(raw, previous) {
		return append([]string(nil), DefaultTabs...)
	}
	legacy := []string{"overview", "targets", "sale", "encyclopedia", "statistics", "settings"}
	rows := v.Arr(raw)
	if version < 2 && len(rows) == len(legacy) {
		matches := true
		for i, key := range legacy {
			if v.Str(rows[i]) != key {
				matches = false
				break
			}
		}
		if matches {
			return append([]string(nil), DefaultTabs...)
		}
	}
	return NormalizeTabs(raw)
}

func sameNavigation(raw any, expected []string) bool {
	rows := v.Arr(raw)
	if len(rows) != len(expected) {
		return false
	}
	for i, id := range expected {
		if v.Str(rows[i]) != id {
			return false
		}
	}
	return true
}
