//go:build windows

package nativeui

import (
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"

	"github.com/Naenier/ED-Copilot/internal/exobiology"
	"github.com/Naenier/ED-Copilot/internal/frontend"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/Naenier/ED-Copilot/internal/reference"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

// The renderer receives serializable views and action descriptions. Raw engine
// methods, API keys and arbitrary filesystem paths are not exposed to scripts.
func dispatchWeb(backend Controller, op string, input M) (M, error) {
	tab, mode, query := v.Text(input["tab"], "overview"), v.Str(input["mode"]), v.Str(input["query"])
	valid := false
	for _, t := range p.Tabs {
		if t.ID == tab {
			valid = true
		}
	}
	if !valid {
		return nil, errors.New("Раздел не найден")
	}
	// Direct commands are validated by the backend. Building every domain and
	// every page before saving a checkbox made even small interactions wait on
	// the full observation catalogue and its global snapshot lock.
	switch op {
	case "rank.insignia":
		return frontend.RankInsignia(input)
	case "exo.sites":
		if provider, ok := backend.(interface{ ExobiologySites(M) (M, error) }); ok {
			return provider.ExobiologySites(input)
		}
		return M{"entries": []M{}, "total": 0, "limit": 12}, nil
	case "exobiology":
		return backend.Do("/api/exobiology", v.Obj(input["values"]))
	case "system-search":
		if provider, ok := backend.(interface{ QuerySystems(M) (M, error) }); ok {
			return provider.QuerySystems(input)
		}
		return M{"status": "disabled", "suggestions": []M{}, "minChars": 3}, nil
	case "system-search-cancel":
		if provider, ok := backend.(interface{ CancelSystems(M) (M, error) }); ok {
			return provider.CancelSystems(input)
		}
		return M{"ok": true}, nil
	case "deals-search":
		return backend.Do("/api/trading/routes", v.Obj(input["values"]))
	case "deals-cancel":
		return backend.Do("/api/trading/cancel", M{})
	case "deals-plan":
		return backend.Do("/api/trading/plan", v.Obj(input["values"]))
	case "deals-plan-clear":
		return backend.Do("/api/trading/plan/clear", M{})
	case "deals-destination":
		return backend.Do("/api/trading/destination", v.Obj(input["values"]))
	case "deals-saved-route":
		return backend.Do("/api/trading/saved-routes", v.Obj(input["values"]))
	case "carrier-trade":
		return backend.Do("/api/carrier/trade", v.Obj(input["values"]))
	case "carrier-trade-markets", "reference.market-search":
		return backend.Do("/api/trading/search", v.Obj(input["values"]))
	case "reference.markets":
		return v.Obj(snapshotForWeb(backend, "reference")["trading"]), nil
	case "reference.list":
		if input["category"] == "exobiology" {
			limit := v.Int(input["limit"])
			if limit <= 0 || limit > 12 {
				limit = 12
			}
			output := exobiology.Catalog().List(exobiology.Query{Category: "exobiology", Query: v.Str(input["query"]), Group: v.Str(input["group"]), Offset: v.Int(input["offset"]), Limit: limit})
			for _, entry := range v.Rows(output["entries"]) {
				entry["image"] = exobiology.SpeciesImage(strings.TrimPrefix(v.Str(entry["id"]), "exo/"))
			}
			return p.LocalizeReference(webLanguage(backend), output), nil
		}
		result := reference.Catalog().List(reference.Query{Category: v.Text(input["category"], "articles"), Query: v.Str(input["query"]), Group: v.Str(input["group"]), Offset: v.Int(input["offset"]), Limit: v.Int(input["limit"])})
		output := v.Obj(v.Clone(result))
		output["personal"] = snapshotForWeb(backend, "sync")["engineering"]
		return p.LocalizeReference(webLanguage(backend), output), nil
	case "reference.entry":
		id := v.Str(input["id"])
		if strings.HasPrefix(id, "exo/") {
			if entry, found := exobiology.Catalog().Entry(id); found {
				return p.LocalizeReference(webLanguage(backend), entry), nil
			}
			return nil, errors.New("Материал не найден в справочнике")
		}
		entry, found := reference.Catalog().Entry(id)
		result := v.Obj(v.Clone(entry))
		if found && entry.Category == "systems" {
			result = mergeReferenceSystem(backend, result)
		}
		if !found && strings.HasPrefix(id, "system/") {
			if provider, ok := backend.(interface{ ReferenceSystem(string) M }); ok {
				name, _ := url.PathUnescape(strings.TrimPrefix(id, "system/"))
				result = provider.ReferenceSystem(name)
				found = len(result) > 0
			}
		}
		if !found {
			return nil, errors.New("Материал не найден в справочнике")
		}
		result["personal"] = snapshotForWeb(backend, "sync")["engineering"]
		return p.LocalizeReference(webLanguage(backend), result), nil
	case "navigation":
		return backend.Do("/api/navigation", M{"tabs": input["tabs"]})
	case "settings":
		return backend.Do("/api/settings", v.Obj(input["values"]))
	case "desktop":
		return backend.Do("/api/desktop", v.Obj(input["values"]))
	case "theme-apply":
		return backend.Do("/api/theme", M{"settings": v.Obj(input["values"])})
	case "politics":
		return backend.Do("/api/politics", v.Obj(input["values"]))
	case "politics-refresh":
		return backend.Do("/api/politics/refresh", v.Obj(input["values"]))
	case "updates-check":
		return backend.Do("/api/updates/check", nil)
	case "updates-dismiss":
		return backend.Do("/api/updates/dismiss", nil)
	case "learning":
		return backend.Do("/api/learning", v.Obj(input["values"]))
	case "calculator":
		return backend.Do("/api/carrier/calculator", v.Obj(input["values"]))
	case "select-colony":
		return backend.Do("/api/colonization", M{"action": "select-project", "id": input["id"]})
	case "boot", "view", "action":
		// These operations use the current screen's snapshot below.
	case "catalog":
		state := snapshotForWeb(backend, "engineering")
		return M{"engineering": v.At(state, "engineering", "catalog")}, nil
	default:
		return nil, errors.New("Неизвестный запрос интерфейса")
	}
	if op == "view" && (tab == "encyclopedia" || (tab == "engineering" || tab == "targets") && mode == "Инженеры") {
		state := snapshotForWeb(backend, "reference")
		modes := []string{"Статьи", "Корабли", "Инженеры", "Товары", "Модули", "Материалы", "Чертежи", "Эффекты", "Экзобиология"}
		if tab == "engineering" {
			modes = []string{"План", "Материалы", "Материалы плана", "Сырьё", "Промышленные материалы", "Данные", "Сырьё плана", "Промышленные материалы плана", "Данные плана", "Обмен", "Инженеры"}
		}
		if tab == "targets" {
			modes = []string{"Личные цели", "Места добычи", "Сохранённые места", "Навигация", "Инженеры"}
		}
		if mode == "" {
			mode = "Статьи"
		}
		page := p.LocalizePage(tab, p.Page{Title: p.Title(tab), Description: "Справочник командира", Modes: modes, Columns: []string{}, Rows: []p.Row{}}, state)
		return M{"page": page, "actions": []M{}, "state": webState(state), "help": p.HelpForLanguage(tab, p.Language(state)), "context": M{"mode": mode, "selected": ""}}, nil
	}
	state := snapshotForWeb(backend, tab)
	if op == "boot" {
		return M{"app": state["app"], "tabs": p.Tabs, "navigation": state["navigation"], "activities": p.Activities, "state": webState(state)}, nil
	}
	page := p.Build(tab, mode, query, state)
	if op == "view" && mode == "" && len(page.Modes) > 0 {
		mode = page.Modes[0]
		page = p.Build(tab, mode, query, state)
	}
	selected := M{}
	selectedDetails, selectedTitle := "", page.Title
	selectedID, found := v.Str(input["selected"]), false
	selectRow := func(row p.Row) {
		selected, selectedDetails = v.Copy(row.Data), row.Details
		selectedTitle = v.Text(row.Data["title"], row.Data["name"], page.Title)
		selectedID, found = row.ID, true
	}
	for _, row := range page.Rows {
		if row.ID == selectedID {
			selectRow(row)
			break
		}
	}
	if op == "view" && !found {
		// The Academy reader can show a lesson while its underlying table is
		// the course catalogue. Keep that explicit, valid reading selection.
		if tab == "learning" && selectedID != "" {
			for _, lesson := range v.Rows(v.At(state, "learning", "lessons")) {
				if lesson["id"] == selectedID {
					selected, selectedDetails = v.Copy(lesson), p.LessonText(lesson)
					selectedTitle, found = v.Text(lesson["title"], page.Title), true
					break
				}
			}
		}
		if !found {
			selectedID = ""
			if len(page.Rows) > 0 {
				selectRow(page.Rows[0])
			}
		}
	}
	actions := p.CoreActions(tab, mode, state, selected, v.Obj(input["prefill"]))
	switch op {
	case "view":
		return M{"page": page, "actions": webActions(actions), "state": webState(state), "help": p.HelpForLanguage(tab, p.Language(state)), "context": M{"mode": mode, "selected": selectedID}}, nil
	case "action":
		for _, a := range actions {
			if a.ID != v.Str(input["actionId"]) {
				continue
			}
			if a.Disabled {
				return nil, errors.New("Сначала выберите подходящую строку")
			}
			values := M{}
			for _, f := range a.Fields {
				raw, present := v.Obj(input["values"])[f.Key]
				if !present {
					raw = f.Default
				}
				parsed, e := p.ParseField(f, raw)
				if e != nil {
					return nil, e
				}
				values[f.Key] = parsed
			}
			body, e := a.Body(values)
			if e != nil {
				return nil, e
			}
			switch a.Kind {
			case "purchase":
				prefill, e := p.PurchasePrefill(a, values)
				return M{"navigate": "sale", "mode": "Купить", "prefill": prefill}, e
			case "navigate":
				return M{"navigate": a.Path, "mode": v.Str(a.Payload["mode"])}, nil
			case "help":
				return M{"help": p.HelpForLanguage(tab, p.Language(state))}, nil
			case "details":
				return M{"details": selectedDetails, "title": selectedTitle}, nil
			case "copy":
				return M{"copy": v.Text(a.Path, a.Payload["text"], a.Payload["system"])}, nil
			case "url":
				return M{"url": a.Path}, nil
			case "export":
				return M{"export": a.Path, "projectId": a.Payload["projectId"]}, nil
			case "folder", "import-plan":
				return M{"native": a.Kind}, nil
			case "engineering-trade":
				result, e := backend.Do(a.Path, body)
				if e != nil {
					return nil, e
				}
				return v.Merge(v.Copy(result), M{"navigate": "engineering", "mode": "Обмен"}), nil
			case "theme-import":
				result, e := backend.Do(a.Path, body)
				if e != nil {
					return nil, e
				}
				return M{"themePreview": v.Copy(v.Obj(result["theme"]))}, nil
			}
			result, e := backend.Do(a.Path, body)
			delete(body, "apiKey")
			return result, e
		}
		return nil, errors.New("Действие больше недоступно. Обновите раздел.")
	}
	return nil, errors.New("Неизвестный запрос интерфейса")
}

func webLanguage(backend Controller) string {
	if provider, ok := backend.(interface{ Language() string }); ok {
		return provider.Language()
	}
	return "ru"
}

func snapshotForWeb(backend Controller, tab string) M {
	if scoped, ok := backend.(interface{ SnapshotForUI(string) M }); ok {
		return scoped.SnapshotForUI(tab)
	}
	return backend.Snapshot()
}

func webState(state M) M {
	out := M{}
	for _, key := range []string{"app", "commander", "connection", "settings", "desktop", "theme", "navigation", "resources", "goods", "tradePlan", "savedTradeRoutes", "tradeRoutes", "tradeLedger", "trading", "carrier", "carrierRoute", "carrierTrade", "colonization", "exobiology", "pilot", "mining", "stats", "learning", "carrierCalculator", "frontier", "integrations", "notices", "objectives", "commanderProfile", "engineeringInventory", "coloniesSelectors", "ranks", "politics", "politicsNetwork", "updates"} {
		out[key] = state[key]
	}
	return p.LocalizeWebState(redactWebSecrets(out))
}

// A Replacer is immutable and safe for concurrent use after construction. A
// per-key Replacer built a new lookup table for every field in every response.
var webSecretKeyNormalizer = strings.NewReplacer("_", "", "-", "")

func redactWebSecrets(input M) M {
	result := make(M, len(input))
	for key, value := range input {
		normalized := webSecretKeyNormalizer.Replace(strings.ToLower(key))
		switch normalized {
		case "apikey", "accesstoken", "refreshtoken", "clientsecret", "codeverifier", "password":
			continue
		}
		result[key] = copyRedactedWebValue(value)
	}
	return result
}

// Copy the JSON boundary while redacting it, rather than serializing and
// decoding the entire state before a second traversal. No mutable child from
// the engine snapshot may escape, including maps inside typed/nested slices.
func copyRedactedWebValue(value any) any {
	if _, customJSON := value.(json.Marshaler); customJSON {
		// In particular, RawMessage must be inspected as JSON, not copied as
		// opaque bytes that could reintroduce a credential during encoding.
		return copyRedactedWebValue(v.Clone(value))
	}
	switch item := value.(type) {
	case map[string]any:
		if item == nil {
			return nil
		}
		return redactWebSecrets(item)
	case []any:
		if item == nil {
			return nil
		}
		out := make([]any, len(item))
		for i, entry := range item {
			out[i] = copyRedactedWebValue(entry)
		}
		return out
	case []M:
		if item == nil {
			return nil
		}
		out := make([]M, len(item))
		for i, entry := range item {
			if entry != nil {
				out[i] = redactWebSecrets(entry)
			}
		}
		return out
	case []string:
		if item == nil {
			return nil
		}
		out := make([]string, len(item))
		copy(out, item)
		return out
	case []byte:
		if item == nil {
			return nil
		}
		out := make([]byte, len(item))
		copy(out, item)
		return out
	case nil:
		return nil
	}
	raw := reflect.ValueOf(value)
	switch raw.Kind() {
	case reflect.Array, reflect.Slice:
		if raw.Kind() == reflect.Slice && raw.IsNil() {
			return nil
		}
		if raw.Kind() == reflect.Slice && raw.Type().Elem().Kind() == reflect.Uint8 {
			out := reflect.MakeSlice(raw.Type(), raw.Len(), raw.Len())
			reflect.Copy(out, raw)
			return out.Interface() // Named byte slices retain JSON's base64 form.
		}
		out := make([]any, raw.Len())
		for i := range out {
			out[i] = copyRedactedWebValue(raw.Index(i).Interface())
		}
		return out
	case reflect.Map:
		if raw.IsNil() {
			return nil
		}
		if raw.Type().Key().Kind() == reflect.String {
			out := make(M, raw.Len())
			iter := raw.MapRange()
			for iter.Next() {
				out[iter.Key().String()] = iter.Value().Interface()
			}
			return redactWebSecrets(out)
		}
	case reflect.Pointer, reflect.Interface:
		if raw.IsNil() {
			return nil
		}
		return copyRedactedWebValue(raw.Elem().Interface())
	case reflect.Struct:
		// Respect JSON field names, omitempty and custom marshalers on the
		// uncommon typed payloads (for example presentation.Page).
		return copyRedactedWebValue(v.Clone(value))
	default:
		return value // JSON scalar values are immutable.
	}
	return copyRedactedWebValue(v.Clone(value))
}
func webActions(actions []p.Action) []M {
	out := []M{}
	for _, a := range actions {
		fields := []M{}
		for _, f := range a.Fields {
			defaultValue := f.Default
			if f.Kind == "password" {
				defaultValue = nil
			}
			fields = append(fields, M{"key": f.Key, "label": f.Label, "kind": f.Kind, "help": f.Help, "default": defaultValue, "choices": f.Choices, "required": f.Required, "min": f.Min, "max": f.Max})
		}
		out = append(out, M{"id": a.ID, "title": a.Title, "kind": a.Kind, "path": a.Path, "confirm": a.Confirm, "disabled": a.Disabled, "fields": fields})
	}
	return out
}
