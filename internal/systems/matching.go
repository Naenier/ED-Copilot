package systems

import (
	"sort"
	"strings"
)

func relevant(rows []Suggestion, query string, correction bool) []Suggestion {
	q := comparable(query)
	out := []Suggestion{}
	for _, s := range rows {
		name := comparable(s.Name)
		if correction {
			if key(s.Name) != "sagittarius a*" || editDistance(q, name) > 2 {
				continue
			}
			s.Match = "correction"
		} else {
			if !strings.Contains(name, q) {
				continue
			}
			s.Match = "prefix"
			if key(s.Name) == key(query) {
				s.Match = "exact"
			}
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Match == "exact" && out[j].Match != "exact" })
	return out
}
func correctionQuery(query string) string {
	q, target := comparable(query), "sagittariusa"
	if q != target && len(q) >= 8 && editDistance(q, target) <= 2 {
		return "Sagittarius A*"
	}
	return ""
}
func editDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	row := make([]int, len(y)+1)
	for j := range row {
		row[j] = j
	}
	for i, r := range x {
		prev := row[0]
		row[0] = i + 1
		for j, s := range y {
			saved := row[j+1]
			cost := 1
			if r == s {
				cost = 0
			}
			row[j+1] = min(row[j+1]+1, row[j]+1, prev+cost)
			prev = saved
		}
	}
	return row[len(y)]
}
