package colonization

import (
	"errors"
	"math"
	"path/filepath"
	"time"

	"github.com/Naenier/ED-Copilot/internal/value"
)

const maxSupplyReservations = 4000

type reservationTarget struct {
	project, building, commodity string
}

type reservationJournalPosition struct {
	at     time.Time
	file   string
	offset int64
}

func reservationPosition(raw M, at time.Time) reservationJournalPosition {
	file := text(raw["file"], 500)
	offset, known := value.Number(raw["offset"])
	if !known || file == "" || offset < 0 || offset >= 1<<53 || math.Trunc(offset) != offset {
		return reservationJournalPosition{}
	}
	return reservationJournalPosition{at: at, file: filepath.Base(file), offset: int64(offset)}
}

func (p reservationJournalPosition) before(other reservationJournalPosition) bool {
	if !p.at.Equal(other.at) {
		return p.at.Before(other.at)
	}
	if p.file != other.file {
		return p.file < other.file
	}
	return p.offset < other.offset
}

type supplyReservation struct {
	id, workspace               string
	target                      reservationTarget
	buildingName, name, system  string
	source, sourceID            string
	count, remaining, fulfilled float64
	status                      string
	createdAt, updatedAt        time.Time
	transitAt                   time.Time
	deliveryMarket              string
	deliveryThrough             float64
	deliveryKnown               bool
	deliveryPosition            reservationJournalPosition
}

func (r supplyReservation) public() M {
	row := M{
		"id": r.id, "projectId": r.target.project, "buildingId": r.target.building,
		"buildingName": r.buildingName, "commodity": r.target.commodity, "name": r.name, "targetSystem": r.system,
		"source": r.source, "count": r.count, "remaining": r.remaining,
		"fulfilled": r.fulfilled, "status": r.status, "inTransit": r.status == "active" && !r.transitAt.IsZero(),
		"createdAt": value.ISO(r.createdAt), "updatedAt": value.ISO(r.updatedAt),
	}
	if !r.transitAt.IsZero() {
		row["transitAt"] = value.ISO(r.transitAt)
	}
	return row
}

func (p *Planner) savedSupplyReservations() []M {
	saved := make([]M, 0, len(p.supplyReservations))
	for _, r := range p.supplyReservations {
		row := merge(r.public(), M{"workspace": r.workspace, "sourceId": r.sourceID, "deliveryMarket": r.deliveryMarket})
		if r.deliveryKnown {
			row["deliveryThrough"] = r.deliveryThrough
		}
		if pos := r.deliveryPosition; !pos.at.IsZero() {
			row["deliveryPosition"] = M{"at": value.ISO(pos.at), "file": pos.file, "offset": pos.offset}
		}
		saved = append(saved, row)
	}
	return saved
}

func (p *Planner) restoreSupplyReservations(raw any) {
	seen := map[string]bool{}
	for _, row := range limitedRows(raw, maxSupplyReservations) {
		count, countOK := reservationAmount(row["count"])
		remaining, remainingOK := reservationAmount(row["remaining"])
		fulfilled, fulfilledOK := reservationAmount(row["fulfilled"])
		source, sourceID := str(row["source"]), marketID(row["sourceId"])
		status := str(row["status"])
		r := supplyReservation{
			id: text(row["id"], 80), workspace: text(row["workspace"], 80),
			target:       reservationTarget{text(row["projectId"], 80), text(row["buildingId"], 80), value.Key(row["commodity"])},
			buildingName: text(row["buildingName"], 200), name: text(row["name"], 200),
			system: text(row["targetSystem"], 200), transitAt: value.Time(row["transitAt"]),
			source: source, sourceID: sourceID, count: count, remaining: remaining, fulfilled: fulfilled,
			status: status, createdAt: value.Time(row["createdAt"]), updatedAt: value.Time(row["updatedAt"]),
			deliveryMarket: marketID(row["deliveryMarket"]),
		}
		r.deliveryThrough, r.deliveryKnown = reservationAmount(row["deliveryThrough"])
		r.deliveryKnown = r.deliveryKnown && r.deliveryMarket != ""
		last := value.Obj(row["deliveryPosition"])
		r.deliveryPosition = reservationPosition(last, value.Time(last["at"]))
		if !countOK || count == 0 || !remainingOK || !fulfilledOK || remaining+fulfilled > count ||
			r.id == "" || seen[r.id] || r.workspace == "" || r.target.project == "" || r.target.building == "" ||
			r.target.commodity == "" || len(r.target.commodity) > 100 || r.createdAt.IsZero() ||
			!has([]string{"active", "fulfilled", "cancelled", "released"}, status) ||
			(status == "active") != (remaining > 0) ||
			source != "ship" && (source != "fleet" && source != "squadron" || sourceID == "") {
			continue
		}
		seen[r.id] = true
		if r.transitAt.Before(r.createdAt) {
			r.transitAt = time.Time{}
		}
		p.supplyReservations = append(p.supplyReservations, r)
	}
	p.releaseClosedSupplyReservations()
}

