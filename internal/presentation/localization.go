package presentation

import (
	"regexp"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

func Language(state M) string {
	return localization.Normalize(v.Text(v.At(state, "settings", "language"), v.At(state, "app", "language")))
}

func Build(tab, mode, query string, state M) Page {
	page := buildPage(tab, mode, query, state)
	// Guide topics already select their language and search both bodies.
	if tab == "help" || Language(state) != "en" {
		return page
	}
	// English search must also work when the persisted catalogue's display
	// names are Russian. Keep canonical IDs and observations in their language.
	if query != "" && len(page.Rows) == 0 {
		page = buildPage(tab, mode, "", state)
	}
	page = LocalizePage(tab, page, state)
	if query != "" {
		rows := make([]Row, 0, len(page.Rows))
		for _, row := range page.Rows {
			values := make([]any, 0, len(row.Cells)+3)
			for _, cell := range row.Cells {
				values = append(values, cell)
			}
			values = append(values, row.Data["name"], row.Data["title"], row.Data["key"])
			if contains(query, values...) {
				rows = append(rows, row)
			}
		}
		page.Rows = rows
	}
	return page
}

func CoreActions(tab, mode string, state, selected, prefill M) []Action {
	actions := coreActions(tab, mode, state, selected, prefill)
	if tab == "settings" {
		actions = append([]Action{{ID: "settings.language", Title: "Язык интерфейса", Path: "/api/settings", Fields: []Field{{Key: "language", Label: "Язык интерфейса", Kind: "select", Default: Language(state), Choices: []Choice{{"ru", "Русский"}, {"en", "English"}}}}}}, actions...)
	}
	if Language(state) != "en" {
		return actions
	}
	out := append([]Action(nil), actions...)
	for i := range out {
		a := &out[i]
		a.Title = localization.Text("en", a.Title)
		a.Confirm = localization.Text("en", a.Confirm)
		a.Fields = append([]Field(nil), a.Fields...)
		for j := range a.Fields {
			f := &a.Fields[j]
			f.Label = localization.Text("en", f.Label)
			f.Help = localization.Text("en", f.Help)
			f.Choices = append([]Choice(nil), f.Choices...)
			for k := range f.Choices {
				f.Choices[k].Label = localization.Text("en", f.Choices[k].Label)
			}
		}
	}
	return out
}

var identityColumns = map[string]bool{"Планета": true, "Система": true, "Станция": true, "Тело": true, "Откуда": true, "Куда": true, "Система закупки": true, "Система продажи": true, "Станция закупки": true, "Станция продажи": true, "Исходная система": true, "Имя": true, "Командир": true, "Идентификатор": true, "Позывной": true, "Фракция": true}
var identityFieldLabels = map[string]bool{"Система": true, "Станция": true, "Тело": true, "Корабль": true, "Модель": true, "Личное имя": true, "Идентификатор": true, "Командир": true, "Название": true, "Исходная система": true, "Следующий прыжок": true}

func LocalizePage(tab string, page Page, state M) Page {
	if Language(state) != "en" {
		return page
	}
	originalColumns := page.Columns
	page.Title = localization.T("en", page.Title)
	page.Description = localization.Text("en", page.Description)
	page.Summary = localization.TextProtected("en", page.Summary, stateIdentities(state))
	page.Columns = append([]string(nil), page.Columns...)
	for i, c := range page.Columns {
		page.Columns[i] = localization.T("en", c)
		if tab == "exobiology" && c == "Вид" {
			page.Columns[i] = "Species"
		}
	}
	page.Rows = append([]Row(nil), page.Rows...)
	for i := range page.Rows {
		row := &page.Rows[i]
		row.Cells = append([]string(nil), row.Cells...)
		identities := rowIdentities(row.Data)
		originalFirst := ""
		if len(row.Cells) > 0 {
			originalFirst = row.Cells[0]
		}
		for j, cell := range row.Cells {
			column := ""
			if j < len(originalColumns) {
				column = originalColumns[j]
			}
			protected := identityColumns[column]
			if column == "Параметр" {
				protected = false
			}
			if column == "Значение" && identityFieldLabels[originalFirst] {
				protected = true
			}
			if column == "Объект" || column == "Название" || column == "Цель" {
				protected = tab == "targets" || tab == "overview" || tab == "colonization" || tab == "carrier" || tab == "route"
			}
			if column == "Объект" && tab == "colonization" {
				row.Cells[j] = localizedFacilityName(cell)
			} else if !protected {
				row.Cells[j] = localizedNumber(localization.TextProtected("en", cell, identities))
			}
		}
		row.Details = localization.TextProtected("en", row.Details, identities)
	}
	// Modes are stable internal tokens. The renderer translates their labels.
	return page
}

var numberPrefix = regexp.MustCompile(`^[-+]?\d+(?: \d{3})*(?:,\d+)?(?:$|[^\p{L}\p{N}])`)
var localDate = regexp.MustCompile(`^(\d{2})\.(\d{2})\.(\d{4})( \d{2}:\d{2})$`)

func localizedNumber(text string) string {
	if parts := localDate.FindStringSubmatch(text); parts != nil {
		return parts[3] + "-" + parts[2] + "-" + parts[1] + parts[4]
	}
	if prefix := numberPrefix.FindString(text); prefix != "" {
		numeric := strings.TrimRight(prefix, " ")
		converted := strings.ReplaceAll(numeric, ",", ".")
		converted = strings.ReplaceAll(converted, " ", ",")
		return converted + text[len(numeric):]
	}
	return text
}

func rowIdentities(row M) []string {
	out := []string{}
	for _, key := range []string{"system", "starSystem", "station", "sourceSystem", "destinationSystem", "sourceStation", "destinationStation", "originSystem", "targetSystem", "body", "bodyName", "callsign", "shipName", "shipIdent", "commanderName", "faction"} {
		if x := v.Str(row[key]); x != "" {
			out = append(out, x)
		}
	}
	if row["kind"] == "objective" || row["type"] == "objective" || row["targetSystem"] != nil || row["journalDiscovered"] != nil {
		for _, key := range []string{"name", "title"} {
			if x := v.Str(row[key]); x != "" {
				out = append(out, x)
			}
		}
	}
	return out
}

func stateIdentities(state M) []string {
	out := rowIdentities(v.Obj(state["commander"]))
	if name := v.Str(v.At(state, "commander", "name")); name != "" {
		out = append(out, name)
	}
	for _, key := range []string{"objectives", "bookmarks"} {
		for _, r := range v.Rows(state[key]) {
			out = append(out, rowIdentities(r)...)
			if name := v.Text(r["name"], r["title"]); name != "" {
				out = append(out, name)
			}
		}
	}
	for _, p := range v.Rows(v.At(state, "colonization", "projects")) {
		out = append(out, rowIdentities(p)...)
		if x := v.Str(p["name"]); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func localizedFacilityName(name string) string {
	for _, prefix := range []string{"Планетарная стройплощадка", "Орбитальная стройплощадка", "Поверхностная стройплощадка"} {
		if strings.HasPrefix(name, prefix+" · ") {
			return localization.T("en", prefix) + strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

func LocalizeHUD(doc HUDDocument, state M) HUDDocument {
	doc.Language = Language(state)
	if doc.Language != "en" || len(doc.Sections) == 0 {
		return doc
	}
	doc.Sections = append([]HUDSection(nil), doc.Sections...)
	ids := hudIdentities(state, doc.Activity)
	for i := range doc.Sections {
		section := &doc.Sections[i]
		section.Title = localization.Text("en", section.Title)
		section.Subtitle = localization.TextProtected("en", section.Subtitle, ids)
		section.Lines = append([]HUDLine(nil), section.Lines...)
		for j := range section.Lines {
			section.Lines[j].Text = localization.TextProtected("en", section.Lines[j].Text, ids)
			section.Lines[j].Value = localization.TextProtected("en", section.Lines[j].Value, ids)
		}
	}
	return doc
}

func hudIdentities(state M, activity string) []string {
	ids := stateIdentities(state)
	switch activity {
	case "carrier":
		c := v.Obj(state["carrier"])
		pending := v.Obj(v.At(c, "jump", "pending"))
		next := hudCarrierNextStop(v.Obj(state["carrierRoute"]))
		ids = append(ids, v.Text(c["callsign"], c["name"]), v.Str(v.At(c, "location", "system")),
			v.Text(pending["system"], pending["name"]), v.Text(next["name"], next["system"]))
	case "trade", "asteroid":
		ids = append(ids, rowIdentities(v.Obj(v.At(state, "trading", "destination")))...)
	case "exobiology":
		ex := v.Obj(state["exobiology"])
		ids = append(ids, v.Str(v.At(ex, "currentBody", "name")), v.Str(v.At(ex, "tracker", "bodyName")), v.Str(v.At(ex, "system", "name")))
		if targets := v.Rows(ex["targets"]); len(targets) > 0 {
			ids = append(ids, v.Str(targets[0]["name"]), v.Str(targets[0]["system"]))
		}
		for _, mark := range v.Rows(ex["surfaceMarks"]) {
			ids = append(ids, v.Str(mark["label"]))
		}
	}
	return ids
}

// LocalizeWebState receives an already copied/redacted response. Only known
// catalogue names and UI status prose are changed; editable plans and identity
// fields are never used as a generic translation surface.
func LocalizeWebState(state M) M {
	if Language(state) != "en" {
		return state
	}
	localizeRows := func(raw any) {
		for _, r := range v.Rows(raw) {
			for _, key := range []string{"name", "label", "description", "help", "categoryLabel", "priceSource"} {
				if text := v.Str(r[key]); text != "" {
					if key == "name" {
						r[key] = v.Text(r["english"], r["name_en"], r["englishName"], localization.Text("en", text))
					} else {
						r[key] = localization.Text("en", text)
					}
				}
			}
		}
	}
	for _, key := range []string{"goods", "resources"} {
		localizeRows(state[key])
	}
	engineering := v.Obj(state["engineering"])
	for _, key := range []string{"materials", "shopping", "needed", "exchanges"} {
		localizeRows(engineering[key])
	}
	for _, key := range []string{"materials", "blueprints", "effects", "modules", "engineers"} {
		localizeRows(v.At(engineering, "catalog", key))
	}
	localizeRows(v.At(state, "engineeringInventory", "materials"))
	for _, category := range []string{"raw", "manufactured", "encoded"} {
		localizeRows(v.At(state, "commanderProfile", "inventory", "materials", category, "entries"))
	}
	for _, r := range v.Rows(state["notices"]) {
		r["text"] = localization.Text("en", v.Str(r["text"]))
	}
	for _, key := range []string{"connection", "frontier", "integrations", "trading", "tradeRoutes", "search"} {
		m := v.Obj(state[key])
		for _, field := range []string{"error", "lastError", "statusText", "message"} {
			if text := v.Str(m[field]); text != "" {
				m[field] = localization.Text("en", text)
			}
		}
	}
	if route := v.Obj(state["carrierRoute"]); len(route) > 0 {
		for _, field := range []string{"assumptions", "warnings"} {
			items := v.Arr(route[field])
			for i, item := range items {
				if text, ok := item.(string); ok {
					items[i] = localization.TextProtected("en", text, stateIdentities(state))
				}
			}
			if route[field] != nil {
				route[field] = items
			}
		}
	}
	for _, project := range v.Rows(v.At(state, "colonization", "projects")) {
		localizeColonyPresentation(project)
	}
	localizeColonyPresentation(v.Obj(v.At(state, "colonization", "activeProject")))
	return state
}

func localizeColonyPresentation(project M) {
	ids := rowIdentities(project)
	if name := v.Str(project["name"]); name != "" {
		ids = append(ids, name)
	}
	for _, b := range v.Rows(project["buildings"]) {
		for _, key := range []string{"name", "displayName"} {
			if name := v.Str(b[key]); name != "" {
				b[key] = localizedFacilityName(name)
			}
		}
	}
	if next := v.Obj(project["nextTask"]); len(next) > 0 {
		next["text"] = localization.TextProtected("en", v.Str(next["text"]), ids)
	}
	for _, advice := range v.Rows(project["buildAdvice"]) {
		for _, field := range []string{"title", "text"} {
			if text := v.Str(advice[field]); text != "" {
				advice[field] = localization.TextProtected("en", text, ids)
			}
		}
	}
	if plan := v.Obj(project["economyPlan"]); len(plan) > 0 {
		for _, field := range []string{"name", "description"} {
			if text := v.Str(plan[field]); text != "" {
				plan[field] = localization.T("en", text)
			}
		}
	}
	for _, field := range []string{"warnings", "buildAdvice"} {
		switch values := project[field].(type) {
		case []string:
			out := append([]string(nil), values...)
			for i := range out {
				out[i] = localization.TextProtected("en", out[i], ids)
			}
			project[field] = out
		case []any:
			for i, r := range values {
				if text, ok := r.(string); ok {
					values[i] = localization.TextProtected("en", text, ids)
				}
			}
		}
	}
	for _, field := range []string{"operationalSupplies", "supplies", "aggregateSupplies"} {
		for _, r := range v.Rows(project[field]) {
			if name := v.Str(r["name"]); name != "" {
				r["name"] = localization.Text("en", name)
			}
		}
	}
}
