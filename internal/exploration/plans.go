package exploration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

const maxSurveyFile = 512 << 10
const maxWorkspacePlanBytes = 256 << 10

var proceduralName = regexp.MustCompile(`(?i)^(.+ [A-Z]{2}-[A-Z] [A-H])(?:([0-9]+)-)?([0-9]+)$`)

type boxel struct {
	prefix  string
	mass    int
	ordinal int
}

func parseBoxel(seed string) (boxel, bool) {
	parts := proceduralName.FindStringSubmatch(strings.Join(strings.Fields(seed), " "))
	if len(parts) != 4 {
		return boxel{}, false
	}
	n, err := strconv.ParseUint(parts[3], 10, 24)
	if err != nil {
		return boxel{}, false
	}
	prefix := parts[1]
	mass := int(strings.ToLower(prefix[len(prefix)-1:])[0] - 'a')
	if parts[2] != "" {
		index, err := strconv.ParseUint(parts[2], 10, 16)
		if err != nil {
			return boxel{}, false
		}
		if index != 0 {
			prefix += strconv.FormatUint(index, 10) + "-"
		}
	}
	return boxel{prefix: prefix, mass: mass, ordinal: int(n)}, true
}

func (b boxel) projection(seed *system) M {
	out := M{"prefix": b.prefix, "widthLy": 10 << b.mass, "seedOrdinal": b.ordinal, "enumeration": "candidate-range"}
	if seed == nil {
		return out
	}
	number, err := strconv.ParseUint(seed.Address, 10, 64)
	if err != nil || number>>55 != 0 || int(number&7) != b.mass {
		return out
	}
	m := b.mass
	ordinal := number >> (44 - 3*m)
	if ordinal != uint64(b.ordinal) {
		return out
	}
	// The ID64 stores Z, Y and X grid indices after the three mass bits.
	z := (number >> 3) & ((1 << (14 - m)) - 1)
	y := (number >> (17 - m)) & ((1 << (13 - m)) - 1)
	x := (number >> (30 - 2*m)) & ((1 << (14 - m)) - 1)
	out["grid"] = []uint64{x, y, z}
	return out
}

