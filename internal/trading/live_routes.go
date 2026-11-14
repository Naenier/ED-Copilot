package trading

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const (
	runSource   = "source"
	runOutbound = "outbound"
	runReturn   = "return"
)

// LiveRoutes is owned by the app's domain lock. Each commander has one tracked
// route and the last completed round. Quote snapshots are stored separately.
type LiveRoutes struct {
	runs map[string]*liveRoute
}

type liveEndpoint struct {
	System        string `json:"system"`
	SystemAddress string `json:"systemAddress,omitempty"`
	Station       string `json:"station"`
	MarketID      string `json:"marketId,omitempty"`
}

type liveLeg struct {
	Buy       liveEndpoint `json:"buy"`
	Sell      liveEndpoint `json:"sell"`
	Key       string       `json:"key"`
	Name      string       `json:"name"`
	Quantity  float64      `json:"quantity"`
	BuyPrice  float64      `json:"buyPrice"`
	SellPrice float64      `json:"sellPrice"`
}

type liveFill struct {
	Quantity float64 `json:"quantity"`
	Total    float64 `json:"total"`
}

type liveTrade struct {
	Bought liveFill `json:"bought"`
	Sold   liveFill `json:"sold"`
}

type liveRound struct {
	Seconds float64      `json:"seconds"`
	Trades  [2]liveTrade `json:"trades"`
}

type liveRoute struct {
	ID         string       `json:"routeId"`
	Legs       []liveLeg    `json:"legs"`
	Phase      string       `json:"phase"`
	Docked     int          `json:"docked"`
	SelectedAt time.Time    `json:"selectedAt"`
	LastAt     time.Time    `json:"lastAt"`
	LastEvents []string     `json:"lastEvents,omitempty"`
	DepartedAt time.Time    `json:"departedAt"`
	Copied     bool         `json:"copied"`
	Returned   bool         `json:"returned"`
	Settling   bool         `json:"settling"`
	Trades     [2]liveTrade `json:"trades"`
	Rounds     int          `json:"completedRounds"`
	LastRound  liveRound    `json:"lastRound"`
}

func NewLiveRoutes(saved any) *LiveRoutes {
	s := &LiveRoutes{runs: map[string]*liveRoute{}}
	data, err := json.Marshal(saved)
	if err != nil || len(data) > 512<<10 {
		return s
	}
	var rows map[string]*liveRoute
	if json.Unmarshal(data, &rows) != nil {
		return s
	}
	for owner, run := range rows {
		if !validLiveText(owner) || run == nil || !validLiveText(run.ID) || run.SelectedAt.IsZero() ||
			run.LastAt.Before(run.SelectedAt) || !validLiveLegs(run.Legs) || run.Docked < 0 || run.Docked > 2 ||
			run.Rounds < 0 || run.LastRound.Seconds < 0 || len(run.LastEvents) > 128 {
			continue
		}
		if run.Phase == "completed" && len(run.Legs) == 1 {
			run.Phase = runReturn
		}
		switch run.Phase {
		case runSource, runOutbound, runReturn:
			s.runs[owner] = run
		}
	}
	return s
}

func (s *LiveRoutes) Export() M {
	return v.Obj(v.Clone(s.runs))
}

func (s *LiveRoutes) Clear(owner string) {
	delete(s.runs, owner)
}

func (s *LiveRoutes) Select(owner string, route, commander M, now time.Time) (string, error) {
	if !validLiveText(owner) {
		return "", errors.New("Сначала подключите журнал командира.")
	}
	id := v.Str(route["id"])
	if existing := s.runs[owner]; existing != nil && id != "" && existing.ID == id {
		if existing.Phase == runReturn {
			return existing.Legs[0].Buy.System, nil
		}
		return existing.Legs[0].Sell.System, nil
	}
	rows := v.Rows(route["legs"])
	if !validLiveText(id) || len(rows) == 0 || len(rows) > 2 || now.IsZero() {
		return "", errors.New("Выбранный рейс недоступен.")
	}
	if s.runs[owner] == nil && len(s.runs) >= 128 {
		return "", errors.New("Достигнут предел сохранённых торговых рейсов.")
	}
	legs := make([]liveLeg, 0, len(rows))
	for _, row := range rows {
		legs = append(legs, liveLeg{
			Buy: liveMarket(v.Obj(row["buy"])), Sell: liveMarket(v.Obj(row["sell"])),
			Key: v.Key(row["key"]), Name: v.Text(row["name"], Name(v.Str(row["key"]), nil)),
			Quantity: v.Num(row["quantity"]), BuyPrice: v.Num(row["buyPrice"]), SellPrice: v.Num(row["sellPrice"]),
		})
	}
	if !validLiveLegs(legs) {
		return "", errors.New("Выбранный рейс содержит некорректные точки или котировки.")
	}
	at := now.UTC().Truncate(time.Second)
	run := &liveRoute{ID: id, Legs: legs, Phase: runSource, SelectedAt: at, LastAt: at}
	current := liveMarket(M{"system": commander["system"], "station": commander["station"], "systemAddress": commander["systemAddress"], "marketId": commander["marketId"]})
	if v.Bool(commander["docked"]) && legs[0].Buy.matches(current) {
		run.Phase, run.Docked = runOutbound, 1
	}
	s.runs[owner] = run
	return legs[0].Sell.System, nil
}

