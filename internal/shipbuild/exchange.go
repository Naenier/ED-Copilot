// Package shipbuild preserves ship fitting documents and compares the data
// they actually contain. SLEF wraps Frontier Loadout objects in an array of
// header/data pairs. Coriolis detailed exports retain their own slot layout.
package shipbuild

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Naenier/ED-Copilot/internal/ships"
	v "github.com/Naenier/ED-Copilot/internal/value"
)

type M = v.M

const (
	maxInput   = 2 << 20
	maxBuilds  = 16
	maxModules = 128
)

var symbol = regexp.MustCompile(`^[A-Za-z0-9_.$;-]{1,120}$`)

func label(value any, limit int) string {
	text := strings.TrimSpace(v.Str(value))
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > limit || strings.ContainsFunc(text, unicode.IsControl) {
		return ""
	}
	return text
}

// Parse accepts one journal Loadout, a SLEF bundle, or a Coriolis detailed
// export. The payload is retained instead of reconstructing unknown modules.
func Parse(text string) ([]M, error) {
	if len(text) == 0 || len(text) > maxInput {
		return nil, errors.New("Вставьте JSON сборки размером до 2 МБ.")
	}
	decoder := json.NewDecoder(strings.NewReader(strings.TrimPrefix(strings.TrimSpace(text), "\uFEFF")))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, errors.New("Некорректный JSON сборки.")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, errors.New("Некорректный JSON сборки.")
	}
	rows := v.Arr(raw)
	if _, isObject := raw.(map[string]any); isObject {
		rows = []any{raw}
	}
	if len(rows) == 0 || len(rows) > maxBuilds {
		return nil, errors.New("Импортируйте от одной до 16 сборок.")
	}
	builds := make([]M, 0, len(rows))
	for _, row := range rows {
		obj := v.Obj(row)
		var build M
		switch {
		case v.At(obj, "data", "Ship") != nil:
			build = journalBuild(v.Obj(obj["data"]), v.Obj(obj["header"]), "slef")
		case obj["Ship"] != nil:
			build = journalBuild(obj, nil, "journal")
		case obj["ship"] != nil && obj["components"] != nil:
			build = coriolisBuild(obj)
		}
		if build == nil {
			return nil, errors.New("Нужен экспорт SLEF, Coriolis Detailed или событие Loadout.")
		}
		builds = append(builds, build)
	}
	return builds, nil
}

func journalBuild(payload, header M, format string) M {
	ship := label(payload["Ship"], 120)
	modules := v.Arr(payload["Modules"])
	if ship == "" || modules == nil || len(modules) > maxModules {
		return nil
	}
	seen := map[string]bool{}
	for _, raw := range modules {
		row := v.Obj(raw)
		slot := strings.ToLower(label(row["Slot"], 120))
		if slot == "" || seen[slot] || !symbol.MatchString(label(row["Item"], 120)) {
			return nil
		}
		seen[slot] = true
	}
	return M{
		"name": v.Text(label(payload["ShipName"], 160), ships.DisplayShip(ship)),
		"ship": ship, "format": format, "payload": v.Copy(payload), "header": v.Copy(header),
	}
}

func coriolisBuild(payload M) M {
	ship := label(payload["ship"], 120)
	components := v.Obj(payload["components"])
	if ship == "" || len(components) == 0 || len(coriolisSlots(components)) > maxModules {
		return nil
	}
	return M{
		"name": v.Text(label(payload["name"], 160), ships.DisplayShip(ship)),
		"ship": ship, "format": "coriolis", "payload": v.Copy(payload),
	}
}

func externalLink(raw any, provider string) string {
	address, err := url.Parse(v.Str(raw))
	if err != nil || address.Scheme != "https" || address.User != nil || address.Port() != "" {
		return ""
	}
	host := strings.ToLower(address.Hostname())
	if provider == "coriolis" && host == "coriolis.edcd.io" {
		address.Host, host = "coriolis.io", "coriolis.io"
	}
	if provider == "edsy" && host == "edsy.org" || provider == "coriolis" && (host == "coriolis.io" || host == "beta.coriolis.io") {
		return address.String()
	}
	return ""
}

