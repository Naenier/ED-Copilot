//go:build windows

package nativeui

import (
	"strings"
)

// Protect only identity-bearing input fields. Ordinary UI captions and enum
// values must remain translatable, and no catalogue snapshot is needed.
func webInputIdentities(input M) []string {
	out := []string{}
	var visit func(M, int)
	visit = func(object M, depth int) {
		if depth > 4 || len(out) >= 64 {
			return
		}
		for key, value := range object {
			lower := strings.ToLower(key)
			if lower == "name" || lower == "title" || lower == "notes" || lower == "label" || lower == "station" || lower == "body" || lower == "faction" || strings.HasSuffix(lower, "system") || strings.HasSuffix(lower, "station") || lower == "source" || lower == "destination" {
				if text, ok := value.(string); ok && len(text) > 0 && len(text) <= 1024 {
					out = append(out, text)
				}
			}
			if len(out) >= 64 {
				return
			}
			if nested, ok := value.(map[string]any); ok {
				visit(nested, depth+1)
			}
		}
	}
	visit(input, 0)
	return out
}