func validLiveText(s string) bool {
	return strings.TrimSpace(s) != "" && len([]rune(s)) <= 240 && !strings.ContainsAny(s, "\r\n\x00")
}

func validLiveLegs(legs []liveLeg) bool {
	if len(legs) == 0 || len(legs) > 2 {
		return false
	}
	for _, leg := range legs {
		if !validLiveText(leg.Buy.System) || !validLiveText(leg.Buy.Station) || !validLiveText(leg.Sell.System) ||
			!validLiveText(leg.Sell.Station) || leg.Buy.matches(leg.Sell) || !validLiveText(leg.Key) ||
			!validLiveText(leg.Name) || leg.Quantity < 1 || leg.Quantity > 25000 || math.Trunc(leg.Quantity) != leg.Quantity ||
			leg.BuyPrice <= 0 || leg.SellPrice <= leg.BuyPrice || leg.SellPrice > 1e12 {
			return false
		}
	}
	return len(legs) == 1 || legs[0].Buy.matches(legs[1].Sell) && legs[0].Sell.matches(legs[1].Buy)
}

func liveMarket(m M) liveEndpoint {
	id := v.Text(m["marketId"], m["market_id"])
	address := v.Str(m["systemAddress"])
	if id == "0" {
		id = ""
	}
	if address == "0" {
		address = ""
	}
	return liveEndpoint{v.Str(m["system"]), address, v.Str(m["station"]), id}
}

func (point liveEndpoint) matchesSystem(observed liveEndpoint) bool {
	if point.SystemAddress != "" && observed.SystemAddress != "" {
		return point.SystemAddress == observed.SystemAddress
	}
	return strings.EqualFold(point.System, observed.System)
}

func (point liveEndpoint) matches(observed liveEndpoint) bool {
	if observed.System != "" && !point.matchesSystem(observed) {
		return false
	}
	if point.MarketID != "" && observed.MarketID != "" {
		return point.MarketID == observed.MarketID
	}
	return observed.System != "" && observed.Station != "" && strings.EqualFold(point.Station, observed.Station)
}

func (point liveEndpoint) view() M {
	return M{"system": point.System, "systemAddress": point.SystemAddress, "station": point.Station, "marketId": point.MarketID}
}

func (leg liveLeg) operation(buy bool) M {
	price := leg.SellPrice
	if buy {
		price = leg.BuyPrice
	}
	return M{"key": leg.Key, "name": leg.Name, "quantity": leg.Quantity, "price": price}
}

func (r *liveRoute) stop(index int, includeReturnSale bool) M {
	if index == 2 {
		view := r.Legs[0].Sell.view()
		view["sell"] = r.Legs[0].operation(false)
		if len(r.Legs) == 2 {
			view["buy"] = r.Legs[1].operation(true)
		}
		return view
	}
	view := r.Legs[0].Buy.view()
	view["buy"] = r.Legs[0].operation(true)
	if includeReturnSale && len(r.Legs) == 2 {
		view["sell"] = r.Legs[1].operation(false)
	}
	return view
}

