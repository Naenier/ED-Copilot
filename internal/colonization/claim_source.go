package colonization

import (
	"strings"

	"github.com/Naenier/ED-Copilot/internal/value"
)

// A missing service in community or journal observations does not prove that
// the colonisation contact is absent. Only an explicit boolean can deny it.
func colonisationContact(row M, services any) any {
	for _, key := range []string{"hasColonisationContact", "hasColonizationContact", "has_colonisation_contact", "has_colonization_contact", "HasColonisationContact", "HasColonizationContact"} {
		if known, ok := row[key].(bool); ok {
			return known
		}
	}
	for _, service := range stringsOf(services) {
		if colonisationService(service) {
			return true
		}
	}
	return nil
}

func placeholderFaction(name string) bool {
	return has([]string{"carrier", "fleetcarrier", "squadroncarrier", "strongholdcarrier", "megaship", "constructiondepot", "constructionsite", "depot"}, value.Key(name))
}

func claimSourceEligible(station M) bool {
	typ := value.Key(station["type"])
	if strings.Contains(typ, "carrier") || strings.Contains(typ, "megaship") || strings.Contains(typ, "construction") || strings.Contains(typ, "depot") {
		return false
	}
	if value.Bool(station["isCarrier"]) || value.Bool(station["constructionDepot"]) || value.Bool(station["invalidClaimFaction"]) || placeholderFaction(str(station["faction"])) {
		return false
	}
	if known, ok := station["hasColonisationContact"].(bool); ok && !known {
		return false
	}
	return true
}
