package presentation

import (
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/Naenier/ED-Copilot/internal/data"
	"github.com/Naenier/ED-Copilot/internal/localization"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

var referenceNamesOnce sync.Once
var referenceNames map[string]string
var referenceGradeCaption = regexp.MustCompile(`^Класс (G[1-5]): один ролл$`)
var referenceWeaponClass = regexp.MustCompile(`^Оружие: класс (.+)$`)
var referenceNumericValue = regexp.MustCompile(`^[+−±-]?[0-9]`)

// LocalizeReference returns an independent RPC DTO. Only schema-defined UI
// captions and known catalogue entities are translated. Paragraphs, station and
// system identities, user data, links, model keys and action modes are retained.
func LocalizeReference(language string, payload M) M {
	out := v.Copy(payload)
	if localization.Normalize(language) != "en" {
		return out
	}
	for _, field := range []string{"categories", "groups"} {
		for _, row := range v.Rows(out[field]) {
			referenceCaption(row, "title")
		}
	}
	for _, field := range []string{"entries", "related"} {
		for _, row := range v.Rows(out[field]) {
			localizeReferenceEntry(row)
		}
	}
	if v.Str(out["category"]) != "" {
		localizeReferenceEntry(out)
	}
	return out
}

func referenceCaption(row M, key string) {
	if source, ok := row[key].(string); ok {
		row[key] = localization.Text("en", source)
	}
}
func referenceIdentityLabel(label string) bool {
	switch strings.TrimSpace(label) {
	case "Система", "Станция", "Тело", "База", "Фракция", "Имя", "Название", "Командир", "Позывной", "Личное имя", "Личное имя корабля", "Исходная система", "Целевая система", "Система закупки", "Система продажи", "Станция закупки", "Станция продажи", "Заметки", "Примечание", "Ваша пометка", "System", "Station", "Body", "Base", "Faction", "Name", "Commander", "Callsign", "Custom name", "Custom ship name", "Origin system", "Target system", "Notes", "Note", "Your note":
		return true
	}
	return false
}
func referenceIdentityLink(id string) bool {
	return strings.HasPrefix(id, "system/") || strings.HasPrefix(id, "station/") || strings.HasPrefix(id, "body/")
}
func referenceCanonical(id string) string {
	referenceNamesOnce.Do(func() {
		referenceNames = map[string]string{}
		put := func(id, name string) {
			if id != "" && name != "" {
				referenceNames[id] = name
			}
		}
		catalogue := data.Object("engineering-data.json")
		for _, group := range []struct{ key, prefix string }{{"materials", "material/"}, {"engineers", "engineer/"}, {"blueprints", "blueprint/"}, {"experimentals", "experimental/"}} {
			for _, row := range v.Rows(catalogue[group.key]) {
				name := v.Str(row["english"])
				if name == "" {
					continue
				}
				put(group.prefix+v.Str(row["key"]), name)
				if group.key == "blueprints" || group.key == "experimentals" {
					put(group.prefix+referenceSlug(name), name)
				}
			}
		}
		for _, row := range v.Rows(data.Array("encyclopedia-engineers.json")) {
			put("engineer/"+v.Str(row["key"]), v.Str(row["english"]))
		}
		for _, row := range v.Rows(data.Object("catalog.json")["resources"]) {
			if row["kind"] == "commodity" {
				put("commodity/"+v.Str(row["key"]), v.Str(row["english"]))
			}
		}
		for alias, raw := range data.Object("commodity-names.json") {
			values := v.Arr(raw)
			if len(values) < 2 {
				continue
			}
			name := v.Str(values[0])
			put("commodity/"+alias, name)
			put("commodity/"+v.Key(values[1]), name)
		}
		for _, row := range v.Rows(data.Array("encyclopedia-commodity-metadata.json")) {
			put("commodity/"+v.Str(row["key"]), v.Str(row["english"]))
		}
	})
	return referenceNames[id]
}
func referenceSlug(value string) string {
	var result strings.Builder
	dash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			result.WriteRune(r)
			dash = false
		} else if result.Len() > 0 && !dash {
			result.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(result.String(), "-")
}
func referenceEntityName(id, source string) string {
	if referenceIdentityLink(id) {
		return source
	}
	if known := referenceCanonical(id); known != "" {
		return known
	}
	if referenceNumericValue.MatchString(source) {
		return localization.Text("en", source)
	}
	return localization.T("en", source)
}
func referenceProperties(rows []M) {
	for _, property := range rows {
		label := v.Str(property["label"])
		referenceCaption(property, "label")
		if referenceIdentityLabel(label) || referenceIdentityLink(v.Str(property["entryId"])) {
			continue
		}
		if value, ok := property["value"].(string); ok {
			property["value"] = referenceEntityName(v.Str(property["entryId"]), value)
		}
	}
}
func localizeReferenceEntry(entry M) {
	id, category := v.Str(entry["id"]), v.Str(entry["category"])
	oldTitle := v.Str(entry["title"])
	if category != "systems" && !referenceIdentityLink(id) {
		entry["title"] = referenceEntityName(id, oldTitle)
	}
	if category == "exobiology" {
		entry["title"] = v.Text(entry["name_en"], entry["title"])
		for _, field := range []string{"description", "summary"} {
			if source := v.Str(entry[field]); source != "" {
				entry[field] = localization.T("en", source)
			}
		}
		if description := v.Str(entry["description_en"]); description != "" {
			entry["description"] = description
		}
	}
	// In these catalogue records the subtitle may be a canonical game name;
	// translating it never substitutes a system, station or personal identity.
	referenceCaption(entry, "subtitle")
	referenceCaption(entry, "groupTitle")
	if category != "systems" && !referenceIdentityLink(id) {
		for _, field := range []string{"description", "summary"} {
			if text := v.Str(entry[field]); text != "" {
				entry[field] = localization.T("en", text)
			}
		}
	}
	for _, badge := range v.Rows(entry["badges"]) {
		referenceCaption(badge, "label")
	}
	for i, badge := range v.Arr(entry["badges"]) {
		if text, ok := badge.(string); ok {
			v.Arr(entry["badges"])[i] = localization.T("en", text)
		}
	}
	referenceProperties(v.Rows(entry["properties"]))
	referenceProperties(v.Rows(entry["meta"]))
	if image, ok := entry["image"].(map[string]any); ok && v.Str(image["alt"]) == oldTitle && oldTitle != v.Str(entry["title"]) {
		image["alt"] = entry["title"]
	}
	for _, action := range v.Rows(entry["actions"]) {
		referenceCaption(action, "label")
	}
	for _, section := range v.Rows(entry["sections"]) {
		referenceCaption(section, "title")
		referenceProperties(v.Rows(section["properties"]))
		if section["type"] == "facts" {
			referenceProperties(v.Rows(section["items"]))
		}
		if category != "systems" {
			for i, paragraph := range v.Arr(section["paragraphs"]) {
				v.Arr(section["paragraphs"])[i] = localization.T("en", v.Str(paragraph))
			}
		}
		for _, item := range v.Rows(section["items"]) {
			if category == "systems" {
				continue
			}
			if title, ok := item["title"].(string); ok {
				item["title"] = referenceEntityName(v.Str(item["entryId"]), title)
			}
			for _, field := range []string{"text", "description"} {
				if text := v.Str(item[field]); text != "" {
					item[field] = localization.T("en", text)
				}
			}
		}
		columns := v.Arr(section["columns"])
		original := append([]any{}, columns...)
		for i, column := range columns {
			columns[i] = localization.T("en", v.Str(column))
		}
		links := v.Arr(section["cellLinks"])
		for i, rawRow := range v.Arr(section["rows"]) {
			row := v.Arr(rawRow)
			var linked []any
			if i < len(links) {
				linked = v.Arr(links[i])
			}
			for j, cell := range row {
				link := ""
				if j < len(linked) {
					link = v.Str(linked[j])
				}
				if referenceIdentityLink(link) || j < len(original) && referenceIdentityLabel(v.Str(original[j])) {
					continue
				}
				value := v.Str(cell)
				if referenceWeaponClass.MatchString(value) {
					row[j] = referenceWeaponClass.ReplaceAllString(value, "Weapon: class $1")
				} else {
					row[j] = referenceEntityName(link, value)
				}
			}
		}
		for _, grade := range v.Rows(section["grades"]) {
			title := v.Str(grade["title"])
			if referenceGradeCaption.MatchString(title) {
				grade["title"] = referenceGradeCaption.ReplaceAllString(title, "Grade $1: one roll")
			} else {
				referenceCaption(grade, "title")
			}
			for _, material := range v.Rows(grade["materials"]) {
				if name, ok := material["name"].(string); ok {
					material["name"] = referenceEntityName(v.Str(material["entryId"]), name)
				}
			}
			referenceProperties(v.Rows(grade["effects"]))
		}
	}
	for _, related := range v.Rows(entry["related"]) {
		localizeReferenceEntry(related)
	}
}