func (s *LiveRoutes) Snapshot(owner string) M {
	run := s.runs[owner]
	if run == nil {
		return nil
	}
	profit := 0.0
	legs := make([]M, 0, len(run.Legs))
	for _, leg := range run.Legs {
		profit += leg.Quantity * (leg.SellPrice - leg.BuyPrice)
		legs = append(legs, M{"buy": leg.Buy.view(), "sell": leg.Sell.view(), "key": leg.Key, "name": leg.Name,
			"quantity": leg.Quantity, "buyPrice": leg.BuyPrice, "sellPrice": leg.SellPrice})
	}
	view := M{"routeId": run.ID, "phase": run.Phase, "legs": legs, "completedRounds": run.Rounds, "profitPerRound": profit}
	switch run.Phase {
	case runSource:
		view["target"] = run.stop(1, false)
	case runOutbound:
		view["target"] = run.stop(2, false)
	case runReturn:
		view["target"] = run.stop(1, true)
	}
	if run.Docked != 0 {
		view["atStation"] = run.stop(run.Docked, run.Returned)
	}
	if run.LastRound.Seconds > 0 {
		view["lastRoundSeconds"], view["lastRoundProfit"] = run.LastRound.Seconds, profit
		view["profitPerHour"] = profit * 3600 / run.LastRound.Seconds
		if actual, known := run.LastRound.actualProfit(len(run.Legs)); known {
			view["actualRoundProfit"] = actual
			view["actualProfitPerHour"] = actual * 3600 / run.LastRound.Seconds
		}
	}
	return view
}

func (round liveRound) actualProfit(loadedLegs int) (float64, bool) {
	total := 0.0
	for _, trade := range round.Trades[:loadedLegs] {
		if trade.Bought.Quantity <= 0 || trade.Bought.Quantity != trade.Sold.Quantity {
			return 0, false
		}
		total += trade.Sold.Total - trade.Bought.Total
	}
	return total, true
}

// Observe advances journal observations and returns only a fresh clipboard
// destination. System arrival queues the next system, while exact docking
// advances the displayed market and determines the lap duration.
func (s *LiveRoutes) Observe(owner string, event, context M) string {
	run := s.runs[owner]
	if run == nil || v.Bool(context["side"]) || v.Bool(event["Taxi"]) || v.Bool(event["Multicrew"]) {
		return ""
	}
	kind := v.Str(event["event"])
	switch kind {
	case "Docked", "Undocked", "Location", "FSDJump", "CarrierJump", "MarketBuy", "MarketSell", "Died":
	default:
		return ""
	}
	at := v.Time(event["timestamp"])
	replay := v.Bool(context["replay"])
	if at.IsZero() || at.Before(run.LastAt) || replay && !at.After(run.SelectedAt) {
		return ""
	}
	cmd := v.Obj(context["commander"])
	observed := liveMarket(M{"system": v.Text(event["StarSystem"], cmd["system"]), "systemAddress": v.Text(event["SystemAddress"], cmd["systemAddress"]),
		"station": v.Text(event["StationName"], cmd["station"]), "marketId": v.Text(event["MarketID"], cmd["marketId"])})
	if !run.recordEvent(at, event, context) {
		return ""
	}
	switch kind {
	case "MarketBuy", "MarketSell":
		run.recordFill(kind, event, observed)
	case "Undocked":
		run.depart(at, observed)
	case "Died":
		run.DepartedAt, run.Trades, run.Docked, run.Copied, run.Settling = time.Time{}, [2]liveTrade{}, 0, false, false
		run.Returned = false
		run.Phase = runSource
	case "Docked":
		return run.dock(at, observed, replay, true)
	case "Location":
		if v.Bool(event["Docked"]) {
			// Location can re-establish the current station after restart.
			// Logging in doesn't count as a completed trip or trigger paste.
			return run.dock(at, observed, true, false)
		}
		run.Docked, run.Returned, run.Settling = 0, false, false
	case "CarrierJump", "FSDJump":
		if run.Docked == 1 && run.Phase == runOutbound {
			// A carrier trip has no Undocked event; its jump completion
			// cannot stand in for departure time.
			run.DepartedAt = time.Time{}
		}
		run.Docked, run.Returned, run.Settling = 0, false, false
		if kind == "CarrierJump" && v.Bool(event["Docked"]) {
			return run.dock(at, observed, replay, true)
		}
		return run.arriveSystem(observed, replay)
	}
	return ""
}

func (r *liveRoute) recordEvent(at time.Time, event, context M) bool {
	if at.After(r.LastAt) {
		r.LastAt, r.LastEvents = at, nil
	}
	id := v.Str(context["file"])
	if id != "" && context["offset"] != nil {
		id += ":" + v.Str(context["offset"])
	} else {
		data, err := json.Marshal(event)
		if err != nil || len(data) > 4096 {
			return false
		}
		digest := sha256.Sum256(data)
		id = hex.EncodeToString(digest[:])
	}
	if slices.Contains(r.LastEvents, id) {
		return false
	}
	if len(r.LastEvents) >= 128 {
		r.LastEvents = r.LastEvents[1:]
	}
	r.LastEvents = append(r.LastEvents, id)
	return true
}

