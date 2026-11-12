package exploration

import (
	"math"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Body estimates use the community-measured base formula documented by
// MattG and EDCD/EDDI. They exclude speculative discovery and system bonuses,
// rank modifiers, station differences and payout deductions.
func estimate(b *body, mapped, efficient bool) int64 {
	if b.StarClass != "" && b.StellarMass != nil {
		constant := 1200.0
		switch {
		case b.StarClass == "N" || b.StarClass == "H":
			constant = 22628
		case b.StarClass == "SuperMassiveBlackHole":
			return 0
		case strings.HasPrefix(b.StarClass, "D"):
			constant = 14057
		}
		return int64(math.Round(constant * (1 + *b.StellarMass/66.25)))
	}
	if b.PlanetClass == "" || b.Mass == nil {
		return 0
	}
	base, terraform := 300.0, 93328.0
	switch v.Key(b.PlanetClass) {
	case "earthlikebody", "earthlikeworld", "earthlikeplanet", "waterworld":
		base, terraform = 64831, 116295
	case "ammoniaworld":
		base = 96932
	case "metalrichbody":
		base = 21790
	case "highmetalcontentbody", "highmetalcontentworld":
		base, terraform = 9654, 100677
	case "sudarskyclassigasgiant", "classigasgiant":
		base = 1656
	case "sudarskyclassiigasgiant", "classiigasgiant":
		base = 9654
	}
	if b.Terraformable {
		base += terraform
	}
	result := base * (1 + 0.56591828*math.Pow(*b.Mass, 0.2))
	if mapped {
		result = math.Max(result*(10.0/3.0), 500)
		result += math.Max(result*.3, 555)
		if efficient {
			result *= 1.25
		}
	}
	return int64(math.Round(math.Max(result, 500)))
}

func (ws *workspace) pendingAt(sy *system, b *body, at string) (int64, string) {
	paid, lost := latest(sy.SoldAt, ws.paidSystems[normalized(sy.Name)]), ws.LastLoss
	if at != "" {
		if !newer(at, paid) {
			paid = ""
		}
		if !newer(at, lost) {
			lost = ""
		}
	}
	soldScan := paid != "" && b.ScanFirst != "" && !newer(b.ScanFirst, paid)
	soldMap := paid != "" && b.MapFirst != "" && !newer(b.MapFirst, paid)
	scan, mapped := b.ScanPayable, b.MappedAt
	if at != "" && newer(scan, at) {
		scan = b.ScanFirst
	}
	if at != "" && newer(mapped, at) {
		mapped = b.MapFirst
	}
	if !soldMap && newer(mapped, lost) && (at == "" || !newer(mapped, at)) {
		value := estimate(b, true, b.Efficient)
		if soldScan {
			value = max(0, value-estimate(b, false, false))
		}
		return value, mapped
	}
	if !soldScan && newer(scan, lost) && (at == "" || !newer(scan, at)) {
		return estimate(b, false, false), scan
	}
	return 0, ""
}

func (ws *workspace) bodyStatus(sy *system, b *body) string {
	if _, at := ws.pendingAt(sy, b, ""); at != "" {
		return "onboard"
	}
	last := latest(b.ScanPayable, b.MappedAt)
	if last == "" {
		return ""
	}
	sale := latest(sy.SoldAt, ws.paidSystems[normalized(sy.Name)])
	if sale != "" && !newer(last, sale) {
		return "sold"
	}
	if ws.LastLoss != "" && !newer(last, ws.LastLoss) {
		return "lost"
	}
	// A repeated scan after sale does not put the same data back on board.
	if sale != "" {
		return "sold"
	}
	return ""
}
