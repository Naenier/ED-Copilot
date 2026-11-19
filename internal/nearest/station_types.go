package nearest

import (
	"context"
	"errors"
	"slices"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	stationCarrier    = "Drake-Class Carrier"
	stationStronghold = "Stronghold Carrier"
	stationMegaship   = "Mega ship"
	stationTitan      = "Thargoid Titan"
)

var stationOrbitals = []string{"Coriolis Starport", "Orbis Starport", "Ocellus Starport", "Dodec Starport", "Asteroid base"}
var stationSurfacePorts = []string{"Dockable Planet Station", "Planetary Outpost", "Planetary Port"}
var stationSettlements = []string{"Settlement", "Surface Settlement"}
var stationDepots = []string{"Planetary Construction Depot", "Space Construction Depot"}

// Spansh exposes the hull of a starport, but describes outposts and settlements
// by a common type and their economic role. Role filters keep those fields separate.
type stationKind struct {
	types      []string
	economy    []string
	government string
	fixedName  string
	dockable   bool
	role       bool
}

var stationKinds = map[string]stationKind{
	"orbital-starports":        {types: stationOrbitals},
	"surface-ports":            {types: stationSurfacePorts},
	"surface-all":              {types: slices.Concat(stationSurfacePorts, stationSettlements, stationDepots[:1])},
	"odyssey-settlements":      {types: stationSettlements},
	"construction-depots":      {types: stationDepots},
	"mobile-ports":             {types: []string{stationCarrier, stationMegaship}},
	"dockable-megaships":       {types: []string{stationMegaship}, dockable: true},
	"rescue-megaships":         {types: []string{stationMegaship}, economy: []string{"Rescue"}},
	"prison-megaships":         {types: []string{stationMegaship}, economy: []string{"Prison"}},
	"military-megaships":       {types: []string{stationMegaship}, economy: []string{"Military"}, role: true},
	"stronghold-carriers":      {fixedName: stationStronghold},
	"civilian-outposts":        {types: []string{"Outpost"}, economy: []string{"Agriculture", "Tourism"}, role: true},
	"commercial-outposts":      {types: []string{"Outpost"}, economy: []string{"Service", "Private Enterprise"}, role: true},
	"industrial-outposts":      {types: []string{"Outpost"}, economy: []string{"Industrial", "Refinery"}, role: true},
	"military-outposts":        {types: []string{"Outpost"}, economy: []string{"Military"}, role: true},
	"mining-outposts":          {types: []string{"Outpost"}, economy: []string{"Extraction"}, role: true},
	"scientific-outposts":      {types: []string{"Outpost"}, economy: []string{"High Tech"}, role: true},
	"anarchy-outposts":         {types: []string{"Outpost"}, government: "Anarchy", role: true},
	"agricultural-settlements": {types: stationSettlements, economy: []string{"Agriculture"}, role: true},
	"industrial-settlements":   {types: stationSettlements, economy: []string{"Industrial", "Refinery"}, role: true},
	"mining-settlements":       {types: stationSettlements, economy: []string{"Extraction"}, role: true},
	"military-settlements":     {types: stationSettlements, economy: []string{"Military"}, role: true},
	"scientific-settlements":   {types: stationSettlements, economy: []string{"High Tech"}, role: true},
	"tourism-settlements":      {types: stationSettlements, economy: []string{"Tourism"}, role: true},
}

var knownStationTypes = slices.Concat(stationOrbitals, stationSurfacePorts, stationSettlements, stationDepots, []string{"Outpost", stationCarrier, stationMegaship})

func canonicalStationType(name string) string {
	name = strings.TrimSpace(name)
	switch strings.ToLower(name) {
	case "asteroid base", "asteroidbase":
		return "Asteroid base"
	case "coriolis", "coriolis starport":
		return "Coriolis Starport"
	case "dodec", "dodec starport":
		return "Dodec Starport"
	case "orbis", "orbis starport":
		return "Orbis Starport"
	case "ocellus", "ocellus starport":
		return "Ocellus Starport"
	case "megaship", "mega ship":
		return stationMegaship
	case "fleetcarrier", "fleet carrier", "drake-class carrier":
		return stationCarrier
	}
	for _, kind := range knownStationTypes {
		if strings.EqualFold(kind, name) {
			return kind
		}
	}
	return name
}

func findStationKind(name string) stationKind {
	if kind, ok := stationKinds[name]; ok {
		return kind
	}
	if name == "" {
		return stationKind{}
	}
	return stationKind{types: []string{canonicalStationType(name)}}
}

func stationTypeMatches(a, b string) bool {
	return strings.EqualFold(canonicalStationType(a), canonicalStationType(b))
}

func containsStationType(types []string, value string) bool {
	return slices.ContainsFunc(types, func(kind string) bool { return stationTypeMatches(kind, value) })
}

func stationTypeFilters(input M, filters M, kind stationKind) error {
	var excluded []string
	if v.Bool(input["ignoreCarriers"]) {
		excluded = append(excluded, stationCarrier)
	}
	switch input["surface"] {
	case "no":
		// Spansh indexes Powerplay strongholds as planetary outposts.
		if kind.fixedName != stationStronghold {
			filters["is_planetary"] = stationValue(false)
		}
		excluded = append(excluded, stationSurfacePorts...)
		excluded = append(excluded, stationSettlements...)
		excluded = append(excluded, "Planetary Construction Depot")
	case "legacy":
		excluded = append(excluded, stationSettlements...)
	}
	if len(kind.types) != 0 {
		types := make([]string, 0, len(kind.types))
		for _, name := range kind.types {
			if !containsStationType(excluded, name) {
				types = append(types, name)
			}
		}
		if len(types) == 0 {
			return errors.New("Тип станции исключён остальными фильтрами.")
		}
		filters["type"] = stationValue(types)
	} else if len(excluded) > 0 {
		filters["type"] = M{"value": excluded, "logic": "not"}
	}
	if len(kind.economy) != 0 {
		economies := kind.economy
		if selected := stationValues(input["economy"], 30); len(selected) != 0 {
			economies = nil
			for _, economy := range kind.economy {
				if slices.Contains(selected, economy) {
					economies = append(economies, economy)
				}
			}
			if len(economies) == 0 {
				return errors.New("Профиль станции исключён фильтром экономики.")
			}
		}
		filters["primary_economy"] = stationValue(economies)
	}
	if kind.government != "" {
		if selected := stationValues(input["government"], 30); len(selected) != 0 && !slices.Contains(selected, kind.government) {
			return errors.New("Профиль станции исключён фильтром правительства.")
		}
		filters["government"] = stationGroup(kind.government)
	}
	return nil
}

func stationSignalKind(kind string) string {
	switch kind {
	case "orbital-installations":
		return "installation"
	case "scannable-megaships":
		return "scannable-megaship"
	}
	return ""
}

func (c *Client) searchStationOrbitalSignals(ctx context.Context, q Query, signal string) (Result, error) {
	unavailable := Result{Note: "Для этих объектов в каталоге сигналов нет сведений о площадках, услугах, владельце и расстоянии от звезды."}
	for _, field := range []string{"minPad", "allegiance", "government", "economy", "owner", "ownerAllegiance", "ownerGovernment", "ownerStates", "services"} {
		if stationText(q.Filters[field]) != "" {
			return unavailable, nil
		}
	}
	_, hasArrival := v.Number(q.Filters["maxArrivalLs"])
	if hasArrival || v.Bool(q.Filters["ownerPlayerOnly"]) || v.Bool(q.Filters["ownerControllingOnly"]) {
		return unavailable, nil
	}
	return c.searchOrbitalSignals(ctx, q, signal)
}