func (r *liveRoute) arriveSystem(observed liveEndpoint, replay bool) string {
	if r.Phase == runSource && r.Legs[0].Sell.matchesSystem(observed) && !r.Legs[0].Buy.matchesSystem(observed) {
		r.Phase = runOutbound
	}
	var reached, next liveEndpoint
	switch {
	case r.Phase == runOutbound:
		reached, next = r.Legs[0].Sell, r.Legs[0].Buy
	case r.Phase == runReturn:
		reached, next = r.Legs[0].Buy, r.Legs[0].Sell
	default:
		return ""
	}
	if !reached.matchesSystem(observed) || reached.matchesSystem(next) {
		return ""
	}
	return r.copyDestination(next, replay)
}

func (r *liveRoute) copyDestination(point liveEndpoint, replay bool) string {
	if replay || r.Copied {
		return ""
	}
	r.Copied = true
	return point.System
}

func (r *liveRoute) depart(at time.Time, observed liveEndpoint) {
	if r.Docked == 1 && r.Phase == runOutbound && r.Legs[0].Buy.matches(observed) {
		r.DepartedAt = at
	}
	r.Docked, r.Returned, r.Settling = 0, false, false
}

func (r *liveRoute) dock(at time.Time, observed liveEndpoint, replay, actualArrival bool) string {
	first, second := r.Legs[0].Buy, r.Legs[0].Sell
	switch {
	case first.matches(observed):
		r.Docked = 1
		switch r.Phase {
		case runSource:
			r.Phase, r.Copied = runOutbound, false
			return r.copyThenReset(second, replay)
		case runReturn:
			r.Returned = true
			if actualArrival && !r.DepartedAt.IsZero() && at.After(r.DepartedAt) {
				r.LastRound = liveRound{Seconds: at.Sub(r.DepartedAt).Seconds(), Trades: r.Trades}
				r.Rounds++
				r.Settling = len(r.Legs) == 2
			}
			r.Phase, r.DepartedAt, r.Trades = runOutbound, time.Time{}, [2]liveTrade{}
			return r.copyThenReset(second, replay)
		case runOutbound:
			// Returning to the source before the delivery ends the timing
			// attempt. Goods already bought can still be sent on the next.
			if actualArrival {
				r.DepartedAt = time.Time{}
			}
			r.Copied = false
		}
	case second.matches(observed):
		r.Docked = 2
		if r.Phase != runSource && r.Phase != runOutbound {
			return ""
		}
		if !actualArrival {
			r.DepartedAt = time.Time{}
		}
		r.Phase = runReturn
		return r.copyThenReset(first, replay)
	default:
		r.Docked, r.Returned, r.Settling = 0, false, false
	}
	return ""
}

func (r *liveRoute) copyThenReset(point liveEndpoint, replay bool) string {
	text := r.copyDestination(point, replay)
	r.Copied = false
	return text
}

func (r *liveRoute) recordFill(kind string, event M, observed liveEndpoint) {
	if v.Bool(event["BlackMarket"]) || v.Bool(event["StolenGoods"]) || v.Bool(event["IllegalGoods"]) {
		return
	}
	key := v.Key(event["Type"])
	count := v.Num(event["Count"])
	if count <= 0 || count > 25000 || math.Trunc(count) != count {
		return
	}
	for i, leg := range r.Legs {
		if key != leg.Key {
			continue
		}
		if kind == "MarketBuy" && leg.Buy.matches(observed) && (i == 0 && r.Phase == runOutbound || i == 1 && r.Phase == runReturn) {
			addLiveFill(&r.Trades[i].Bought, event["TotalCost"], event["BuyPrice"], count)
		}
		if kind == "MarketSell" && leg.Sell.matches(observed) {
			if i == 0 && r.Phase == runReturn {
				addLiveFill(&r.Trades[i].Sold, event["TotalSale"], event["SellPrice"], count)
			} else if i == 1 && r.Settling && r.Docked == 1 {
				addLiveFill(&r.LastRound.Trades[i].Sold, event["TotalSale"], event["SellPrice"], count)
			}
		}
	}
}

func addLiveFill(fill *liveFill, totalRaw, unitRaw any, count float64) {
	total, known := v.Number(totalRaw)
	if !known {
		if unit, known := v.Number(unitRaw); known && unit > 0 {
			total = unit * count
		}
	}
	if total > 0 && total <= 1e16 {
		fill.Quantity += count
		fill.Total += total
	}
}