func Export(saved, request M) (M, error) {
	build := selectedBuild(saved, v.Str(request["id"]))
	if build == nil {
		return nil, errors.New("Сборка не найдена.")
	}
	payload := build["payload"]
	format := v.Str(build["format"])
	var exported any = payload
	if format != "coriolis" && (format == "slef" || request["format"] == "slef") {
		header := v.Copy(v.Obj(build["header"]))
		if header == nil {
			header = M{}
		}
		if v.Str(header["appName"]) == "" {
			header["appName"] = "ED Copilot"
		}
		if v.Str(header["appVersion"]) == "" {
			header["appVersion"] = "1.0"
		}
		exported, format = []M{{"header": header, "data": payload}}, "slef"
	}
	bytes, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		return nil, err
	}
	result := M{"name": build["name"], "text": string(bytes), "format": format}
	if provider := v.Str(request["provider"]); provider != "" {
		target := builderURL(build, provider)
		if target == "" {
			return nil, errors.New("Для этой сборки нет ссылки конструктора. Импортируйте экспортированный JSON вручную.")
		}
		result["url"] = target
	}
	return result, nil
}

// The import endpoints use URL-safe base64 of gzip-compressed Loadout JSON.
// Send fitting fields only; journal identity and unrelated import metadata
// are retained in the local export but are not needed by the calculators.
func builderURL(build M, provider string) string {
	base := map[string]string{"edsy": "https://edsy.org/#/I=", "coriolis": "https://coriolis.io/import?data="}[provider]
	if base == "" {
		return ""
	}
	if build["format"] == "coriolis" {
		if provider == "coriolis" {
			return v.Str(buildView(build)["providerURL"])
		}
		return ""
	}
	payload := v.Obj(build["payload"])
	document := M{"event": "Loadout", "Ship": payload["Ship"]}
	for _, name := range []string{"ShipName", "ShipIdent"} {
		if title := label(payload[name], 160); title != "" {
			document[name] = title
		}
	}
	modules := []M{}
	for _, item := range v.Rows(payload["Modules"]) {
		row := M{"Slot": item["Slot"], "Item": item["Item"]}
		for _, field := range []string{"On", "Priority"} {
			if item[field] != nil {
				row[field] = item[field]
			}
		}
		if engineering := v.Obj(item["Engineering"]); len(engineering) > 0 {
			fields := M{}
			for _, field := range []string{"BlueprintName", "Level", "Quality", "ExperimentalEffect"} {
				if engineering[field] != nil {
					fields[field] = engineering[field]
				}
			}
			modifiers := []M{}
			for _, raw := range v.Rows(engineering["Modifiers"]) {
				mod := M{}
				for _, field := range []string{"Label", "Value", "OriginalValue", "LessIsGood", "ValueStr"} {
					if raw[field] != nil {
						mod[field] = raw[field]
					}
				}
				if len(mod) > 0 {
					modifiers = append(modifiers, mod)
				}
			}
			if len(modifiers) > 0 {
				fields["Modifiers"] = modifiers
			}
			row["Engineering"] = fields
		}
		modules = append(modules, row)
	}
	document["Modules"] = modules
	payloadBytes, err := json.Marshal(document)
	if err != nil {
		return ""
	}
	var buffer bytes.Buffer
	zipped := gzip.NewWriter(&buffer)
	if _, err := zipped.Write(payloadBytes); err != nil {
		return ""
	}
	if err := zipped.Close(); err != nil {
		return ""
	}
	encoded := base64.URLEncoding.EncodeToString(buffer.Bytes())
	if len(encoded) > 20000 {
		return ""
	}
	return base + strings.ReplaceAll(encoded, "=", "%3D")
}
