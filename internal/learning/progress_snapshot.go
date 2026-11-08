package learning

import "math"

// ProgressSnapshot is the small dashboard summary. Gameplay/HUD refreshes need
// these counts, not independently decoded copies of every offline lesson.
func (l *Library) ProgressSnapshot() M {
	l.mu.RLock()
	defer l.mu.RUnlock()
	started, inProgress := 0, 0
	for id := range l.lessons {
		completed := l.completed[id] != nil
		position, _ := l.positions[id].(M)
		opened := l.opened[id] != nil || len(position) > 0
		if completed || opened {
			started++
		}
		if !completed && opened {
			inProgress++
		}
	}
	percent := 0
	if len(l.lessons) > 0 {
		percent = int(math.Round(float64(len(l.completed)) * 100 / float64(len(l.lessons))))
	}
	return M{"progress": M{"completed": len(l.completed), "total": len(l.lessons), "percent": percent, "started": started, "inProgress": inProgress, "bookmarked": len(l.bookmarks)}}
}
