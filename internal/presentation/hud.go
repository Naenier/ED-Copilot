package presentation

import (
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// HUDDocument contains only the selected activity. It has no dependency on a
// window system, so focus policy, unknown observations and colony isolation are
// tested without a running game or a real commander profile.
type HUDDocument struct {
	Activity string
	Sections []HUDSection
}
type HUDSection struct {
	Title string
	Lines []HUDLine
}
type HUDLine struct{ Text, Role string }
type HUDRect struct{ X, Y, Width, Height int }

// HUDVisible requires an identified game process. Journal activity alone is
// insufficient: old journal files also exist when another program has focus.
func HUDVisible(enabled bool, running any, foregroundProcess string) bool {
	if !enabled || running == false {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(foregroundProcess))
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	switch name {
	case "elitedangerous.exe", "elitedangerous32.exe", "elitedangerous64.exe":
		return true
	}
	return false
}

func BuildHUD(s M, now time.Time) HUDDocument { return LocalizeHUD(buildHUDContent(s, now), s) }

func hudActivity(id string) string {
	for _, a := range Activities {
		if a.Value == id {
			return strings.ToUpper(a.Label)
		}
	}
	return "ПОЛЁТ И НАВИГАЦИЯ"
}
func hudGood(s M, key string) string {
	for _, r := range v.Rows(s["goods"]) {
		if v.Key(r["key"]) == v.Key(key) {
			return v.Text(r["name"], key)
		}
	}
	return key
}
