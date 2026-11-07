package colonization

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

type mockDoer func(*http.Request) (*http.Response, error)

func (f mockDoer) Do(r *http.Request) (*http.Response, error) { return f(r) }
func responseJSON(v any) *http.Response {
	b, _ := json.Marshal(v)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b)))}
}
func searchFixture(page func(int) M) HTTPClient {
	return mockDoer(func(r *http.Request) (*http.Response, error) {
		if e := r.Context().Err(); e != nil {
			return nil, e
		}
		if strings.Contains(r.URL.Path, "system_names") {
			return responseJSON(M{"min_max": []M{{"name": "Origin", "id64": json.Number("90071992547409999")}}}), nil
		}
		if strings.Contains(r.URL.Path, "/system/") {
			return responseJSON(M{"record": M{"name": "Origin", "id64": json.Number("90071992547409999"), "x": 0, "y": 0, "z": 0, "population": 100, "minor_faction_presences": []M{{"name": "Wanted"}, {"name": "Other"}}, "stations": []M{{"name": "Wanted port", "market_id": 123, "controlling_minor_faction": "Wanted", "type": "Coriolis", "services": []string{"systemcolonisation"}}, {"name": "Other port", "controlling_minor_faction": "Other", "type": "Outpost"}}}}), nil
		}
		var input M
		json.NewDecoder(r.Body).Decode(&input)
		return responseJSON(page(value.Int(input["page"]))), nil
	})
}
func candidate(id, name string, x int) M {
	return M{"name": name, "id64": id, "x": x, "y": 0, "z": 0, "population": 0, "needs_permit": false, "is_colonised": false, "is_being_colonised": false}
}
func TestCandidateAvailability(t *testing.T) {
	origin := M{"x": 0, "y": 0, "z": 0}
	base := candidate("123", "Candidate", 5)
	for _, tc := range []struct {
		name   string
		change M
		status string
	}{{"candidate", nil, "candidate"}, {"population", M{"population": 1}, "unavailable"}, {"permit", M{"needs_permit": true}, "unavailable"}, {"colonised", M{"is_colonised": true}, "unavailable"}, {"claim", M{"is_being_colonised": true}, "unavailable"}, {"station", M{"stations": []M{{"type": "Outpost"}}}, "unavailable"}, {"carrier", M{"stations": []M{{"type": "Fleet Carrier"}}}, "candidate"}, {"distant", M{"x": 16}, "unavailable"}, {"unknown population", M{"population": nil}, "unknown"}, {"missing coords", M{"x": nil}, "unknown"}} {
		t.Run(tc.name, func(t *testing.T) {
			row := ClassifyCandidate(merge(copyM(base), tc.change), origin, value.ISO(testNow))
			if str(row["availability"]) != tc.status {
				t.Fatalf("candidate=%#v", row)
			}
			if value.Bool(row["availabilityConfirmed"]) {
				t.Fatal("public search claimed authoritative availability")
			}
		})
	}
}
func TestSearchFullPaginationAndFactionStation(t *testing.T) {
	pages := []int{}
	client := searchFixture(func(page int) M {
		pages = append(pages, page)
		if page == 0 {
			return M{"count": 3, "results": []M{candidate("90071992547409999", "Origin", 0), candidate("2", "Near", 1)}}
		}
		return M{"count": 3, "results": []M{candidate("3", "Far", 10)}}
	})
	p := New(nil, WithHTTPClient(client))
	s, e := p.Search(context.Background(), M{"originSystem": "Origin", "faction": "Wanted"})
	if e != nil {
		t.Fatal(e)
	}
	if !value.Bool(value.Obj(s["completeness"])["complete"]) || len(rows(s["results"])) != 2 || len(rows(s["claimStations"])) != 1 || str(rows(s["claimStations"])[0]["name"]) != "Wanted port" {
		t.Fatalf("result=%#v", s)
	}
	if str(value.Obj(s["origin"])["id64"]) != "90071992547409999" {
		t.Fatal("uint64 system address lost precision")
	}
	if len(pages) != 2 || pages[0] != 0 || pages[1] != 1 {
		t.Fatalf("pages=%v", pages)
	}
	if _, e = p.Search(context.Background(), M{"originSystem": ""}); e == nil {
		t.Fatal("empty origin accepted")
	}
}
func TestSearchIncompletePagesAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		page      func(int) M
		wantError bool
	}{{"repeated", func(int) M { return M{"count": 5, "results": []M{candidate("1", "Near", 1), candidate("2", "Far", 2)}} }, true}, {"empty", func(int) M { return M{"count": 5, "results": []M{}} }, true}, {"changing catalog", func(page int) M {
		if page == 0 {
			return M{"count": 4, "results": []M{candidate("1", "Near", 1), candidate("2", "Far", 2)}}
		}
		return M{"count": 3, "results": []M{candidate("3", "Third", 3)}}
	}, false}, {"invalid count", func(int) M { return M{"count": "bad", "results": []M{}} }, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p := New(nil, WithHTTPClient(searchFixture(tc.page)))
			s, e := p.Search(context.Background(), M{"originSystem": "Origin"})
			if (e != nil) != tc.wantError {
				t.Fatalf("error=%v", e)
			}
			if value.Bool(value.Obj(s["completeness"])["complete"]) {
				t.Fatal("incomplete list marked complete")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		client HTTPClient
	}{{"http", mockDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("limited"))}, nil
	})}, {"invalid json", mockDoer(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("not json"))}, nil
	})}, {"no exact origin", mockDoer(func(*http.Request) (*http.Response, error) {
		return responseJSON(M{"min_max": []M{{"name": "Other", "id64": "1"}}}), nil
	})}} {
		t.Run(tc.name, func(t *testing.T) {
			p := New(nil, WithHTTPClient(tc.client))
			if _, e := p.Search(context.Background(), M{"originSystem": "Origin"}); e == nil {
				t.Fatal("invalid response accepted")
			}
			if str(value.Obj(p.Snapshot(nil)["search"])["status"]) != "error" {
				t.Fatal("error state missing")
			}
		})
	}
}
func TestSearchCancellationAndConcurrentSnapshots(t *testing.T) {
	entered := make(chan struct{})
	client := mockDoer(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	p := New(nil, WithHTTPClient(client))
	done := make(chan error, 1)
	go func() { _, e := p.Search(context.Background(), M{"originSystem": "Origin"}); done <- e }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("search did not start")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				p.Snapshot(nil)
				p.ExportSettings()
			}
		}()
	}
	p.CancelSearch()
	wg.Wait()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("search did not cancel")
	}
	s := value.Obj(p.Snapshot(nil)["search"])
	if str(s["status"]) != "cancelled" || value.Bool(s["running"]) {
		t.Fatal("cancel state missing")
	}
}
