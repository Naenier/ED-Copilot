package colonization

import (
	"testing"

	"github.com/Naenier/ED-Copilot/internal/value"
)

func TestDevelopmentMeasuresKnownEffectsAndPreservesUnknowns(t *testing.T) {
	p := New(M{"selectedProjectId": "colony", "projects": []M{{
		"id": "colony", "targetSystem": "Example", "systemAddress": "99",
		"points": M{"tier2": 2, "tier3": 0},
		"buildings": []M{
			{"id": "port", "typeId": "dodec", "body": "Example 1", "isPrimary": true, "status": "complete"},
			{"id": "hub", "typeId": "hub-refinery", "body": "1", "status": "planned"},
			{"id": "settlement", "typeId": "settlement-small-hightech", "body": "Example 2", "status": "planned"},
			{"id": "unknown", "typeId": "custom", "status": "complete"},
			{"id": "cancelled", "typeId": "installation-pirate", "status": "cancelled"},
		},
	}}})
	if got := len(developmentTypes); got != 55 {
		t.Fatalf("catalog has %d characterized types", got)
	}
	project := value.Obj(p.Snapshot(M{"includeDevelopment": true})["activeProject"])
	development := value.Obj(project["development"])
	bs := rows(project["buildings"])
	if development["builtCount"] != 2 || development["plannedCount"] != 2 || development["builtTechFloor"] != true {
		t.Fatalf("incorrect development groups: %v", development)
	}
	closeTo := func(got any, want float64) bool { return value.Num(got) >= want-.001 && value.Num(got) <= want+.001 }
	if !closeTo(value.At(bs[0], "impact", "metrics", "security", "weighted"), -5.6) ||
		!closeTo(value.At(bs[1], "impact", "metrics", "development", "weighted"), 6.3) ||
		value.At(bs[0], "impact", "pad") != "L" {
		t.Fatalf("incorrect facility effects: %v, %v", bs[0]["impact"], bs[1]["impact"])
	}
	metrics := rows(development["metrics"])
	if metrics[0]["builtKnown"] != 1 || metrics[0]["plannedKnown"] != 2 || !closeTo(metrics[0]["total"], 10) {
		t.Fatalf("unknown structure treated as known: %v", metrics[0])
	}
	links := rows(value.At(bs[0], "impact", "links"))
	if len(links) != 2 || links[0]["id"] != "hub" || links[0]["strength"] != "strong" ||
		links[1]["id"] != "settlement" || links[1]["strength"] != "weak" || links[1]["state"] != "planned" {
		t.Fatalf("wrong station link geometry: %v", links)
	}
	if value.Num(value.At(project, "pointsProjected", "tier2")) != 0 || value.Num(value.At(project, "pointsProjected", "tier3")) != 2 {
		t.Fatalf("T1 layout and T2 point cost are distinct: %v", project["pointsProjected"])
	}
	if got := value.At(p.Snapshot(nil), "activeProject", "development"); got != nil {
		t.Fatalf("non-development snapshots contain the projection: %v", got)
	}
}

func TestDevelopmentMatchesScannedBodyAndDodecButKeepsObservedEconomy(t *testing.T) {
	p := New(M{"projects": []M{{"id": "colony", "targetSystem": "Example", "systemAddress": "99",
		"buildings": []M{
			{"id": "first", "typeId": "orbis", "body": "1", "isPrimary": true, "status": "planned"},
			{"id": "other", "typeId": "custom", "marketId": "42", "journalSource": "station", "journalStationType": "Dodec", "status": "complete", "body": "Example 1"},
		},
	}}})
	ctx := M{"commander": M{"system": "Example", "systemAddress": "99"}}
	at := "2026-11-13T12:00:00Z"
	p.Apply(M{"event": "Scan", "timestamp": at, "BodyName": "Example 1", "PlanetClass": "Earthlike body", "Rings": []M{{"Name": "Example 1 Ring"}}}, ctx)
	p.Apply(M{"event": "Scan", "timestamp": at, "SystemAddress": "100", "BodyName": "Example 1", "PlanetClass": "Icy body"}, ctx)
	p.Apply(M{"event": "Docked", "timestamp": at, "StarSystem": "Example", "SystemAddress": "99", "MarketID": "42", "StationName": "Actual Port", "StationType": "Dodec",
		"StationEconomies": []M{{"Name": "Tourism", "Proportion": .25}, {"Name": "Industrial", "Proportion": .75}, {"Name": "Invalid", "Proportion": 50}},
		"StationServices":  []string{"shipyard"}}, ctx)
	buildings := rows(value.At(p.Snapshot(M{"includeDevelopment": true}), "activeProject", "buildings"))
	first, other := value.Obj(buildings[0]["impact"]), value.Obj(buildings[1]["impact"])
	if first["bodyType"] != "Earthlike body" || value.Num(first["populationCapacityExample"]) != 3100000000 ||
		!has(stringsOf(first["bodyEconomies"]), "extraction") || !has(stringsOf(first["bodyEconomies"]), "agriculture") {
		t.Fatalf("wrong body projection: %v", first)
	}
	if other["typeId"] != "dodec" || other["populationCapacityExample"] != nil ||
		!has(stringsOf(other["observedServices"]), "shipyard") {
		t.Fatalf("Dodec or observed services lost: %v", other)
	}
	mix := rows(other["observedEconomies"])
	if len(mix) != 2 || mix[0]["name"] != "Industrial" || value.Num(mix[0]["fraction"]) != .75 {
		t.Fatalf("actual economies were replaced by predictions: %v", mix)
	}
}