func reservationAmount(raw any) (float64, bool) {
	n, ok := value.Number(raw)
	return n, ok && n >= 0 && n <= 1e9 && math.Trunc(n) == n
}

func (p *Planner) supplyOwner(context M) (string, string, error) {
	fid := str(value.At(context, "commander", "fid"))
	if p.commander != "" && fid != "" && p.commander != fid {
		return "", "", errors.New("Профиль командира изменился.")
	}
	return text(context["planWorkspace"], 80), ownerKey(firstText(fid, p.commander)), nil
}

func ownsSupplyProject(project M, owner string) bool {
	projectOwner := str(value.At(project, "journalOwnership", "ownerKey"))
	return projectOwner == "" || projectOwner == owner
}

func supplyBuildingOpen(status string) bool {
	return status == "planned" || status == "building"
}

// UpdateSupplyReservation creates, dispatches or cancels a hold in the active
// commander workspace. Stock and construction deficits are checked under
// the same lock that commits the reservation.
func (p *Planner) UpdateSupplyReservation(context, request M) (M, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	workspace, owner, err := p.supplyOwner(context)
	if err != nil {
		return nil, err
	}
	if workspace == "" || owner == "" {
		return nil, errors.New("Для резерва нужен командир из журнала игры.")
	}
	action := str(request["action"])
	switch action {
	case "reserve":
		return p.reserveSupplyLocked(context, request, workspace, owner)
	case "cancel", "transit":
		for i := range p.supplyReservations {
			r := &p.supplyReservations[i]
			if r.id != str(request["id"]) || r.workspace != workspace || r.status != "active" {
				continue
			}
			if action == "cancel" {
				r.close("cancelled", p.now())
			} else {
				inTransit, ok := request["inTransit"].(bool)
				if !ok {
					return nil, errors.New("Укажите состояние партии.")
				}
				if inTransit != !r.transitAt.IsZero() {
					r.updatedAt = p.now()
					r.transitAt = time.Time{}
					if inTransit {
						r.transitAt = r.updatedAt
					}
				}
			}
			return r.public(), nil
		}
		return nil, errors.New("Действующий резерв не найден.")
	default:
		return nil, errors.New("Неизвестное действие с резервом.")
	}
}

func (p *Planner) reserveSupplyLocked(context, request M, workspace, owner string) (M, error) {
	target := reservationTarget{str(request["projectId"]), str(request["buildingId"]), value.Key(request["commodity"])}
	source := str(request["source"])
	count, valid := reservationAmount(request["count"])
	if !valid || count == 0 || target.commodity == "" || len(target.commodity) > 100 {
		return nil, errors.New("Укажите товар и положительное целое количество.")
	}
	if len(p.supplyReservations) >= maxSupplyReservations {
		return nil, errors.New("Достигнут предел записей о резервах.")
	}
	project := p.projectLocked(target.project)
	b := building(project, target.building)
	if target.project == "" || project == nil || !ownsSupplyProject(project, owner) ||
		project["status"] == "cancelled" || b == nil || !supplyBuildingOpen(str(b["status"])) || !supplyBuildingOpen(p.statusOf(b)) {
		return nil, errors.New("Выбранный объект строительства недоступен.")
	}
	market := str(b["marketId"])
	depot := p.depots[market]
	var requirement M
	for _, row := range buildingRequirements(project, b, depot, p.buildingContributions(b)) {
		if row["commodity"] == target.commodity {
			requirement = row
			break
		}
	}
	stock := p.readReservationStock(context)
	held := p.activeSupplyHolds(workspace, nil)
	if num(requirement["missing"])-held.targets[target] < count {
		return nil, errors.New("Количество превышает свободную потребность стройки.")
	}
	available := stock.available(source, target.commodity, held)
	if available < count {
		return nil, errors.New("Подтверждённого свободного запаса недостаточно.")
	}

	now := p.now()
	r := supplyReservation{
		id: value.ID(), workspace: workspace, target: target,
		buildingName: displayBuildingName(b), name: str(requirement["name"]), system: str(project["targetSystem"]),
		source: source, sourceID: stock.sourceIDs[source], count: count, remaining: count,
		status: "active", createdAt: now, updatedAt: now, deliveryMarket: market,
		deliveryThrough: num(requirement["delivered"]), deliveryKnown: depot != nil,
	}
	p.supplyReservations = append(p.supplyReservations, r)
	return r.public(), nil
}

