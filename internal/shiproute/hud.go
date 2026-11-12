package shiproute

// HUD exposes just the next actionable waypoint, even for long routes.
func (s *Store) HUD(owner string) M {
	route := s.active(owner)
	if route == nil || route.Next >= len(route.Stops) {
		return nil
	}
	stop := route.Stops[route.Next]
	return M{"system": stop.System, "station": stop.Station, "action": stop.Action, "starKind": StarKind(stop.StarClass), "next": route.Next + 1, "total": len(route.Stops)}
}
