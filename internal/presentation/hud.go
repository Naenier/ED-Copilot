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
	Language string
	Sections []HUDSection
}
type HUDSection struct {
	Title, Subtitle, Kind string
	Lines                 []HUDLine
	More                  int
}
type HUDLine struct{ Text, Role, Value string }
type HUDRect struct{ X, Y, Width, Height int }

func hudCard(doc *HUDDocument, kind, title, subtitle string, more int, lines ...HUDLine) {
	clean := make([]HUDLine, 0, len(lines))
	seen := make(map[[2]string]bool, len(lines))
	for _, line := range lines {
		line.Text = strings.TrimSpace(line.Text)
		line.Value = strings.TrimSpace(line.Value)
		key := [2]string{line.Text, line.Value}
		if line.Text != "" && !seen[key] {
			clean = append(clean, line)
			seen[key] = true
		}
	}
	if len(clean) > 0 {
		doc.Sections = append(doc.Sections, HUDSection{Title: title, Subtitle: strings.TrimSpace(subtitle), Kind: kind, Lines: clean, More: more})
	}
}

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

func hudGood(s M, key string) string {
	for _, r := range v.Rows(s["goods"]) {
		if v.Key(r["key"]) == v.Key(key) {
			return v.Text(r["name"], key)
		}
	}
	return key
}
