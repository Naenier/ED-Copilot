package mining

import v "github.com/Naenier/ED-Copilot/internal/value"

// SnapshotForUI keeps run planning intact while avoiding the observed-world
// clone and full target search on screens which do not display targets.
func (s *Service) SnapshotForUI(context M, includeTargets bool) M {
	context = v.Merge(M{}, context)
	context["snapshotTargets"], context["snapshotWorld"] = includeTargets, false
	return s.Snapshot(context)
}

func (s *Service) SnapshotForStatistics(context M) M {
	context = v.Copy(context)
	context["snapshotTargets"], context["snapshotWorld"] = false, false
	return s.observedSnapshot(context)
}
