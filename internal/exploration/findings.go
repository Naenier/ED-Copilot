package exploration

import (
	"fmt"
	"math"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type findingProfile struct {
	ID    string                 `json:"id"`
	Name  string                 `json:"name"`
	Rules map[string]findingRule `json:"rules"`
}

type findingRule struct {
	Mode      string  `json:"mode"`
	Threshold float64 `json:"threshold"`
	Material  string  `json:"material,omitempty"`
	Cooldown  int     `json:"cooldown"`
}

type finding struct {
	ID            string `json:"id"`
	Rule          string `json:"rule"`
	System        string `json:"system"`
	SystemAddress string `json:"systemAddress"`
	Body          string `json:"body"`
	BodyID        string `json:"bodyId"`
	ObservedAt    string `json:"observedAt"`
	NotifiedAt    string `json:"notifiedAt,omitempty"`
	Detail        string `json:"detail"`
	EnglishDetail string `json:"englishDetail"`
	Values        M      `json:"values"`
}

type record struct {
	Key           string  `json:"key"`
	System        string  `json:"system"`
	SystemAddress string  `json:"systemAddress"`
	Body          string  `json:"body"`
	BodyID        string  `json:"bodyId"`
	Value         float64 `json:"value"`
	Unit          string  `json:"unit"`
	ObservedAt    string  `json:"observedAt"`
}

type ruleDefinition struct {
	key, title, english, unit string
	value, maximum            float64
}

var ruleDefinitions = []ruleDefinition{
	{"earthlike", "Землеподобный мир", "Earth-like world", "", 0, 0},
	{"terraformable", "Терраформируемый мир", "Terraformable world", "", 0, 0},
	{"ring-types", "Разные типы колец", "Multiple ring types", "", 0, 0},
	{"inside-rings", "Спутник внутри кольца", "Moon within a ring", "", 0, 0},
	{"material", "Искомый материал", "Target material", "%", 0, 100},
	{"hot", "Высокая температура", "High temperature", "K", 1500, 1e8},
	{"gravity", "Высокая гравитация", "High gravity", "g", 2, 1e6},
	{"heavy-landing", "Высокая гравитация на посадочном теле", "High gravity on a landable body", "g", 2, 1e6},
	{"eccentric", "Вытянутая орбита", "High orbital eccentricity", "", .8, 1},
}

func defaultProfile() findingProfile {
	profile := findingProfile{ID: "default", Rules: map[string]findingRule{}}
	for _, d := range ruleDefinitions {
		mode := "history"
		switch d.key {
		case "earthlike", "inside-rings":
			mode = "notify"
		case "material", "heavy-landing":
			mode = "off"
		}
		profile.Rules[d.key] = findingRule{Mode: mode, Threshold: d.value, Cooldown: 15}
	}
	return profile
}

func normalizedProfile(raw findingProfile) findingProfile {
	profile := defaultProfile()
	profile.ID, profile.Name = raw.ID, text(raw.Name, 100)
	if !workspaceID.MatchString(profile.ID) {
		profile.ID = "default"
	}
	for _, d := range ruleDefinitions {
		rule, exists := raw.Rules[d.key]
		if !exists {
			continue
		}
		if rule.Mode != "off" && rule.Mode != "history" && rule.Mode != "notify" {
			rule.Mode = profile.Rules[d.key].Mode
		}
		if rule.Threshold < 0 || rule.Threshold > d.maximum || math.IsNaN(rule.Threshold) || math.IsInf(rule.Threshold, 0) {
			rule.Threshold = d.value
		}
		rule.Material = v.Key(text(rule.Material, 80))
		rule.Cooldown = min(1440, max(0, rule.Cooldown))
		profile.Rules[d.key] = rule
	}
	return profile
}

func (ws *workspace) restoreFindingProfiles() {
	profiles := []findingProfile{}
	seen := map[string]bool{}
	for _, raw := range bounded(ws.FindingProfiles, 12) {
		row := normalizedProfile(raw)
		if !seen[row.ID] {
			profiles = append(profiles, row)
			seen[row.ID] = true
		}
	}
	if len(profiles) == 0 {
		profiles = append(profiles, defaultProfile())
	}
	if !seen[ws.SelectedProfile] {
		ws.SelectedProfile = profiles[0].ID
	}
	ws.FindingProfiles = profiles
}

func (ws *workspace) profile() findingProfile {
	for _, row := range ws.FindingProfiles {
		if row.ID == ws.SelectedProfile {
			return row
		}
	}
	return defaultProfile()
}

func (ws *workspace) find(sy *system, b *body, at string, notify bool) []M {
	if b.PlanetClass == "" || b.ScannedAt == "" {
		return nil
	}
	profile := ws.profile()
	var notices []M
	for _, d := range ruleDefinitions {
		rule := profile.Rules[d.key]
		if rule.Mode == "off" {
			continue
		}
		row := finding{Rule: d.key, System: sy.Name, SystemAddress: sy.Address, Body: b.Name, BodyID: b.ID, ObservedAt: at}
		if !findingEvidence(&row, sy, b, rule) {
			continue
		}
		id := observationID(d.key, sy.Address, b.ID)
		if ws.findings[id] {
			continue
		}
		row.ID = id
		since := v.Time(at).Sub(v.Time(ws.lastAlert[d.key]))
		if notify && rule.Mode == "notify" && since >= time.Duration(rule.Cooldown)*time.Minute {
			row.NotifiedAt = at
			ws.lastAlert[d.key] = at
			notices = append(notices, M{"id": id, "category": "exploration", "priority": findingPriority(d.key), "title": d.title, "englishTitle": d.english,
				"detail": b.Name + " · " + row.Detail, "englishDetail": b.Name + " · " + row.EnglishDetail, "system": sy.Name, "systemAddress": sy.Address, "body": b.Name, "bodyId": b.ID, "rule": d.key, "observedAt": at})
		}
		ws.findings[id] = true
		ws.Findings = append(ws.Findings, row)
		if len(ws.Findings) > maxFindings {
			delete(ws.findings, ws.Findings[0].ID)
			ws.Findings = append([]finding(nil), ws.Findings[1:]...)
		}
	}
	return notices
}

func findingPriority(rule string) string {
	if rule == "earthlike" || rule == "inside-rings" {
		return "high"
	}
	return "normal"
}

func findingEvidence(row *finding, sy *system, b *body, rule findingRule) bool {
	switch row.Rule {
	case "earthlike":
		row.Detail, row.EnglishDetail = "Класс тела: землеподобный", "Body class: Earth-like"
		row.Values = M{"planetClass": b.PlanetClass}
		return worldKind(b) == "earthlike"
	case "terraformable":
		row.Detail, row.EnglishDetail = "Журнал отмечает пригодность для терраформирования", "Journal confirms terraformability"
		row.Values = M{"terraformable": b.Terraformable, "planetClass": b.PlanetClass}
		return b.Terraformable
	case "ring-types":
		types := []string{}
		seen := map[string]bool{}
		for _, r := range b.Rings {
			class := strings.TrimPrefix(r.Class, "eRingClass_")
			if class != "" && !seen[class] {
				types = append(types, class)
				seen[class] = true
			}
		}
		if len(types) < 2 {
			return false
		}
		ru, en := []string{}, []string{}
		labels := map[string][2]string{"Icy": {"Ледяное", "Icy"}, "Rocky": {"Каменистое", "Rocky"}, "MetalRich": {"Богатое металлами", "Metal-rich"}, "Metallic": {"Металлическое", "Metallic"}}
		for _, kind := range types {
			label := labels[kind]
			ru = append(ru, v.Text(label[0], kind))
			en = append(en, v.Text(label[1], kind))
		}
		row.Detail, row.EnglishDetail = strings.Join(ru, ", "), strings.Join(en, ", ")
		row.Values = M{"ringTypes": types}
		return true
	case "inside-rings":
		parent := sy.Bodies[b.ParentID]
		if parent == nil || parent.PlanetClass == "" || b.SemiMajorAxis == nil || b.Eccentricity == nil {
			return false
		}
		peri, apo := *b.SemiMajorAxis*(1-*b.Eccentricity), *b.SemiMajorAxis*(1+*b.Eccentricity)
		for _, r := range parent.Rings {
			if peri < r.Inner || apo > r.Outer {
				continue
			}
			row.Detail = fmt.Sprintf("Орбита %.0f–%.0f км, кольцо %.0f–%.0f км", peri/1000, apo/1000, r.Inner/1000, r.Outer/1000)
			row.EnglishDetail = fmt.Sprintf("Orbit %.0f–%.0f km, ring %.0f–%.0f km", peri/1000, apo/1000, r.Inner/1000, r.Outer/1000)
			row.Values = M{"parent": parent.Name, "periapsisM": peri, "apoapsisM": apo, "ringInnerM": r.Inner, "ringOuterM": r.Outer}
			return true
		}
		return false
	case "material":
		n, known := b.Materials[rule.Material]
		if !known || n < rule.Threshold {
			return false
		}
		row.Detail = fmt.Sprintf("%s: %.2f%%", rule.Material, n)
		row.EnglishDetail = row.Detail
		row.Values = M{"material": rule.Material, "percent": n, "threshold": rule.Threshold}
		return true
	case "hot":
		return numericFinding(row, b.Temperature, 1, rule.Threshold, "K")
	case "gravity":
		return numericFinding(row, b.Gravity, 9.80665, rule.Threshold, "g")
	case "heavy-landing":
		if !b.Landable || !numericFinding(row, b.Gravity, 9.80665, rule.Threshold, "g") {
			return false
		}
		row.Values["landable"] = true
		row.Detail = "Доступна посадка · " + row.Detail
		row.EnglishDetail = "Landable · " + row.EnglishDetail
		return true
	case "eccentric":
		return numericFinding(row, b.Eccentricity, 1, rule.Threshold, "")
	}
	return false
}

func numericFinding(row *finding, raw *float64, divisor, threshold float64, unit string) bool {
	if raw == nil || *raw/divisor < threshold {
		return false
	}
	value := *raw / divisor
	row.Detail = strings.TrimSpace(fmt.Sprintf("%.3g %s", value, unit))
	row.EnglishDetail = row.Detail
	row.Values = M{"value": value, "unit": unit, "threshold": threshold}
	return true
}

func (ws *workspace) updateRecords(sy *system, b *body) {
	if b.PlanetClass == "" || b.ScannedAt == "" {
		return
	}
	for _, metric := range []struct {
		key, unit string
		raw       *float64
		divisor   float64
		lowest    bool
	}{
		{"hot", "K", b.Temperature, 1, false}, {"cold", "K", b.Temperature, 1, true},
		{"gravity", "g", b.Gravity, 9.80665, false}, {"low-gravity", "g", b.Gravity, 9.80665, true},
		{"radius", "km", b.Radius, 1000, false}, {"small-radius", "km", b.Radius, 1000, true},
		{"mass", "M⊕", b.Mass, 1, false}, {"eccentric", "", b.Eccentricity, 1, false},
	} {
		if metric.raw == nil {
			continue
		}
		n := *metric.raw / metric.divisor
		old, exists := ws.Records[metric.key]
		if !exists || metric.lowest && n < old.Value || !metric.lowest && n > old.Value {
			ws.Records[metric.key] = record{Key: metric.key, System: sy.Name, SystemAddress: sy.Address, Body: b.Name, BodyID: b.ID, Value: n, Unit: metric.unit, ObservedAt: b.ScannedAt}
		}
	}
}
