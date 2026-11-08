package app

import (
	"errors"
	"math"

	"github.com/Naenier/ED-Copilot/internal/exobiology"
	"github.com/Naenier/ED-Copilot/internal/presentation"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// ExobiologySites queries the bounded public observation index, independently of
// the live journal service. The provider lacks Y coordinates, so returned
// distances remain explicitly lower bounds.
func (e *Engine) ExobiologySites(input M) (M, error) {
	radius := v.Num(input["radiusLy"])
	if math.IsNaN(radius) || math.IsInf(radius, 0) || radius < 0 || radius > 1000000 {
		return nil, errors.New("Некорректный радиус поиска")
	}
	e.mu.RLock()
	commander := v.Copy(e.commander)
	language := v.Str(e.settings["language"])
	e.mu.RUnlock()
	coordinates := []float64{}
	for _, x := range v.Arr(commander["coordinates"]) {
		if n, ok := v.Number(x); ok {
			coordinates = append(coordinates, n)
		}
	}
	limit := v.Int(input["limit"])
	if limit <= 0 || limit > 24 {
		limit = 12
	}
	result := exobiology.Catalog().Sites(exobiology.SiteQuery{Query: v.Str(input["query"]), SpeciesID: v.Str(input["speciesId"]), Region: v.Str(input["region"]), RadiusLy: radius, Coordinates: coordinates, Offset: v.Int(input["offset"]), Limit: limit})
	if language == "en" {
		for _, r := range v.Rows(result["entries"]) {
			for _, o := range v.Rows(r["observations"]) {
				o["name"] = v.Text(o["name_en"], o["name"])
			}
		}
	}
	return presentation.LocalizeReference(language, result), nil
}
