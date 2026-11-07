package colonization

func isRowArray(v any) bool {
	switch v.(type) {
	case []M, []any:
		return true
	}
	return false
}
