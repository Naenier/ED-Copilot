package data

import (
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestEngineeringRoadmapCatalogueReferences(t *testing.T) {
	catalogue := Object("engineering-roadmap.json")
	ship := Object("engineering-data.json")
	engineers := v.Rows(catalogue["engineers"])
	byKey, byID := map[string]v.M{}, map[string]bool{}
	shipCount, odysseyCount := 0, 0
	for _, row := range engineers {
		key, id := v.Str(row["key"]), v.Str(row["journalId"])
		if key == "" || id == "" || byKey[key] != nil || byID[id] || v.Str(row["base"]) == "" || v.Str(row["system"]) == "" {
			t.Fatalf("incomplete or duplicate engineer: %v", row)
		}
		byKey[key], byID[id] = row, true
		switch row["kind"] {
		case "ship":
			shipCount++
		case "odyssey":
			odysseyCount++
		default:
			t.Fatalf("unknown engineer category: %v", row)
		}
	}
	if shipCount != 25 || odysseyCount != 13 {
		t.Fatalf("engineer counts ship=%d Odyssey=%d", shipCount, odysseyCount)
	}
	active, done := map[string]bool{}, map[string]bool{}
	var check func(string)
	check = func(key string) {
		if done[key] {
			return
		}
		if active[key] || byKey[key] == nil {
			t.Fatalf("invalid engineer referral: %s", key)
		}
		active[key] = true
		row := byKey[key]
		steps := v.Rows(row["requirements"])
		if len(steps) == 0 {
			t.Fatalf("missing unlock requirements for %s", key)
		}
		stepKeys := map[string]bool{}
		for _, step := range steps {
			stepKey := v.Str(step["key"])
			if stepKey == "" || stepKeys[stepKey] || v.Str(step["text"]) == "" || v.Str(step["english"]) == "" {
				t.Fatalf("invalid unlock requirement for %s: %v", key, step)
			}
			stepKeys[stepKey] = true
			if step["kind"] == "referral" {
				check(v.Str(step["engineerKey"]))
			}
		}
		active[key], done[key] = false, true
	}
	for key := range byKey {
		check(key)
	}
	if yi := v.Rows(byKey["odyssey-yi-shen"]["requirements"]); len(yi) != 3 {
		t.Fatalf("Yi Shen must require three referrals: %v", yi)
	}
	odysseyMats := catalogueKeys(v.Rows(catalogue["odysseyMaterials"]))
	shipMats := catalogueKeys(v.Rows(ship["materials"]))
	recipes := v.Rows(catalogue["odysseyRecipes"])
	if len(recipes) < 31 {
		t.Fatalf("missing Odyssey recipes: %d", len(recipes))
	}
	for _, recipe := range recipes {
		if v.Str(recipe["journalSymbol"]) == "" || v.Str(recipe["name"]) == "" || v.Int(recipe["credits"]) <= 0 {
			t.Fatalf("incomplete Odyssey recipe: %v", recipe)
		}
		providers := v.Arr(recipe["engineerKeys"])
		if len(providers) == 0 {
			t.Fatalf("no provider: %v", recipe)
		}
		for _, provider := range providers {
			if byKey[v.Str(provider)]["kind"] != "odyssey" {
				t.Fatalf("invalid Odyssey recipe provider: %v", recipe)
			}
		}
		checkRecipeMaterials(t, recipe["materials"], odysseyMats)
	}
	grades := v.Rows(catalogue["odysseyGradeRecipes"])
	if len(grades) != 56 {
		t.Fatalf("personal equipment grade recipes: %d", len(grades))
	}
	for _, recipe := range grades {
		checkRecipeMaterials(t, recipe["materials"], odysseyMats)
	}
	for _, recipe := range v.Rows(catalogue["specialShipRecipes"]) {
		if recipe["acquisition"] != "mercenary" || len(v.Rows(recipe["variants"])) == 0 {
			t.Fatalf("missing special module variant: %v", recipe)
		}
		for _, ingredients := range v.Obj(recipe["grades"]) {
			checkRecipeMaterials(t, ingredients, shipMats)
		}
	}
}

func TestShipEngineeringJournalModuleAndRecipeCoverage(t *testing.T) {
	ship := Object("engineering-data.json")
	symbols := map[string]string{}
	blueprints := catalogueKeys(v.Rows(ship["blueprints"]))
	effects := catalogueKeys(v.Rows(ship["experimentals"]))
	materials := catalogueKeys(v.Rows(ship["materials"]))
	for _, row := range v.Rows(ship["modules"]) {
		key := v.Str(row["key"])
		if len(v.Arr(row["journalSymbols"])) == 0 || v.Str(row["english"]) == "" {
			t.Fatalf("incomplete module: %s", key)
		}
		for _, symbol := range v.Arr(row["journalSymbols"]) {
			symbol := strings.ToLower(v.Str(symbol))
			if previous := symbols[symbol]; previous != "" {
				t.Fatalf("ambiguous journal symbol %s in %s and %s", symbol, previous, key)
			}
			symbols[symbol] = key
		}
		for _, key := range v.Arr(row["blueprints"]) {
			if !blueprints[v.Str(key)] {
				t.Fatalf("unknown blueprint %s", key)
			}
		}
		for _, key := range v.Arr(row["experimentals"]) {
			if !effects[v.Str(key)] {
				t.Fatalf("unknown experimental effect %s", key)
			}
		}
	}
	for _, row := range v.Rows(ship["blueprints"]) {
		for _, recipe := range v.Obj(row["grades"]) {
			checkRecipeMaterials(t, recipe, materials)
		}
	}
	for _, row := range v.Rows(ship["experimentals"]) {
		checkRecipeMaterials(t, row["materials"], materials)
	}
}

func catalogueKeys(rows []v.M) map[string]bool {
	keys := make(map[string]bool, len(rows))
	for _, row := range rows {
		keys[v.Str(row["key"])] = true
	}
	return keys
}

func checkRecipeMaterials(t *testing.T, recipe any, known map[string]bool) {
	t.Helper()
	rows := v.Rows(recipe)
	if len(rows) == 0 {
		t.Fatalf("missing recipe materials: %v", recipe)
	}
	for _, row := range rows {
		if !known[v.Str(row["key"])] || v.Int(row["count"]) <= 0 {
			t.Fatalf("invalid recipe ingredient: %v", row)
		}
	}
}
