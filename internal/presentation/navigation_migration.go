package presentation

import v "github.com/Naenier/ED-Copilot/internal/value"

// MigrateNavigation upgrades only the unmodified old default, preserving a
// pilot's explicitly customized order and visibility.
func MigrateNavigation(raw any, version int) []string {
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