// Update changes only the active commander's exploration plans.
func (s *Service) Update(owner string, request M) (M, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ws := s.workspace(owner, true)
	if ws == nil {
		return nil, errors.New("Для сохранения нужен командир из журнала игры.")
	}
	kind := v.Str(request["action"])
	now := v.ISO(s.now())
	id := v.Str(request["id"])
	switch kind {
	case "save-expedition":
		exp, err := ws.saveExpedition(request, now)
		if err != nil {
			return nil, err
		}
		request = v.Merge(M{}, request, M{"expeditionId": exp.ID, "mode": "expeditions"})
	case "start-expedition", "finish-expedition", "remove-expedition":
		if err := ws.expeditionAction(kind, id, now); err != nil {
			return nil, err
		}
	case "save-survey":
		project, err := ws.saveSurvey(request, false, now)
		if err != nil {
			return nil, err
		}
		request = v.Merge(M{}, request, M{"surveyId": project.ID, "mode": "surveys"})
		ws.SelectedSurvey = project.ID
	case "import-survey":
		input, err := surveyImport(v.Str(request["text"]))
		if err != nil {
			return nil, err
		}
		project, err := ws.saveSurvey(input, true, now)
		if err != nil {
			return nil, err
		}
		request = v.Merge(M{}, request, M{"surveyId": project.ID, "mode": "surveys"})
		ws.SelectedSurvey = project.ID
	case "select-survey", "skip-survey-target":
		index := ws.surveyIndex(id)
		if index < 0 {
			return nil, errors.New("Обследование не найдено.")
		}
		if kind == "skip-survey-target" {
			found := false
			for i, item := range ws.Surveys[index].Targets {
				if normalized(item.System) == normalized(v.Str(request["systemName"])) {
					ws.Surveys[index].Targets[i].Skip = v.Bool(request["skip"])
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("Система не найдена в обследовании.")
			}
		}
		ws.SelectedSurvey = id
		request = v.Merge(M{}, request, M{"surveyId": id, "mode": "surveys"})
	case "remove-survey":
		index := ws.surveyIndex(id)
		if index < 0 {
			return nil, errors.New("Обследование не найдено.")
		}
		ws.Surveys = append(ws.Surveys[:index], ws.Surveys[index+1:]...)
		if ws.SelectedSurvey == id {
			ws.SelectedSurvey = ""
		}
	case "save-note":
		sy := ws.systemFor(request["systemAddress"], request["systemName"])
		if sy == nil {
			return nil, errors.New("Система не найдена в журнале.")
		}
		key := sy.Address
		if body := v.Str(request["bodyId"]); body != "" {
			if sy.Bodies[body] == nil {
				return nil, errors.New("Тело не найдено в журнале.")
			}
			key += "|" + body
		}
		notes := make(map[string]string, len(ws.Notes)+1)
		for id, note := range ws.Notes {
			notes[id] = note
		}
		if note := text(request["note"], 900); note != "" {
			if ws.Notes[key] == "" && len(ws.Notes) >= 400 {
				return nil, errors.New("Достигнут предел заметок.")
			}
			notes[key] = note
		} else {
			delete(notes, key)
		}
		if err := checkPlanSize(ws.Expeditions, ws.Surveys, notes); err != nil {
			return nil, err
		}
		ws.Notes = notes
	case "add-expedition-stop":
		index := ws.expeditionIndex(id)
		if index < 0 {
			return nil, errors.New("Экспедиция не найдена.")
		}
		exp := ws.Expeditions[index]
		sy := ws.systemFor(request["systemAddress"], request["systemName"])
		if sy == nil {
			return nil, errors.New("Система не найдена в журнале.")
		}
		if len(exp.Stops) >= maxTargets {
			return nil, errors.New("Достигнут предел остановок.")
		}
		for _, stop := range exp.Stops {
			if stop.Address == sy.Address || normalized(stop.System) == normalized(sy.Name) {
				return nil, errors.New("Эта остановка уже в экспедиции.")
			}
		}
		candidate := *exp
		candidate.Stops = append(append([]target(nil), exp.Stops...), target{System: sy.Name, Address: sy.Address, Note: text(request["note"], 240)})
		plans := append([]*expedition(nil), ws.Expeditions...)
		plans[index] = &candidate
		if err := checkPlanSize(plans, ws.Surveys, ws.Notes); err != nil {
			return nil, err
		}
		ws.Expeditions = plans
	case "save-finding-profile", "select-finding-profile", "remove-finding-profile":
		if err := ws.profileAction(kind, request); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("Неизвестное действие в исследованиях.")
	}
	return ws.snapshot(owner, request), nil
}

func (ws *workspace) systemFor(rawAddress, rawName any) *system {
	if id := address(rawAddress); id != "" {
		return ws.Systems[id]
	}
	return ws.Systems[ws.names[normalized(v.Str(rawName))]]
}

func (ws *workspace) expeditionIndex(id string) int {
	for i, row := range ws.Expeditions {
		if row != nil && row.ID == id {
			return i
		}
	}
	return -1
}
func (ws *workspace) surveyIndex(id string) int {
	for i, row := range ws.Surveys {
		if row != nil && row.ID == id {
			return i
		}
	}
	return -1
}

func (ws *workspace) saveExpedition(raw M, now string) (*expedition, error) {
	id, name := v.Str(raw["id"]), text(raw["name"], 100)
	if name == "" {
		return nil, errors.New("Введите название экспедиции.")
	}
	index := ws.expeditionIndex(id)
	if id != "" && index < 0 {
		return nil, errors.New("Экспедиция не найдена.")
	}
	if index < 0 && len(ws.Expeditions) >= maxExpeditions {
		return nil, errors.New("Достигнут предел экспедиций.")
	}
	days, valid := integer(raw["days"], 0, 3650)
	if raw["days"] != nil && !valid {
		return nil, errors.New("Укажите длительность от 0 до 3650 дней.")
	}
	stops, err := cleanTargets(raw["stops"], false)
	if err != nil {
		return nil, err
	}
	exp := &expedition{ID: v.ID(), Name: name, CreatedAt: now, Goal: text(raw["goal"], 240), Notes: text(raw["notes"], 1200),
		ReturnSystem: text(raw["returnSystem"], 200), Days: days, Stops: stops, travelIDs: map[string]bool{}}
	plans := append([]*expedition(nil), ws.Expeditions...)
	if index < 0 {
		plans = append(plans, exp)
	} else {
		previous := ws.Expeditions[index]
		exp.ID, exp.CreatedAt, exp.StartedAt, exp.EndedAt = previous.ID, previous.CreatedAt, previous.StartedAt, previous.EndedAt
		exp.Travel, exp.travelIDs = previous.Travel, previous.travelIDs
		plans[index] = exp
	}
	if err := checkPlanSize(plans, ws.Surveys, ws.Notes); err != nil {
		return nil, err
	}
	ws.Expeditions = plans
	return exp, nil
}

func (ws *workspace) expeditionAction(action, id, now string) error {
	index := ws.expeditionIndex(id)
	if index < 0 {
		return errors.New("Экспедиция не найдена.")
	}
	exp := ws.Expeditions[index]
	switch action {
	case "start-expedition":
		if exp.StartedAt != "" {
			return errors.New("Экспедиция уже началась.")
		}
		for _, existing := range ws.Expeditions {
			if existing != nil && existing.StartedAt != "" && existing.EndedAt == "" {
				return errors.New("Сначала завершите текущую экспедицию.")
			}
		}
		exp.StartedAt = now
	case "finish-expedition":
		if exp.StartedAt == "" || exp.EndedAt != "" {
			return errors.New("Экспедиция не активна.")
		}
		exp.EndedAt = now
	case "remove-expedition":
		ws.Expeditions = append(ws.Expeditions[:index], ws.Expeditions[index+1:]...)
	}
	return nil
}

func (ws *workspace) saveSurvey(raw M, imported bool, now string) (*survey, error) {
	id, name := v.Str(raw["id"]), text(raw["name"], 100)
	if imported {
		id = ""
	}
	if name == "" {
		return nil, errors.New("Введите название обследования.")
	}
	index := ws.surveyIndex(id)
	if id != "" && index < 0 {
		return nil, errors.New("Обследование не найдено.")
	}
	if index < 0 && len(ws.Surveys) >= maxSurveys {
		return nil, errors.New("Достигнут предел обследований.")
	}
	criterion := v.Text(raw["criterion"], "visit")
	if criterion != "visit" && criterion != "fss" && criterion != "mapping" {
		return nil, errors.New("Выберите критерий завершения обследования.")
	}
	mapping := []string{}
	seen := map[string]bool{}
	for _, value := range v.Arr(raw["mappingTypes"]) {
		key := v.Str(value)
		switch key {
		case "earthlike", "water", "ammonia", "terraformable", "all":
			if !seen[key] {
				mapping = append(mapping, key)
				seen[key] = true
			}
		default:
			return nil, errors.New("Выберите типы картографируемых тел.")
		}
	}
	if criterion == "mapping" && len(mapping) == 0 {
		return nil, errors.New("Выберите типы картографируемых тел.")
	}
	targets, err := cleanTargets(raw["targets"], imported)
	if err != nil {
		return nil, err
	}
	project := &survey{ID: v.ID(), Name: name, CreatedAt: now, Notes: text(raw["notes"], 1200), Criterion: criterion, MappingTypes: mapping, SkipVisited: v.Bool(raw["skipVisited"]),
		BoxelSeed: text(raw["boxelSeed"], 200), Targets: targets}
	if imported {
		if created := atTime(raw["createdAt"]); created != "" && !newer(created, now) {
			project.CreatedAt = created
		}
	}
	if project.BoxelSeed != "" {
		box, valid := parseBoxel(project.BoxelSeed)
		from, fromOK := integer(raw["sequenceFrom"], 0, 1<<24-1)
		to, toOK := integer(raw["sequenceTo"], 0, 1<<24-1)
		if !valid || !fromOK || !toOK || to < from || to-from >= maxTargets {
			return nil, errors.New("Введите имя процедурной системы и диапазон до 400 номеров.")
		}
		project.SequenceFrom, project.SequenceTo = from, to
		if len(targets) == 0 {
			for n := from; n <= to; n++ {
				project.Targets = append(project.Targets, target{System: box.prefix + strconv.Itoa(n), Candidate: true})
			}
		}
	}
	if len(project.Targets) == 0 {
		return nil, errors.New("Добавьте системы для обследования.")
	}
	plans := append([]*survey(nil), ws.Surveys...)
	if index < 0 {
		plans = append(plans, project)
	} else {
		project.ID = id
		previous := ws.Surveys[index]
		project.CreatedAt = previous.CreatedAt
		old := map[string]target{}
		for _, item := range previous.Targets {
			old[normalized(item.System)] = item
		}
		for i, item := range project.Targets {
			saved := old[normalized(item.System)]
			if saved.Address == "" || item.Address == "" || item.Address == saved.Address {
				if item.Address == "" {
					project.Targets[i].Address = saved.Address
				}
				if saved.Imported != nil {
					progress := *saved.Imported
					if !slices.Equal(previous.MappingTypes, project.MappingTypes) {
						progress.MappedAt = ""
					}
					project.Targets[i].Imported = &progress
				}
				project.Targets[i].Candidate = saved.Candidate
			}
		}
		plans[index] = project
	}
	if err := checkPlanSize(ws.Expeditions, plans, ws.Notes); err != nil {
		return nil, err
	}
	ws.Surveys = plans
	return project, nil
}

func checkPlanSize(expeditions []*expedition, surveys []*survey, notes map[string]string) error {
	plans := []M{}
	for _, exp := range expeditions {
		if exp != nil {
			plans = append(plans, expeditionPlan(exp))
		}
	}
	raw, err := json.Marshal(M{"expeditions": plans, "surveys": surveys, "notes": notes})
	if err != nil || len(raw) > maxWorkspacePlanBytes {
		return errors.New("Планы исследований достигли предела объёма. Сократите список целей или заметки.")
	}
	return nil
}

func cleanTargets(raw any, imported bool) ([]target, error) {
	out := []target{}
	seen, names := map[string]bool{}, map[string]bool{}
	rows := v.Arr(raw)
	if len(rows) > maxTargets {
		return nil, errors.New("Список может содержать до 400 систем.")
	}
	for _, value := range rows {
		m := v.Obj(value)
		name := text(v.Text(m["system"], value), 200)
		id := address(m["address"])
		if name == "" || v.Str(m["address"]) != "" && id == "" {
			return nil, errors.New("Укажите название и корректный ID64 системы.")
		}
		key := normalized(name)
		if names[key] || id != "" && seen[id] {
			return nil, errors.New("В списке есть повторяющиеся системы.")
		}
		names[key], seen[id] = true, true
		t := target{System: name, Address: id, Note: text(m["note"], 240), Skip: v.Bool(m["skip"]), Candidate: v.Bool(m["candidate"])}
		if imported {
			raw := v.Obj(m["progress"])
			if len(raw) == 0 {
				raw = v.Obj(m["importedProgress"])
			}
			if len(raw) > 0 {
				t.Imported = &stages{VisitedAt: atTime(raw["visitedAt"]), FSSAt: atTime(raw["fssAt"]), MappedAt: atTime(raw["mappedAt"])}
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func integer(raw any, lower, upper int) (int, bool) {
	n, valid := v.Number(raw)
	if !valid || n < float64(lower) || n > float64(upper) || math.Trunc(n) != n {
		return 0, false
	}
	return int(n), true
}

func (ws *workspace) profileAction(action string, raw M) error {
	id := v.Str(raw["id"])
	index := -1
	for i, row := range ws.FindingProfiles {
		if row.ID == id {
			index = i
			break
		}
	}
	switch action {
	case "save-finding-profile":
		if id != "" && index < 0 {
			return errors.New("Профиль находок не найден.")
		}
		if index < 0 && len(ws.FindingProfiles) >= 12 {
			return errors.New("Достигнут предел профилей находок.")
		}
		profile := findingProfile{ID: id, Name: text(raw["name"], 100), Rules: map[string]findingRule{}}
		for key, item := range v.Obj(raw["rules"]) {
			row := v.Obj(item)
			profile.Rules[key] = findingRule{Mode: v.Str(row["mode"]), Threshold: v.Num(row["threshold"]), Material: v.Str(row["material"]), Cooldown: v.Int(row["cooldown"])}
		}
		if index < 0 {
			profile.ID = v.ID()
		}
		profile = normalizedProfile(profile)
		if index < 0 {
			ws.FindingProfiles = append(ws.FindingProfiles, profile)
		} else {
			ws.FindingProfiles[index] = profile
		}
		ws.SelectedProfile = profile.ID
	case "select-finding-profile":
		if index < 0 {
			return errors.New("Профиль находок не найден.")
		}
		ws.SelectedProfile = id
	case "remove-finding-profile":
		if index < 0 || len(ws.FindingProfiles) == 1 {
			return errors.New("Оставьте хотя бы один профиль находок.")
		}
		ws.FindingProfiles = append(ws.FindingProfiles[:index], ws.FindingProfiles[index+1:]...)
		if ws.SelectedProfile == id {
			ws.SelectedProfile = ws.FindingProfiles[0].ID
		}
	}
	return nil
}

func surveyImport(raw string) (M, error) {
	if len(raw) > maxSurveyFile {
		return nil, errors.New("Файл обследования слишком большой.")
	}
	var envelope M
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(raw, "\ufeff")))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, errors.New("Некорректный JSON обследования.")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || envelope["format"] != "ed-copilot-survey" || v.Int(envelope["version"]) != 1 {
		return nil, errors.New("Неизвестный формат обследования.")
	}
	return v.Obj(envelope["survey"]), nil
}

// ExportSurvey exports only the selected plan and its observable stages.
func (s *Service) ExportSurvey(owner, id string) (M, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ws := s.workspace(owner, false)
	if ws == nil {
		return nil, errors.New("Обследование не найдено.")
	}
	i := ws.surveyIndex(id)
	if i < 0 {
		return nil, errors.New("Обследование не найдено.")
	}
	project := ws.Surveys[i]
	data := v.Obj(v.Clone(project))
	rows := []M{}
	for _, item := range project.Targets {
		row := v.Obj(v.Clone(item))
		delete(row, "importedProgress")
		progress, _, _, _, sy := ws.targetStages(item, project)
		if sy != nil {
			row["address"] = sy.Address
		}
		row["progress"] = progress
		rows = append(rows, row)
	}
	data["targets"] = rows
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(M{"format": "ed-copilot-survey", "version": 1, "survey": data}); err != nil {
		return nil, err
	}
	if output.Len() > maxSurveyFile {
		return nil, fmt.Errorf("Файл обследования превышает допустимый размер.")
	}
	return M{"text": output.String(), "name": "ED Copilot Survey.json"}, nil
}
