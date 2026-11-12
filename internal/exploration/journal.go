package exploration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// Apply consumes trusted player journal observations. Replay can rebuild the
// index, but it never produces a new overlay or speech notification.
func (s *Service) Apply(owner string, event, context M) []M {
	kind := v.Str(event["event"])
	switch kind {
	case "Location", "FSDJump", "CarrierJump", "Scan", "FSSDiscoveryScan", "FSSAllBodiesFound",
		"SAAScanComplete", "Touchdown", "SupercruiseExit", "SellExplorationData", "MultiSellExplorationData", "Died":
	default:
		return nil
	}
	at := atTime(event["timestamp"])
	if at == "" || v.Bool(context["side"]) || v.Bool(v.At(context, "game", "multicrew")) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ws := s.workspace(owner, true)
	if ws == nil {
		return nil
	}
	if kind == "SellExplorationData" || kind == "MultiSellExplorationData" {
		ws.sell(event, at)
		return nil
	}
	if kind == "Died" {
		vehicle := v.Str(v.At(context, "commander", "vehicle"))
		if vehicle == "ship" && !v.Bool(event["OnFoot"]) {
			ws.lose(at)
		}
		return nil
	}
	cmd := v.Obj(context["commander"])
	cmdAddress := address(cmd["systemAddress"])
	id := address(v.First(event["SystemAddress"], cmd["systemAddress"]))
	name := text(v.Text(event["StarSystem"], event["SystemName"]), 200)
	if name == "" && (id == "" || id == cmdAddress) {
		name = text(cmd["system"], 200)
	}
	if id == "" && name != "" {
		id = ws.names[normalized(name)]
	}
	sy := ws.observeSystem(id, name, at)
	if sy == nil {
		return nil
	}
	switch kind {
	case "Location", "FSDJump", "CarrierJump":
		sy.VisitedAt = latest(at, sy.VisitedAt)
		sy.FirstVisit = earliest(at, sy.FirstVisit)
		if coords := coordinates(event["StarPos"]); coords != nil {
			sy.Coordinates = coords
		}
		ws.recordTravel(sy, at)
	case "FSSDiscoveryScan", "FSSAllBodiesFound":
		if n := amount(v.First(event["BodyCount"], event["Count"]), 0, maxSystemBodies); n != nil && math.Trunc(*n) == *n {
			count := int(*n)
			sy.BodyCount = &count
		}
		if kind == "FSSAllBodiesFound" {
			sy.FSSAt = latest(at, sy.FSSAt)
		}
	case "Scan", "SAAScanComplete", "Touchdown", "SupercruiseExit":
		if kind == "Touchdown" && event["PlayerControlled"] == false {
			return nil
		}
		name := text(v.Text(event["BodyName"], event["Body"]), 240)
		b := ws.observeBody(sy, bodyID(v.First(event["BodyID"], event["Body"])), name)
		if b == nil {
			return nil
		}
		switch kind {
		case "Scan":
			if event["PlanetClass"] == nil && event["StarType"] == nil {
				return nil
			}
			newScan := newer(at, b.ScannedAt)
			b.ScannedAt = latest(at, b.ScannedAt)
			scanType := v.Key(event["ScanType"])
			if scanType != "navbeacondetail" && scanType != "navbeacon" {
				b.ScanFirst = earliest(at, b.ScanFirst)
				b.ScanPayable = latest(at, b.ScanPayable)
			}
			if b.ScannedAt != at {
				return nil
			}
			b.readScan(event)
			ws.updateRecords(sy, b)
			notify := newScan && !v.Bool(context["replay"])
			notices := ws.find(sy, b, at, notify)
			if len(b.Rings) > 0 {
				for _, child := range sy.Bodies {
					if child != nil && child.ParentID == b.ID && child != b {
						notices = append(notices, ws.find(sy, child, at, notify)...)
					}
				}
			}
			return notices
		case "SAAScanComplete":
			if b.MappedAt == "" || newer(at, b.MappedAt) {
				used, target := amount(event["ProbesUsed"], 1, 1e4), amount(event["EfficiencyTarget"], 1, 1e4)
				b.Efficient = used != nil && target != nil && *used <= *target
			}
			b.MapFirst = earliest(at, b.MapFirst)
			b.MappedAt = latest(at, b.MappedAt)
		default:
			b.VisitedAt = latest(at, b.VisitedAt)
		}
	}
	return nil
}

