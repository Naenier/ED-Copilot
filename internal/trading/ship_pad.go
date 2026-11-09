package trading

import (
	"sync"

	"github.com/Naenier/ED-Copilot/internal/data"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var shipPads = sync.OnceValue(func() map[string]string {
	pads := make(map[string]string)
	for _, ship := range data.Array("encyclopedia-ships.json") {
		pad := v.Str(ship["pad"])
		if pad != "S" && pad != "M" && pad != "L" {
			continue
		}
		for _, field := range []string{"key", "name", "symbol"} {
			if key := v.Key(ship[field]); key != "" {
				pads[key] = pad
			}
		}
	}
	return pads
})

// ShipPad returns the smallest compatible pad for the observed ship model.
func ShipPad(commander M) string {
	for _, field := range []string{"ship", "shipType"} {
		if pad := shipPads()[v.Key(commander[field])]; pad != "" {
			return pad
		}
	}
	return ""
}
