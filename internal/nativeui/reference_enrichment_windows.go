//go:build windows

package nativeui

import (
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// mergeReferenceSystem combines a static destination guide with observations of
// this exact system. It never enumerates or copies the exploration catalogue.
func mergeReferenceSystem(backend Controller, entry M) M {
	out := v.Copy(entry)
	if v.Str(entry["category"]) != "systems" {
		return out
	}
	provider, ok := backend.(interface{ ReferenceSystem(string) M })
	if !ok {
		return out
	}
	name := strings.TrimSpace(v.Str(entry["title"]))
	if name == "" {
		return out
	}
	observed := provider.ReferenceSystem(name)
	if !strings.EqualFold(strings.TrimSpace(v.Str(observed["title"])), name) {
		return out
	}
	// A provider may return a useful unknown-system card. Do not replace a
	// known workshop guide with its generic "no local observations" wording.
	hasObservation := false
	for _, p := range v.Rows(observed["properties"]) {
		if p["label"] == "Адрес системы" || p["label"] == "Известно тел" {
			hasObservation = true
		}
	}
	for _, section := range v.Rows(observed["sections"]) {
		if section["id"] == "bodies" {
			hasObservation = true
		}
	}
	if hasObservation {
		properties := v.Rows(out["properties"])
		seenProperties := map[string]bool{}
		for _, p := range properties {
			seenProperties[v.Str(p["label"])] = true
		}
		for _, p := range v.Rows(observed["properties"]) {
			if label := v.Str(p["label"]); label != "" && !seenProperties[label] {
				properties = append(properties, v.Copy(p))
				seenProperties[label] = true
			}
		}
		out["properties"] = properties
		sections := v.Rows(out["sections"])
		seenSections := map[string]bool{}
		for _, section := range sections {
			seenSections[v.Str(section["id"])] = true
		}
		if description := v.Str(observed["description"]); description != "" && !seenSections["observations"] {
			sections = append(sections, M{"id": "observations", "title": "Наблюдения журнала", "type": "prose", "paragraphs": []string{description}})
		}
		for _, section := range v.Rows(observed["sections"]) {
			if id := v.Str(section["id"]); id != "" && !seenSections[id] {
				sections = append(sections, v.Copy(section))
				seenSections[id] = true
			}
		}
		out["sections"] = sections
	}
	actions := v.Rows(out["actions"])
	seenActions := map[string]bool{}
	for _, action := range actions {
		seenActions[v.Str(action["type"])+"|"+v.Str(action["system"])] = true
	}
	for _, action := range v.Rows(observed["actions"]) {
		key := v.Str(action["type"]) + "|" + v.Str(action["system"])
		if !seenActions[key] {
			actions = append(actions, v.Copy(action))
			seenActions[key] = true
		}
	}
	out["actions"] = actions
	return out
}