func (ws *workspace) observeSystem(id, name, at string) *system {
	if id == "" {
		return nil
	}
	sy := ws.Systems[id]
	if sy == nil {
		if len(ws.Systems) >= maxSystems && !ws.evict("") {
			return nil
		}
		sy = &system{Address: id, Name: name, Bodies: map[string]*body{}}
		ws.Systems[id] = sy
		ws.order = append(ws.order, id)
		if len(ws.order) >= 2*maxSystems {
			ws.compactOrder()
		}
	}
	if name != "" && (sy.Name == "" || newer(at, sy.LastAt)) {
		old := normalized(sy.Name)
		if ws.names[old] == id {
			delete(ws.names, old)
		}
		sy.Name = name
	}
	if sy.Name != "" {
		ws.names[normalized(sy.Name)] = id
		sy.SoldAt = latest(ws.paidSystems[normalized(sy.Name)], sy.SoldAt)
	}
	sy.LastAt = latest(at, sy.LastAt)
	return sy
}

func (ws *workspace) compactOrder() {
	order := make([]string, 0, len(ws.Systems))
	for offset := range ws.order {
		id := ws.order[(ws.oldest+offset)%len(ws.order)]
		if id != "" {
			order = append(order, id)
		}
	}
	ws.order, ws.oldest = order, 0
}

func (ws *workspace) observeBody(sy *system, id, name string) *body {
	key := id
	if key == "" && name == "" {
		return nil
	}
	if key == "" {
		key = "name:" + normalized(name)
		for _, b := range sy.Bodies {
			if b != nil && normalized(b.Name) == normalized(name) {
				return b
			}
		}
	}
	b := sy.Bodies[key]
	if b == nil && id != "" && name != "" {
		alias := "name:" + normalized(name)
		b = sy.Bodies[alias]
		if b != nil {
			delete(sy.Bodies, alias)
			b.ID, sy.Bodies[key] = id, b
		}
	}
	if b == nil {
		if len(sy.Bodies) >= maxSystemBodies || ws.bodies >= maxBodies && !ws.evict(sy.Address) {
			return nil
		}
		b = &body{ID: key, Name: name}
		sy.Bodies[key] = b
		ws.bodies++
	}
	if b.Name == "" {
		b.Name = name
	}
	return b
}

func (b *body) readScan(event M) {
	b.PlanetClass = v.Text(event["PlanetClass"], b.PlanetClass)
	b.StarClass = v.Text(event["StarType"], b.StarClass)
	if state, ok := event["TerraformState"].(string); ok {
		b.Terraformable = v.Key(state) == "terraformable"
	}
	if flag, ok := event["Landable"].(bool); ok {
		b.Landable = flag
	}
	for _, field := range []struct {
		name  string
		to    **float64
		limit float64
	}{
		{"MassEM", &b.Mass, 1e12}, {"StellarMass", &b.StellarMass, 1e9}, {"SurfaceTemperature", &b.Temperature, 1e10},
		{"SurfaceGravity", &b.Gravity, 1e10}, {"Radius", &b.Radius, 1e14}, {"DistanceFromArrivalLS", &b.Distance, 1e14},
		{"Eccentricity", &b.Eccentricity, 1}, {"SemiMajorAxis", &b.SemiMajorAxis, 1e18},
	} {
		if n := amount(event[field.name], 0, field.limit); n != nil {
			*field.to = n
		}
	}
	if parents := v.Rows(event["Parents"]); len(parents) > 0 {
		b.ParentID = bodyID(v.First(parents[0]["Planet"], parents[0]["Star"]))
	}
	if raw := v.Arr(event["Rings"]); raw != nil {
		b.Rings = []ring{}
		for _, row := range bounded(v.Rows(raw), 16) {
			inner, outer := amount(row["InnerRad"], 0, 1e16), amount(row["OuterRad"], 0, 1e16)
			if inner != nil && outer != nil && *inner < *outer {
				b.Rings = append(b.Rings, ring{Class: text(row["RingClass"], 40), Inner: *inner, Outer: *outer})
			}
		}
	}
	if event["Materials"] != nil {
		b.Materials = map[string]float64{}
		for key, value := range v.Obj(event["Materials"]) {
			b.addMaterial(key, value)
		}
		for _, row := range bounded(v.Rows(event["Materials"]), 40) {
			b.addMaterial(row["Name"], row["Percent"])
		}
	}
}

func (b *body) addMaterial(key, raw any) {
	material := v.Key(key)
	if n := amount(raw, 0, 100); n != nil && material != "" && len(material) <= 80 && len(b.Materials) < 40 {
		b.Materials[material] = *n
	}
}

func amount(raw any, min, max float64) *float64 {
	n, ok := v.Number(raw)
	if !ok || n < min || n > max {
		return nil
	}
	return &n
}

func coordinates(raw any) []float64 {
	arr := v.Arr(raw)
	if len(arr) != 3 {
		return nil
	}
	out := make([]float64, 3)
	for index, value := range arr {
		n := amount(value, -1e6, 1e6)
		if n == nil {
			return nil
		}
		out[index] = *n
	}
	return out
}

