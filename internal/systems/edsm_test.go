package systems

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Naenier/ED-Copilot/internal/edsm"
)

type edsmTestSource struct {
	calls  []string
	system func(string) (edsm.Object, error)
	prefix func(string, int) ([]edsm.Object, error)
}

func (s *edsmTestSource) System(_ context.Context, query string) (edsm.Object, error) {
	s.calls = append(s.calls, "system:"+query)
	return s.system(query)
}

func (s *edsmTestSource) Prefix(_ context.Context, query string, limit int) ([]edsm.Object, error) {
	s.calls = append(s.calls, "prefix:"+query)
	return s.prefix(query, limit)
}

func TestEDSMFallbackPrefersUsefulGlobalMatchesAndCachesConfirmedResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "Sol":
			fmt.Fprint(w, `{"values":["Sol","Solati"]}`)
		case "Synthetic Al":
			fmt.Fprint(w, `{"values":["Unrelated Synthetic Alpha"]}`)
		default:
			fmt.Fprint(w, `{"values":[]}`)
		}
	}))
	defer server.Close()
	source := &edsmTestSource{prefix: func(_ string, _ int) ([]edsm.Object, error) {
		t.Fatal("short exact lookup used prefix enumeration")
		return nil, nil
	}, system: func(name string) (edsm.Object, error) {
		return edsm.Object{"name": name, "id64": json.Number("18446744073709551615"),
			"coords": map[string]any{"x": json.Number("0"), "y": json.Number("1.25"), "z": json.Number("-2")}}, nil
	}}
	o := options(server)
	o.EDSM = source
	c := NewClient(o)
	defer c.Close()
	if got := query(t, c, "Sol", 1); got.Source != "Spansh" || len(got.Suggestions) != 2 || len(source.calls) != 0 {
		t.Fatal("healthy Spansh match triggered EDSM work", got, source.calls)
	}
	for _, seq := range []uint64{2, 3} {
		got := query(t, c, "Synthetic Al", seq)
		if got.Source != "EDSM" || got.Status != "ready" || len(got.Suggestions) != 1 || got.Cached != (seq == 3) {
			t.Fatal("weak global match suppressed the confirmed EDSM exact system", got)
		}
		item := got.Suggestions[0]
		if item.Name != "Synthetic Al" || item.ID64 != "18446744073709551615" || item.Coordinates == nil ||
			item.Coordinates.X != 0 || item.Coordinates.Y != 1.25 || item.Coordinates.Z != -2 || item.Match != "exact" {
			t.Fatal("canonical fallback metadata was lost", item)
		}
	}
	if !reflect.DeepEqual(source.calls, []string{"system:Synthetic Al"}) {
		t.Fatal("fallback did not reuse the normalized autocomplete cache", source.calls)
	}
}

func TestEDSMFallbackDoesNotEnumerateBroadPrefixesAndPreservesSpanshErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	source := &edsmTestSource{prefix: func(q string, limit int) ([]edsm.Object, error) {
		if limit != MaxResults {
			t.Fatal("fallback prefix query was not capped", limit)
		}
		if q == "Specific Amber" {
			return []edsm.Object{
				{"name": "Specific Amber 2"}, {"name": "Unrelated Specific Amber"},
				{"name": "Specific Amber"}, {"name": "specific amber 2"},
			}, nil
		}
		return nil, edsm.ErrTooManyMatches
	}, system: func(_ string) (edsm.Object, error) {
		return nil, errors.New("secondary provider failed")
	}}
	o := options(server)
	o.EDSM = source
	c := NewClient(o)
	defer c.Close()
	first, err := c.Query(context.Background(), Request{FieldID: "field", Sequence: 1, Query: "Col 285 Sector"})
	if err == nil || first.Status != "rate-limited" || first.Source != "Spansh" || first.RetryAfterMS < 119000 {
		t.Fatal("provider failure lost its cooldown after empty fallback", first, err)
	}
	got := query(t, c, "Specific Amber", 1)
	if got.Source != "EDSM" || got.ErrorCode != "" || got.RetryAfterMS != 0 || len(got.Suggestions) != 2 ||
		got.Suggestions[0].Name != "Specific Amber" || got.Suggestions[1].Name != "Specific Amber 2" {
		t.Fatal("fallback failed while Spansh was rate-limited or admitted unrelated systems", got)
	}
	wide := query(t, c, "Another Specific", 2)
	if wide.Source != "EDSM" || wide.Status != "empty" || !wide.Limited || len(wide.Suggestions) != 0 {
		t.Fatal("oversized EDSM response was not represented as a query to refine", wide)
	}
	if !reflect.DeepEqual(source.calls, []string{"system:Col 285 Sector", "prefix:Specific Amber", "prefix:Another Specific"}) {
		t.Fatal("unexpected EDSM requests", source.calls)
	}
}