func (r *supplyReservation) close(status string, now time.Time) {
	r.remaining, r.status, r.updatedAt = 0, status, now
}

func (p *Planner) releaseClosedSupplyReservations() {
	if len(p.supplyReservations) == 0 {
		return
	}
	projects := make(map[string]M, len(p.projects))
	for _, project := range p.projects {
		projects[str(project["id"])] = project
	}
	for i := range p.supplyReservations {
		r := &p.supplyReservations[i]
		if r.status != "active" {
			continue
		}
		project := projects[r.target.project]
		b := building(project, r.target.building)
		if project == nil || project["status"] == "cancelled" || b == nil || !supplyBuildingOpen(str(b["status"])) {
			r.close("released", p.now())
		}
	}
}

func (p *Planner) reconcileSupplyReservations(context M, market string, observedAt time.Time, contribution []M) {
	if len(p.supplyReservations) == 0 || value.Bool(context["side"]) {
		return
	}
	workspace, owner, err := p.supplyOwner(context)
	depot := p.depots[market]
	position := reservationPosition(context, observedAt)
	if err != nil || workspace == "" || owner == "" || depot == nil && (len(contribution) == 0 || position.at.IsZero()) {
		return
	}

	groups := map[string][]*supplyReservation{}
	var linked, project M
	for i := range p.supplyReservations {
		r := &p.supplyReservations[i]
		if r.workspace != workspace || r.status != "active" || observedAt.Before(r.createdAt.Truncate(time.Second)) {
			continue
		}
		if last := r.deliveryPosition; !last.at.IsZero() &&
			(observedAt.Before(last.at) || observedAt.Equal(last.at) && !last.before(position)) {
			continue
		}
		if depot == nil && observedAt.Before(r.createdAt) {
			continue
		}
		pj := p.projectLocked(r.target.project)
		b := building(pj, r.target.building)
		if pj == nil || !ownsSupplyProject(pj, owner) || b == nil || b["marketId"] != market {
			continue
		}
		linked, project = b, pj
		groups[r.target.commodity] = append(groups[r.target.commodity], r)
	}
	if linked == nil {
		return
	}
	if depot == nil {
		releaseDirectSupplyHolds(groups, market, contribution, position)
		return
	}

	for _, requirement := range buildingRequirements(project, linked, depot, p.buildingContributions(linked)) {
		group := groups[str(requirement["commodity"])]
		confirmed := num(requirement["delivered"])
		through, anchored := float64(0), false
		for _, r := range group {
			if r.deliveryKnown && r.deliveryMarket == market {
				through = math.Max(through, r.deliveryThrough)
				anchored = true
			}
		}
		if !anchored {
			through = confirmed
		}
		// When all holds have an anchor, each new cumulative tonne can
		// release only one hold. A new market link first establishes an anchor.
		advanceSupplyHolds(group, market, confirmed, through, observedAt, position)
	}
	if status := p.statusOf(linked); status == "complete" || status == "failed" {
		for _, group := range groups {
			for _, r := range group {
				if r.status == "active" {
					r.close("released", observedAt)
				}
			}
		}
	}
}

func advanceSupplyHolds(group []*supplyReservation, market string, confirmed, through float64, observedAt time.Time, position reservationJournalPosition) {
	delta := max0(confirmed - through)
	for _, r := range group {
		if r.deliveryKnown && r.deliveryMarket == market && delta > 0 {
			delta -= r.credit(delta, observedAt)
		}
		r.deliveryThrough = math.Max(through, confirmed)
		r.deliveryKnown, r.deliveryMarket = true, market
		if !position.at.IsZero() {
			r.deliveryPosition = position
		}
	}
}

func releaseDirectSupplyHolds(groups map[string][]*supplyReservation, market string, contribution []M, position reservationJournalPosition) {
	amounts := map[string]float64{}
	for _, row := range contribution {
		amounts[str(row["commodity"])] += num(row["count"])
	}
	for commodity, count := range amounts {
		delta := count
		for _, r := range groups[commodity] {
			if r.deliveryKnown && r.deliveryMarket == market {
				r.deliveryThrough += count
			}
			r.deliveryPosition = position
			delta -= r.credit(delta, position.at)
		}
	}
}

func (r *supplyReservation) credit(delta float64, at time.Time) float64 {
	credited := math.Min(delta, r.remaining)
	if credited <= 0 {
		return 0
	}
	r.remaining -= credited
	r.fulfilled += credited
	r.updatedAt = at
	if r.remaining == 0 {
		r.close("fulfilled", at)
	}
	return credited
}