func (ws *workspace) recordTravel(sy *system, at string) {
	for _, exp := range ws.Expeditions {
		if exp == nil || exp.StartedAt == "" || newer(exp.StartedAt, at) ||
			exp.EndedAt != "" && newer(at, exp.EndedAt) {
			continue
		}
		if len(exp.Travel) == maxTravel {
			continue
		}
		id := sy.Address + "|" + at
		if exp.travelIDs[id] {
			continue
		}
		exp.travelIDs[id] = true
		exp.Travel = append(exp.Travel, travel{Address: sy.Address, System: sy.Name, At: at, Coordinates: sy.Coordinates})
	}
}

func observationID(data ...any) string {
	encoded, _ := json.Marshal(data)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
}

func systemNames(event M) []string {
	seen := map[string]bool{}
	names := []string{}
	add := func(raw any) {
		name := text(raw, 200)
		key := normalized(name)
		if key != "" && !seen[key] && len(names) < maxTargets {
			names = append(names, name)
			seen[key] = true
		}
	}
	for _, raw := range bounded(v.Arr(event["Systems"]), maxTargets) {
		add(raw)
	}
	for _, row := range bounded(v.Rows(event["Discovered"]), maxTargets) {
		add(row["SystemName"])
	}
	return names
}

func (ws *workspace) sell(event M, at string) {
	names := systemNames(event)
	id := observationID("sale", at, names, event["TotalEarnings"], event["BaseValue"], event["Bonus"])
	if ws.receipts[id] {
		return
	}
	r := receipt{ID: id, At: at, Kind: "sale", Systems: names}
	if credits := amount(event["TotalEarnings"], 0, 1e15); credits != nil {
		value := int64(*credits)
		r.Credits = &value
	}
	exactExpedition := len(names) > 0
	for _, name := range names {
		sy := ws.Systems[ws.names[normalized(name)]]
		if sy == nil {
			exactExpedition = false
			continue
		}
		matched := false
		for _, b := range sy.Bodies {
			if b == nil {
				continue
			}
			value, observed := ws.pendingAt(sy, b, at)
			if observed == "" {
				continue
			}
			matched = true
			if value > 0 {
				r.Estimated += value
				r.EstimatedBodies++
			}
			id := ws.expeditionAt(observed)
			if id == "" || r.ExpeditionID != "" && r.ExpeditionID != id {
				exactExpedition = false
			}
			if r.ExpeditionID == "" {
				r.ExpeditionID = id
			}
		}
		exactExpedition = exactExpedition && matched
	}
	if !exactExpedition {
		r.ExpeditionID = ""
	}
	ws.appendReceipt(r)
}

func (ws *workspace) lose(at string) {
	id := observationID("loss", at)
	if ws.receipts[id] {
		return
	}
	r := receipt{ID: id, At: at, Kind: "loss", ExpeditionID: ws.expeditionAt(at)}
	for _, sy := range ws.Systems {
		for _, b := range sy.Bodies {
			if b == nil {
				continue
			}
			if value, _ := ws.pendingAt(sy, b, at); value > 0 {
				r.Estimated += value
				r.EstimatedBodies++
			}
		}
	}
	ws.appendReceipt(r)
}

func (ws *workspace) expeditionAt(at string) string {
	for _, exp := range ws.Expeditions {
		if exp != nil && exp.StartedAt != "" && !newer(exp.StartedAt, at) &&
			(exp.EndedAt == "" || !newer(at, exp.EndedAt)) {
			return exp.ID
		}
	}
	return ""
}

func (ws *workspace) appendReceipt(r receipt) {
	ws.indexReceipt(r)
	ws.Receipts = append(ws.Receipts, r)
	if len(ws.Receipts) > maxReceipts {
		index := 0
		for i := 1; i < len(ws.Receipts); i++ {
			if newer(ws.Receipts[index].At, ws.Receipts[i].At) {
				index = i
			}
		}
		old := ws.Receipts[index]
		delete(ws.receipts, old.ID)
		ws.Receipts = append(ws.Receipts[:index], ws.Receipts[index+1:]...)
	}
}

func (ws *workspace) indexReceipt(r receipt) {
	ws.receipts[r.ID] = true
	if r.Kind == "loss" {
		ws.LastLoss = latest(r.At, ws.LastLoss)
		return
	}
	for _, name := range r.Systems {
		key := normalized(name)
		if key != "" {
			if sy := ws.Systems[ws.names[key]]; sy != nil {
				sy.SoldAt = latest(r.At, sy.SoldAt)
			}
			if ws.paidSystems[key] != "" || len(ws.paidSystems) < 2*maxSystems {
				ws.paidSystems[key] = latest(r.At, ws.paidSystems[key])
			}
		}
	}
}

func worldKind(b *body) string {
	switch v.Key(b.PlanetClass) {
	case "earthlikebody", "earthlikeworld", "earthlikeplanet":
		return "earthlike"
	case "waterworld", "waterplanet":
		return "water"
	case "ammoniaworld", "ammoniaplanet":
		return "ammonia"
	}
	return strings.ToLower(b.PlanetClass)
}
