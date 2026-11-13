package engineering

import "strings"

// Positions matched by system address against the Spansh system records.
// Keeping this small catalogue offline makes the visit order available as soon
// as the journal supplies the current system position.
var workshopSystems = map[string][3]float64{
	"achenar":          {67.5, -119.46875, 24.84375},
	"alioth":           {-33.65625, 72.46875, -20.65625},
	"arque":            {66.5, 38.0625, 61.125},
	"asura":            {-9550.28125, -916.65625, 19816.1875},
	"beta-3 tucani":    {32.25, -55.1875, 23.875},
	"deciat":           {122.625, -0.8125, -47.28125},
	"eurybia":          {51.40625, -54.40625, -30.5},
	"giryak":           {14.6875, 27.65625, 108.65625},
	"khun":             {-171.59375, 19.96875, -56.96875},
	"kuk":              {-21.28125, 69.09375, -16.3125},
	"kuwemaki":         {134.65625, -226.90625, -7.8125},
	"laksak":           {-21.53125, -6.3125, 116.03125},
	"leesti":           {72.75, 48.75, 68.25},
	"los":              {-9509.34375, -886.3125, 19820.125},
	"luchtaine":        {-9523.3125, -914.46875, 19825.90625},
	"meene":            {118.78125, -56.4375, -97.1875},
	"muang":            {17.03125, -172.78125, -3.46875},
	"shenve":           {351.96875, -373.46875, -711.09375},
	"shinrarta dezhra": {55.71875, 17.59375, 27.15625},
	"sirius":           {6.25, -1.28125, -5.75},
	"sol":              {0, 0, 0},
	"tir":              {-9532.9375, -923.4375, 19799.125},
	"wolf 397":         {40, 79.21875, -10.40625},
	"wyrd":             {-11.625, 31.53125, -3.9375},
	"yoru":             {97.875, -86.90625, 64.125},
}

func workshopCoordinates(system string) []float64 {
	position, ok := workshopSystems[strings.ToLower(strings.TrimSpace(system))]
	if !ok {
		return nil
	}
	return position[:]
}
