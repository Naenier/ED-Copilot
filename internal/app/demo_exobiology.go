package app

import (
	"errors"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// SeedDemoExobiology is a launcher-only synthetic journal fixture. It never
// alters the engine's commander, mining world or real journal files.
func (e *Engine) SeedDemoExobiology() error {
	if !e.opts.DisableNetwork {
		return errors.New("synthetic exobiology requires the isolated offline profile")
	}
	const address = "42424000024"
	const system = "Synthetic Exobiology"
	started := time.Now().UTC().Add(-time.Hour)
	index := 0
	apply := func(kind string, values M, body int, replay bool) {
		index++
		event := v.Merge(M{"event": kind, "timestamp": v.ISO(started.Add(time.Duration(index) * time.Second)), "SystemAddress": address}, values)
		context := M{"file": "Journal.SyntheticExobiology.log", "offset": index, "replay": replay, "commander": M{"system": system, "systemAddress": address, "bodyId": body, "body": system + " " + v.Str(body)}}
		e.Exobiology.Apply(event, context)
	}
	apply("LoadGame", M{"FID": "COPILOT-SYNTHETIC-EXOBIOLOGY"}, 0, false)
	apply("Location", M{"StarSystem": system, "StarPos": []float64{0, 0, 0}}, 0, false)
	for id := 1; id <= 2; id++ {
		apply("Scan", M{"StarSystem": system, "BodyID": id, "BodyName": system + " " + v.Str(id), "Landable": true, "PlanetClass": "Rocky body", "AtmosphereType": "CarbonDioxide", "Atmosphere": "Thin Carbon Dioxide", "SurfacePressure": 1700, "SurfaceTemperature": 175, "SurfaceGravity": 1.96133, "Radius": 1000000, "DistanceFromArrivalLS": 420 * id, "WasFootfalled": id == 2}, id, false)
		genera := []M{{"Genus": "Bacterium", "Genus_Localised": "Bacterium"}, {"Genus": "Stratum", "Genus_Localised": "Stratum"}}
		if id == 2 {
			genera = []M{{"Genus": "Tussock", "Genus_Localised": "Tussock"}}
		}
		apply("SAASignalsFound", M{"BodyID": id, "BodyName": system + " " + v.Str(id), "Signals": []M{{"Type": "$SAA_SignalType_Biological;", "Count": len(genera)}}, "Genuses": genera}, id, false)
	}
	organic := func(body int, species, genus, kind string, replay bool) {
		apply("ScanOrganic", M{"Body": body, "Species_Localised": species, "Genus_Localised": genus, "Genus": genus, "ScanType": kind}, body, replay)
	}
	apply("Disembark", M{"BodyID": 1, "BodyName": system + " 1", "OnPlanet": true}, 1, false)
	for _, kind := range []string{"Log", "Sample", "Analyse"} {
		organic(1, "Stratum tectonicas", "Stratum", kind, true)
	}
	apply("SellOrganicData", M{"MarketID": 4242401, "BioData": []M{{"Species_Localised": "Stratum tectonicas", "Genus_Localised": "Stratum", "Value": 19010800, "Bonus": 76043200}}}, 1, false)
	for _, kind := range []string{"Log", "Sample", "Analyse"} {
		organic(1, "Bacterium acies", "Bacterium", kind, true)
	}
	apply("Location", M{"StarSystem": system, "BodyID": 2, "BodyName": system + " 2", "BodyType": "Planet", "StarPos": []float64{0, 0, 0}}, 2, false)
	position := func(longitude float64) {
		apply("Status", M{"BodyName": system + " 2", "Latitude": 0, "Longitude": longitude, "PlanetRadius": 1000000}, 2, false)
	}
	apply("Touchdown", M{"BodyID": 2, "BodyName": system + " 2", "Latitude": 0, "Longitude": 0, "PlanetRadius": 1000000}, 2, false)
	position(0)
	organic(2, "Tussock pennata", "Tussock", "Log", false)
	position(0.02)
	organic(2, "Tussock pennata", "Tussock", "Sample", false)
	position(0.04)
	if err := e.Exobiology.Update(M{"action": "select-body", "bodyKey": address + ":2"}); err != nil {
		return err
	}
	e.Notify()
	return e.Save()
}
