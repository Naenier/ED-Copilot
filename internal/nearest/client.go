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
	HTTP             *http.Client
	BaseURL          string
	EDSM             *edsm.Client
	signalFeed       string
	missionSearchURL string
}

const (
	DefaultLimit = 24
	maxPages     = 100
)

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
		Sort: v.Str(input["sort"]), Filters: v.Copy(v.Obj(input["filters"])), Limit: DefaultLimit,
	}
	if input["page"] != nil {
		page, ok := v.Number(input["page"])
		if !ok || page < 0 || page >= maxPages || page != float64(int(page)) {
			return Query{}, errors.New("Проверьте номер страницы поиска.")
		}
		query.Page = int(page)
	}
	if !validText(query.Name, 150) || !validText(query.Origin, 150) || !validText(query.Region, 100) || !validText(query.Sort, 40) ||
		len(query.Filters) > 55 {
		return Query{}, errors.New("Проверьте параметры поиска.")
	}
	if err := validateNearestRanges(query); err != nil {
		return Query{}, err
	}
	if query.Origin == "" && query.Region == "" {
		query.Origin = strings.TrimSpace(currentSystem)
	}
	if query.Origin == "" && query.Region == "" {
		return Query{}, errors.New("Укажите систему, от которой нужно искать.")
	}
	return query, nil
}

func validateNearestRanges(q Query) error {
	for key, maxValue := range map[string]float64{
		"radiusLy": 50000, "maxArrivalLs": 1e8, "populationMin": populationLimit, "populationMax": populationLimit,
		"minGravity": 100, "maxGravity": 100, "minTemperature": 1e7, "maxTemperature": 1e7,
	} {
		if q.Filters[key] == nil || q.Filters[key] == "" {
			continue
		}
		number, ok := v.Number(q.Filters[key])
		if !ok || number < 0 || number > maxValue || key == "radiusLy" && number == 0 {
			return errors.New("Числовой фильтр находится вне допустимого диапазона.")
		}
	}
	for _, pair := range [][2]string{{"populationMin", "populationMax"}, {"minGravity", "maxGravity"}, {"minTemperature", "maxTemperature"}} {
		low, lowOK := v.Number(q.Filters[pair[0]])
		high, highOK := v.Number(q.Filters[pair[1]])
		if lowOK && highOK && low > high {
			return errors.New("Минимальное значение превышает максимальное.")
		}
	}
	low, _ := v.Number(q.Filters["populationMin"])
	high, highOK := v.Number(q.Filters["populationMax"])
	if q.Filters["population"] == "unpopulated" && low > 0 || q.Filters["population"] == "populated" && highOK && high < 1 {
		return errors.New("Диапазон населения не совпадает с выбранным типом системы.")
	}
	return nil
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
	if result.HasMore && q.Page+1 >= maxPages {
		result.HasMore = false
		result.Note = appendSearchNote(result.Note, "Доступны первые 100 страниц. Уточните условия поиска.")
	}
	if result.Link == "" {
		result.Link = InaraLink(q)
	}
	return result, err
}

func appendSearchNote(previous, next string) string {
	if previous != "" {
		return previous + " " + next
	}
	return next
}

func hasOrigin(q Query) bool {
	return q.Origin != "" || len(q.OriginCoords) == 3
}

func setNearestReference(payload M, q Query) {
	if len(q.OriginCoords) == 3 {
		payload["reference_coords"] = M{"x": q.OriginCoords[0], "y": q.OriginCoords[1], "z": q.OriginCoords[2]}
	} else if q.Origin != "" {
		payload["reference_system"] = q.Origin
	}
}

func nameCondition(name string) M {
	return M{"value": "*" + name + "*"}
}

func InaraLink(q Query) string {
	paths := map[string]string{
		"stations": "stations", "outfitting": "outfitting", "systems": "starsystems", "bodies": "bodies",
		"factions": "minorfactions", "signals": "signals", "misc": "misc",
	}
	path := paths[q.Category]
	if path == "" {
		return ""
	}
	params := url.Values{"ps1": {q.Origin}}
	if q.Category == "misc" {
		ids := map[string]string{"conflicts": "1", "restore": "6", "massacre": "9", "mining": "7", "multi-state": "10"}
		if id := ids[v.Str(q.Filters["kind"])]; id != "" {
			params.Set("pi20", id)
		}
	}
	if q.Category == "signals" && q.Filters["signal"] == "compromised-nav-beacon" {
		params.Set("pi30", "1")
		params.Set("formbrief", "1")
	}
	return "https://inara.cz/elite/nearest-" + path + "/?" + params.Encode()
}
