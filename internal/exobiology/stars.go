package exobiology

import (
	"sort"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

type starGate struct {
	fragment, scope, allowed string
	orHost                   bool
	luminosity               map[string][]string
}

// Observation-backed gates in the original catalogue supersede older prose
// conditions. Failed gates demote, never erase an observed species.
var starGates = []starGate{
	{fragment: "amphora", scope: "any", allowed: "A B"},
	{fragment: "anemone_luteolum", scope: "any", allowed: "B", luminosity: map[string][]string{"B": {"IV", "V"}}},
	{fragment: "anemone_blatteum_bioluminescent", scope: "any", allowed: "B", luminosity: map[string][]string{"B": {"IV", "V"}}},
	{fragment: "anemone_roseum", scope: "any", allowed: "B", luminosity: map[string][]string{"B": {"I", "II", "III"}}},
	{fragment: "anemone_croceum", scope: "any", allowed: "B A", luminosity: map[string][]string{"B": {"VI"}, "A": {"I", "II", "III"}, "AeBe": {}}},
	{fragment: "anemone_rubeum_bioluminescent", scope: "any", allowed: "B A", luminosity: map[string][]string{"B": {"IV", "VI"}, "A": {"I", "II", "III"}, "AeBe": {}}},
	{fragment: "anemone_puniceum", scope: "any", allowed: "O W"},
	{fragment: "anemone_prasinum_bioluminescent", scope: "any", allowed: "O W A", luminosity: map[string][]string{"A": {}, "AeBe": {"I", "II", "III", "IV", "V", "VI", "VII"}}},
	{fragment: "stratum_araneamus", scope: "any", allowed: "A N B H"},
	{fragment: "stratum_tectonicas", scope: "host", allowed: "O B F K M L T Y TTS D W H other"},
	{fragment: "concha_labiata", scope: "main", orHost: true, allowed: "O B A F G K L T Y TTS D W N H other"},
	{fragment: "crystalline_shards", scope: "host", allowed: "A F G K M L T Y TTS other"},
	{fragment: "electricae_pluma", scope: "any", allowed: "A N D H"},
}

func gateForSpecies(id string) *starGate {
	for i := range starGates {
		if strings.Contains(id, starGates[i].fragment) {
			return &starGates[i]
		}
	}
	return nil
}
func starClass(raw any) string {
	text := strings.TrimSpace(strings.ToUpper(v.Str(raw)))
	if text == "" {
		return ""
	}
	for _, pair := range [][2]string{{"BLACK HOLE", "H"}, {"NEUTRON", "N"}, {"WHITE DWARF", "D"}, {"WOLF", "W"}, {"TAURI", "TTS"}, {"HERBIG", "A"}} {
		if strings.Contains(text, pair[0]) {
			return pair[1]
		}
	}
	if strings.HasPrefix(text, "TTS") {
		return "TTS"
	}
	for _, prefix := range []string{"D", "N", "H", "W"} {
		if strings.HasPrefix(text, prefix) {
			return prefix
		}
	}
	if strings.Contains("OBAFGKMLTY", text[:1]) {
		return text[:1]
	}
	return "other"
}
func luminosityClass(raw any) string {
	text := strings.ToUpper(v.Str(raw))
	if text == "0" {
		return "I"
	}
	for _, class := range []string{"VII", "VI", "IV", "V", "III", "II", "I"} {
		if strings.HasPrefix(text, class) {
			return class
		}
	}
	return ""
}
func (s *Service) systemStars(b *body) []*body {
	rows := []*body{}
	if sy := s.systems[b.Address]; sy != nil {
		for key := range sy.BodyIDs {
			candidate := s.bodies[key]
			if candidate != nil && candidate.Scan["StarType"] != nil {
				rows = append(rows, candidate)
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return v.Int(rows[i].ID) < v.Int(rows[j].ID) })
	return rows
}
func (s *Service) gateStars(b *body, gate *starGate) []*body {
	all := s.systemStars(b)
	if gate.scope == "any" {
		return all
	}
	hosts := []*body{}
	explicitParent := false
	current := b
	visited := map[string]bool{}
	for depth := 0; depth < 16; depth++ {
		next := (*body)(nil)
		for _, parent := range v.Rows(current.Scan["Parents"]) {
			if id, exists := parent["Star"]; exists {
				explicitParent = true
				if star := s.bodies[bodyKey(b.Address, id)]; star != nil && star.Scan["StarType"] != nil {
					hosts = append(hosts, star)
					break
				}
			}
			if next == nil {
				if id, exists := parent["Planet"]; exists {
					next = s.bodies[bodyKey(b.Address, id)]
				}
			}
		}
		if len(hosts) > 0 || next == nil || visited[next.Key] {
			break
		}
		visited[next.Key] = true
		current = next
	}
	if len(hosts) == 0 && !explicitParent {
		hosts = all
	}
	if gate.scope == "host" {
		return hosts
	}
	result := []*body{}
	for _, star := range all {
		if v.Int(star.ID) == 0 {
			result = append(result, star)
			break
		}
	}
	if gate.orHost {
		result = append(result, hosts...)
	}
	return result
}
func (s *Service) checkObservedStarGate(a *assessment, b *body, gate *starGate) {
	stars := s.gateStars(b, gate)
	allowed := strings.Fields(gate.allowed)
	known, pass, unresolved := false, false, false
	for _, star := range stars {
		class := starClass(star.Scan["StarType"])
		if class == "" {
			continue
		}
		known = true
		if !listHas(allowed, class, func(x any) string { return v.Str(x) }) {
			continue
		}
		lumKey := class
		if strings.EqualFold(v.Str(star.Scan["StarType"]), "AeBe") {
			lumKey = "AeBe"
		}
		rule, hasRule := gate.luminosity[lumKey]
		if !hasRule {
			pass = true
			break
		}
		lum := luminosityClass(star.Scan["Luminosity"])
		if lum == "" {
			unresolved = true
			continue
		}
		if listHas(rule, lum, func(x any) string { return v.Str(x) }) {
			pass = true
			break
		}
	}
	a.check(pass || known && !unresolved, pass, "Тип или класс светимости звезды не соответствует")
}
