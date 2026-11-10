package systems

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/edsm"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// EDSMSource is shared with other EDSM features so they use the same request
// limits and cooldown after a rate limit response.
type EDSMSource interface {
	System(context.Context, string) (edsm.Object, error)
	Prefix(context.Context, string, int) ([]edsm.Object, error)
}

func hasPrefixMatch(rows []Suggestion, query string) bool {
	prefix := comparable(query)
	for _, row := range rows {
		if row.Match == "correction" || strings.HasPrefix(comparable(row.Name), prefix) {
			return true
		}
	}
	return false
}

// EDSM does not document a server-side limit for prefix results. Only ask for
// specific prefixes and use the single-system endpoint for broader inputs.
func useEDSMPrefix(query string) bool {
	if strings.ContainsAny(query, "%_*") || utf8.RuneCountInString(comparable(query)) < 8 {
		return false
	}
	words := strings.Fields(query)
	if len(words) == 1 {
		return true
	}
	tail := words[len(words)-1]
	if utf8.RuneCountInString(comparable(tail)) < 3 {
		return false
	}
	lower := strings.ToLower(query)
	if strings.HasSuffix(lower, " sector") || strings.Contains(lower, " sector ") {
		return strings.Contains(lower, " sector ") && strings.IndexFunc(tail, unicode.IsDigit) >= 0
	}
	return true
}

func (c *Client) edsmFallback(ctx context.Context, result Result) (Result, bool) {
	if c.edsm == nil || ctx.Err() != nil {
		return result, false
	}
	var rows []edsm.Object
	var err error
	query := result.Query
	correction := correctionQuery(query)
	prefix := correction == "" && useEDSMPrefix(query)
	if prefix {
		rows, err = c.edsm.Prefix(ctx, query, MaxResults)
	} else {
		if correction != "" {
			query = correction
		}
		var row edsm.Object
		row, err = c.edsm.System(ctx, query)
		if row != nil {
			rows = []edsm.Object{row}
		}
	}
	limited := errors.Is(err, edsm.ErrTooManyMatches)
	if err != nil && !limited {
		return result, false
	}
	names := make([]Suggestion, 0, min(len(rows), MaxResults))
	seen := map[string]bool{}
	normalizedQuery := key(query)
	for _, row := range rows {
		name, _ := row["name"].(string)
		normalized := key(name)
		if normalized == "" || !validText(name) || seen[normalized] {
			continue
		}
		if (!prefix && normalized != normalizedQuery) || (prefix && !strings.HasPrefix(normalized, normalizedQuery)) {
			continue
		}
		seen[normalized] = true
		item := Suggestion{Name: name}
		if id := v.Str(row["id64"]); id != "0" {
			if _, err := strconv.ParseUint(id, 10, 64); err == nil {
				item.ID64 = id
			}
		}
		coords := v.Obj(row["coords"])
		x, hasX := v.Number(coords["x"])
		y, hasY := v.Number(coords["y"])
		z, hasZ := v.Number(coords["z"])
		if hasX && hasY && hasZ && math.Abs(x) < 1e9 && math.Abs(y) < 1e9 && math.Abs(z) < 1e9 {
			item.Coordinates = &Coordinates{X: x, Y: y, Z: z}
		}
		names = append(names, item)
		if len(names) == MaxResults {
			break
		}
	}
	result.Suggestions = relevant(names, result.Query, correction != "")
	if len(result.Suggestions) == 0 && !limited {
		return result, false
	}
	result.Source = "EDSM"
	result.Status = "empty"
	if len(result.Suggestions) > 0 {
		result.Status = "ready"
	}
	result.Limited, result.ErrorCode, result.RetryAfterMS = limited, "", 0
	return result, true
}
