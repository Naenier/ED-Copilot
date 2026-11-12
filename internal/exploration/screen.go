package exploration

// ScreenPath returns a bounded projection of the active expedition. It does
// not scan the journal index, include notes or expose body observations.
func (s *Service) ScreenPath(owner string) (string, string, [][2]float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws := s.workspace(owner, false)
	if ws == nil {
		return "", "", nil
	}
	for _, expedition := range ws.Expeditions {
		if expedition == nil || expedition.StartedAt == "" || expedition.EndedAt != "" {
			continue
		}
		const limit = 100
		points := make([][2]float64, 0, min(limit+1, len(expedition.Travel)))
		step := max(1, (len(expedition.Travel)+limit-1)/limit)
		for i, observation := range expedition.Travel {
			if len(observation.Coordinates) == 3 && (i%step == 0 || i == len(expedition.Travel)-1) {
				points = append(points, [2]float64{observation.Coordinates[0], observation.Coordinates[2]})
			}
		}
		return expedition.Name, expedition.ReturnSystem, points
	}
	return "", "", nil
}
