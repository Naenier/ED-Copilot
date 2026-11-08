package exobiology

import (
	"math"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func (s *Service) Apply(event, context M) {
	s.mu.Lock()
	defer s.mu.Unlock()
	typ := v.Str(event["event"])
	at := v.Str(event["timestamp"])
	cmd := v.Obj(context["commander"])
	if typ == "LoadGame" {
		fid := v.Str(event["FID"])
		if fid != "" {
			if s.commander != "" && s.commander != fid {
				s.reset()
			}
			s.commander = fid
		}
		return
	}
	if typ == "Status" {
		s.observeStatus(event, cmd)
		return
	}
	allowed := map[string]bool{"Location": true, "FSDJump": true, "CarrierJump": true, "Scan": true, "FSSBodySignals": true, "SAASignalsFound": true, "SAAScanComplete": true, "FSSDiscoveryScan": true, "FSSAllBodiesFound": true, "ScanOrganic": true, "SellOrganicData": true, "CodexEntry": true, "Disembark": true, "Disembarked": true, "Touchdown": true, "Liftoff": true, "SupercruiseEntry": true, "Died": true}
	if !allowed[typ] {
		return
	}
	id := eventIdentity(event, context)
	if s.seen[id] {
		return
	}
	s.seen[id] = true
	s.predictionRevision++
	if s.firstAt == "" && !v.Time(at).IsZero() {
		s.firstAt = at
	}
	address := v.First(event["SystemAddress"], cmd["systemAddress"])
	if v.Str(address) != "" {
		s.currentSystem = v.Str(address)
	}
	if sy := s.systems[v.Str(address)]; sy != nil {
		sy.Revision++
	}
	systemName := v.Text(event["StarSystem"], cmd["system"])
	if sy := s.systems[v.Str(address)]; sy != nil && systemName == "" {
		systemName = sy.Name
	}
	if typ == "Location" || typ == "FSDJump" || typ == "CarrierJump" {
		addr := v.Str(address)
		if addr != "" {
			sy := s.systems[addr]
			if sy == nil {
				sy = &system{Address: addr, BodyIDs: map[string]bool{}}
				s.systems[addr] = sy
			}
			if systemName != "" {
				sy.Name = systemName
			}
			if len(v.Arr(event["StarPos"])) == 3 {
				sy.Coordinates = v.Clone(event["StarPos"])
			}
		}
		if typ != "Location" {
			s.current = ""
			s.liveFix = nil
		}
	}
	if typ == "FSSAllBodiesFound" || typ == "FSSDiscoveryScan" {
		if v.Str(address) != "" && s.systems[v.Str(address)] == nil {
			s.systems[v.Str(address)] = &system{Address: v.Str(address), Name: systemName, BodyIDs: map[string]bool{}}
		}
		if sy := s.systems[v.Str(address)]; sy != nil {
			if typ == "FSSAllBodiesFound" {
				sy.Complete = true
			}
			if n, known := v.Number(v.First(event["Count"], event["BodyCount"])); known && n >= 0 {
				sy.BodyCount = n
			}
		}
		return
	}
	if typ == "SellOrganicData" {
		s.sell(event, at)
		return
	}
	if typ == "Died" {
		s.loseData(at)
		return
	}
	if typ == "SupercruiseEntry" || typ == "Liftoff" {
		s.liveFix = nil
		if typ == "SupercruiseEntry" {
			s.current = ""
		}
		return
	}
	ID := numericBodyID(v.First(event["BodyID"], event["Body"], cmd["bodyId"], cmd["bodyID"]))
	if ID == nil && s.current != "" {
		if b := s.bodies[s.current]; b != nil && b.Address == v.Str(address) {
			ID = b.ID
		}
	}
	name := v.Text(event["BodyName"])
	if name == "" {
		if _, ok := event["Body"].(string); ok && numericBodyID(event["Body"]) == nil {
			name = v.Str(event["Body"])
		}
	}
	if name == "" && typ != "Scan" {
		name = v.Text(cmd["body"])
	}
	b := s.ensureBody(address, ID, name, systemName, at)
	if b == nil {
		return
	}
	if sy := s.systems[b.Address]; sy != nil {
		sy.Revision++
	}
	if s.selected == "" {
		s.selected = b.Key
	}
	switch typ {
	case "Location":
		if event["BodyType"] == "Planet" || event["BodyType"] == "Moon" {
			s.current = b.Key
		}
	case "Scan":
		for k, value := range event {
			if value != nil && k != "event" {
				b.Scan[k] = v.Clone(value)
			}
		}
		if flag, ok := event["WasFootfalled"].(bool); ok {
			b.WasFootfalled = flag
			b.FootfallAt = at
		}
		b.Revision++
	case "FSSBodySignals", "SAASignalsFound":
		for _, r := range v.Rows(event["Signals"]) {
			key := strings.ToLower(v.Text(r["Type"], r["Type_Localised"]))
			if strings.Contains(key, "biological") {
				if n, ok := v.Number(r["Count"]); ok && n >= 0 {
					b.Signals = n
					b.FSSResolved = true
				}
			}
			if strings.Contains(key, "geological") {
				if n, ok := v.Number(r["Count"]); ok && n >= 0 {
					b.GeologicalSignals = n
				}
			}
		}
		if raw, exists := event["Genuses"]; exists {
			b.GenusHints = []M{}
			for _, g := range v.Rows(raw) {
				b.GenusHints = append(b.GenusHints, M{"genus": v.Text(g["Genus"], g["Genus_Localised"]), "name": v.Text(g["Genus_Localised"], g["Genus"])})
			}
		}
		if typ == "SAASignalsFound" {
			b.Mapped = true
		}
		b.Revision++
	case "SAAScanComplete":
		b.Mapped = true
		b.Revision++
	case "Disembark", "Disembarked":
		if event["OnPlanet"] == true && event["OnStation"] != true {
			s.current = b.Key
			if b.WasFootfalled == false || event["FirstFootfall"] == true || event["firstfootfall"] == true {
				b.OwnFirstFootfall = true
			}
			b.WasFootfalled = true
			b.FootfallAt = at
			b.Revision++
			if event["SRV"] == true && !v.Bool(context["replay"]) && s.hasLiveFix(b.Key) && fixIsFresh(s.liveFix, at) {
				s.addMark(v.Merge(v.Copy(s.liveFix), M{"id": id, "kind": "srv", "bodyKey": b.Key, "label": "ТРП", "at": at, "owner": s.commander}))
			}
		}
	case "Touchdown":
		s.current = b.Key
		if fix := fixFrom(event, b, at); fix != nil {
			s.addMark(v.Merge(fix, M{"id": id, "kind": "ship", "bodyKey": b.Key, "label": "Корабль", "at": at, "owner": s.commander}))
		}
	case "CodexEntry":
		s.codex(event, context, b, id, at)
	case "ScanOrganic":
		s.organic(event, context, b, id, at)
	}
}

func numericBodyID(raw any) any {
	n, ok := v.Number(raw)
	if !ok || n < 0 || n > 1e6 || math.Trunc(n) != n {
		return nil
	}
	return int(n)
}

func (s *Service) organic(event, context M, b *body, identity, at string) {
	kind := strings.ToLower(v.Str(event["ScanType"]))
	if kind != "log" && kind != "sample" && kind != "analyse" {
		s.alerts = appendBounded(s.alerts, M{"id": identity, "kind": "unknown-scan-type", "message": "Неизвестный тип органического сканирования; прогресс не изменён.", "at": at}, 100)
		return
	}
	entry := Catalog().ResolveOrganic(event)
	speciesID, label, english, genus := organicLabels(event, entry)
	organicKey := organicIdentity(event, entry)
	fullKey := b.Key + "|" + organicKey
	if !v.Bool(context["replay"]) && len(b.Confirmed) == 0 {
		s.sealPrediction(b, at)
	}
	previous := s.progress[fullKey]
	if previous == nil {
		previous = M{"key": fullKey, "bodyKey": b.Key, "speciesKey": organicKey, "speciesId": speciesID, "name": label, "name_ru": label, "name_en": english, "genus": genus, "sampleCount": 0, "startedAt": at, "phase": "seen"}
		s.progress[fullKey] = previous
	}
	previous["updatedAt"], previous["variant"], previous["genusSymbol"] = at, v.Text(event["Variant_Localised"], event["Variant"]), event["Genus"]
	knownCount := int(v.Num(previous["sampleCount"]))
	wasComplete := previous["completedAt"] != nil && previous["phase"] != "lost"
	if kind == "log" && knownCount < 3 && (previous["phase"] == "lost" || s.tracker != nil && s.tracker["key"] != fullKey) {
		previous["sampleCount"], previous["startedAt"] = 0, at
		knownCount = 0
	}
	if previous["runKey"] == nil || kind == "log" && knownCount == 0 {
		previous["runKey"] = fullKey + "@" + v.Str(previous["startedAt"])
	}
	if kind == "analyse" {
		previous["sampleCount"], previous["phase"], previous["completedAt"] = 3, "complete", at
		if !wasComplete {
			row := v.Copy(previous)
			row["system"], row["body"], row["systemAddress"], row["bodyId"] = b.System, b.Name, b.Address, b.ID
			row["firstFootfall"], row["baseCredits"] = b.OwnFirstFootfall, nil
			if entry != nil {
				row["baseCredits"] = entry.BaseCredits
			}
			s.pending = append(s.pending, row)
		}
	} else if event["WasLogged"] != true {
		if kind == "log" {
			if knownCount == 0 {
				previous["sampleCount"] = 1
			}
		} else if knownCount < 3 {
			previous["sampleCount"] = min(3, knownCount+1)
		}
		previous["phase"] = "tracking"
	}
	confirmation := v.Copy(previous)
	confirmation["scanType"] = kind
	b.Confirmed[normal(genus)] = confirmation
	b.Revision++
	s.current = b.Key
	s.updateTracker(event, context, b, previous, identity, at, entry)
	row := v.Copy(confirmation)
	row["id"], row["system"], row["body"], row["systemAddress"], row["bodyId"] = identity, b.System, b.Name, b.Address, b.ID
	s.discoveries = appendBounded(s.discoveries, row, 500)
	s.totals["discoveries"] = v.Int(s.totals["discoveries"]) + 1
	if kind == "log" || kind == "analyse" {
		s.scorePrediction(b, speciesID, genus, identity, at)
	}
}

func organicLabels(event M, entry *Species) (id, name, english, genus string) {
	if entry != nil {
		return entry.ID, entry.Name, entry.EnglishName, entry.Genus
	}
	name = v.Text(event["Variant_Localised"], event["Species_Localised"], "Неизвестный вид")
	english = name
	genus = v.Text(event["Genus_Localised"], "Неизвестный род")
	id = "unknown-" + normal(v.First(event["Species"], name))
	return
}
func organicIdentity(event M, entry *Species) string {
	if v.Str(event["Species"]) != "" {
		return strings.ToLower(v.Str(event["Species"]) + "|" + v.Str(event["Variant"]))
	}
	base := v.Text(event["Species_Localised"], event["Variant_Localised"])
	if entry != nil {
		base = entry.ID
	}
	return normal(base) + "|" + normal(v.First(event["Variant"], event["Variant_Localised"]))
}

func (s *Service) sell(event M, at string) {
	for _, bio := range v.Rows(event["BioData"]) {
		entry := Catalog().ResolveOrganic(bio)
		speciesID, name, english, genus := organicLabels(bio, entry)
		key := organicIdentity(bio, entry)
		index := -1
		for i, p := range s.pending {
			if p["speciesKey"] == key || (v.Str(bio["Variant"]) == "" && p["speciesId"] == speciesID) {
				index = i
				break
			}
		}
		row := M{"at": at, "name": name, "name_ru": name, "name_en": english, "genus": genus, "speciesId": speciesID, "marketId": event["MarketID"], "bodyKey": nil, "body": nil, "system": nil, "value": nil, "bonus": nil, "credits": nil}
		base, bok := v.Number(bio["Value"])
		bonus, xok := v.Number(bio["Bonus"])
		if bok && base >= 0 {
			row["value"] = base
		}
		if xok && bonus >= 0 {
			row["bonus"] = bonus
		}
		if bok && xok && base >= 0 && bonus >= 0 {
			row["credits"] = base + bonus
		}
		if index >= 0 {
			p := s.pending[index]
			for _, field := range []string{"bodyKey", "body", "system", "systemAddress", "bodyId"} {
				row[field] = p[field]
			}
			if progress := s.progress[v.Str(p["key"])]; progress != nil {
				progress["phase"] = "sold"
				progress["soldAt"] = at
				if b := s.bodies[v.Str(p["bodyKey"])]; b != nil {
					for genus, c := range b.Confirmed {
						if c["key"] == p["key"] {
							c["phase"], c["soldAt"] = "sold", at
							b.Confirmed[genus] = c
						}
					}
				}
			}
			s.pending = append(s.pending[:index], s.pending[index+1:]...)
		}
		s.sales = appendBounded(s.sales, row, 500)
		s.totals["salesCount"] = v.Int(s.totals["salesCount"]) + 1
		if n, ok := v.Number(row["credits"]); ok {
			s.totals["soldCredits"] = v.Num(s.totals["soldCredits"]) + n
		} else {
			s.totals["unknownSales"] = v.Int(s.totals["unknownSales"]) + 1
		}
	}
}

func (s *Service) loseData(at string) {
	for _, row := range s.pending {
		row = v.Copy(row)
		row["lostAt"], row["phase"] = at, "lost"
		s.discoveries = appendBounded(s.discoveries, row, 500)
	}
	s.pending = []M{}
	for _, p := range s.progress {
		if p["phase"] != "sold" {
			p["phase"] = "lost"
			p["sampleCount"] = 0
		}
	}
	s.tracker = nil
	for _, b := range s.bodies {
		for _, c := range b.Confirmed {
			if c["phase"] != "sold" {
				c["phase"], c["sampleCount"] = "lost", 0
			}
		}
	}
	s.liveFix = nil
}

func (s *Service) codex(event, context M, b *body, identity, at string) {
	if !strings.Contains(strings.ToLower(v.Str(event["Category"])), "biology") {
		return
	}
	entry := Catalog().ResolveOrganic(M{"name": v.Text(event["Name_Localised"], event["Name"])})
	if entry != nil {
		if !v.Bool(context["replay"]) && len(b.Confirmed) == 0 {
			s.sealPrediction(b, at)
		}
		if b.Confirmed[normal(entry.Genus)] == nil {
			b.Confirmed[normal(entry.Genus)] = M{"speciesId": entry.ID, "name": entry.Name, "name_en": entry.EnglishName, "genus": entry.Genus, "phase": "codex", "sampleCount": 0, "updatedAt": at}
		}
		b.Revision++
		if !v.Bool(context["replay"]) {
			s.scorePrediction(b, entry.ID, entry.Genus, identity, at)
		}
	}
	if fix := fixFrom(event, b, at); fix != nil {
		if !v.Bool(context["replay"]) && s.liveFix["bodyKey"] == b.Key && fixIsFresh(s.liveFix, at) {
			for _, key := range []string{"heading", "altitudeAboveGroundM", "pitch"} {
				fix[key] = s.liveFix[key]
			}
		}
		s.addCodexMark(v.Merge(fix, M{"id": identity, "bodyKey": b.Key, "kind": "codex", "label": v.Text(event["Name_Localised"], "Биологический сигнал"), "speciesId": func() any {
			if entry != nil {
				return entry.ID
			}
			return nil
		}(), "at": at, "locationBasis": "journal-position", "owner": s.commander}))
	}
}

func appendBounded(rows []M, row M, limit int) []M {
	rows = append(rows, row)
	if len(rows) > limit {
		return rows[len(rows)-limit:]
	}
	return rows
}
