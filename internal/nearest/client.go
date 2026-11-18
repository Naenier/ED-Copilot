package nearest

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/edsm"
	"github.com/Naenier/ED-Copilot/internal/trading"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

type Query struct {
	Category, Origin, Name, Region, Sort string
	Page, Limit                          int
	Filters                              M
	Politics                             []M
	OriginCoords                         []float64
}

type Result struct {
	Rows    []M    `json:"rows"`
	Total   int    `json:"total"`
	HasMore bool   `json:"hasMore"`
	Note    string `json:"note,omitempty"`
	Link    string `json:"link,omitempty"`
}

type Client struct {
	HTTP    *http.Client
	BaseURL string
	EDSM    *edsm.Client
}

const DefaultLimit = 24

func New(httpClient *http.Client) *Client {
	client := &http.Client{}
	if httpClient != nil {
		copy := *httpClient
		client = &copy
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{HTTP: client, BaseURL: "https://spansh.co.uk/api"}
}

func validText(value string, max int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && !strings.ContainsFunc(value, unicode.IsControl)
}

func FromInput(input M, currentSystem string) (Query, error) {
	query := Query{
		Category: v.Str(input["category"]), Origin: strings.TrimSpace(v.Str(input["origin"])),
		Name: strings.TrimSpace(v.Str(input["name"])), Region: strings.TrimSpace(v.Str(input["region"])),
		Sort: v.Str(input["sort"]), Filters: v.Obj(input["filters"]), Page: v.Int(input["page"]), Limit: DefaultLimit,
	}
	if !validText(query.Name, 150) || !validText(query.Origin, 150) || !validText(query.Region, 100) || !validText(query.Sort, 40) ||
		query.Page < 0 || query.Page >= 100 || len(query.Filters) > 55 {
		return Query{}, errors.New("Проверьте параметры поиска.")
	}
	if query.Origin == "" && query.Region == "" {
		query.Origin = strings.TrimSpace(currentSystem)
	}
	if query.Origin == "" && query.Region == "" {
		return Query{}, errors.New("Укажите систему, от которой нужно искать.")
	}
	return query, nil
}

func (c *Client) request(ctx context.Context, path string, payload M) (M, error) {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "..") {
		return nil, errors.New("Некорректный адрес поиска.")
	}
	method := http.MethodGet
	if payload != nil {
		method = http.MethodPost
	}
	raw, err := trading.FetchJSON(ctx, c.HTTP, method, strings.TrimRight(c.BaseURL, "/")+u.String(), payload)
	if err != nil {
		return nil, err
	}
	data, ok := raw.(M)
	if !ok || data == nil {
		return nil, errors.New("Сервис вернул неизвестный формат.")
	}
	return data, nil
}

func (c *Client) Search(ctx context.Context, q Query) (Result, error) {
	var result Result
	var err error
	switch q.Category {
	case "stations", "outfitting":
		result, err = c.searchStations(ctx, q)
	case "systems", "bodies":
		result, err = c.searchSpace(ctx, q)
	case "factions", "signals", "misc":
		result, err = c.searchSpecial(ctx, q)
	default:
		return Result{}, errors.New("Выберите вкладку поиска.")
	}
	if result.Rows == nil {
		result.Rows = []M{}
	}
	if result.Link == "" {
		result.Link = InaraLink(q)
	}
	return result, err
}

func InaraLink(q Query) string {
	paths := map[string]string{
		"stations": "stations", "outfitting": "outfitting", "systems": "systems", "bodies": "bodies",
		"factions": "minorfactions", "signals": "signals", "misc": "misc",
	}
	path := paths[q.Category]
	if path == "" {
		return ""
	}
	params := url.Values{"ps1": {q.Origin}}
	if q.Category == "misc" {
		ids := map[string]string{"conflicts": "1", "restore": "6", "massacre": "5", "mining": "7", "multi-state": "8"}
		if id := ids[v.Str(q.Filters["kind"])]; id != "" {
			params.Set("pi20", id)
		}
	}
	return "https://inara.cz/elite/nearest-" + path + "/?" + params.Encode()
}
