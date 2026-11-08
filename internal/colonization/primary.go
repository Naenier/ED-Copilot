package colonization

import (
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// PrimaryPortComplete reports whether the colony is established for the
// primary-claim deadline. Only an explicit primary or a confirmed real facility
// in an owned claim establishes completion. Filling the colonisation ship's
// depot does not establish a completed port or a building variant.
func PrimaryPortComplete(project M) bool {
	ownership := v.Obj(project["journalOwnership"])
	since := time.Time{}
	// Replayed completion from an earlier claim must not hide a new primary
	// construction deadline. The latest available lifecycle observation wins.
	for _, raw := range []any{project["claimedAt"], ownership["claimedAt"], ownership["beaconAt"], v.At(project, "claimObservation", "claimedAt"), v.At(project, "claimObservation", "beaconAt")} {
		if at := v.Time(raw); at.After(since) {
			since = at
		}
	}
	for _, b := range v.Rows(project["buildings"]) {
		if IsColonisationShip(b) || v.First(b["effectiveStatus"], b["status"]) != "complete" {
			continue
		}
		completedAt := v.Time(b["journalUpdatedAt"])
		if b["journalStatus"] == "complete" && !completedAt.IsZero() && completedAt.Before(since) {
			continue
		}
		if v.Bool(b["isPrimary"]) {
			return true
		}
		confirmedStation := b["journalSource"] == "station" && IsPermanentStation(b)
		confirmedConstruction := v.Bool(b["journalDiscovered"]) && primaryConstructionEvidence(b)
		if ownership["source"] == "journal-claim" && (confirmedStation || confirmedConstruction) && b["journalStatus"] == "complete" && !completedAt.IsZero() && !completedAt.Before(since) {
			return true
		}
	}
	return false
}

func primaryConstructionEvidence(building M) bool {
	if building["journalSource"] == "station" && IsPermanentStation(building) {
		return true
	}
	for _, field := range []string{"journalStationName", "name"} {
		name := strings.ToLower(strings.TrimSpace(v.Str(building[field])))
		for _, prefix := range []string{"planetary construction site:", "orbital construction site:", "surface construction site:"} {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(v.Str(building["journalStationType"]))) {
	case "planetaryconstructiondepot", "planetaryconstructionsite", "orbitalconstructionsite", "orbitalconstructiondepot":
		return true
	}
	return false
}
