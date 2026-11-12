package shipbuild

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"testing"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func TestSLEFPreservesUnknownMetadataAndOffersSafeBuilderImport(t *testing.T) {
	source := `[{"header":{"appName":"EDSY","appVersion":"3.0","appCustomProperties":{"kept":true}},"data":{"Ship":"asp","ShipID":18446744073709551614,"UnladenMass":352.5,"Modules":[{"Slot":"FutureSlot7","Item":"Int_Future_Module","On":false,"Priority":3,"Engineering":{"BlueprintName":"FutureBlueprint","Level":4,"Quality":0.8,"ExperimentalEffect":"FutureEffect","Modifiers":[{"Label":"PowerDraw","Value":1.2}]},"FutureMetadata":{"kept":7}}]}}]`
	workspace, view, err := Update(nil, nil, M{"action": "import", "text": source})
	if err != nil {
		t.Fatal(err)
	}
	slot := v.Rows(v.At(view, "target", "slots"))[0]
	if slot["slot"] != "FutureSlot7" || slot["on"] != false || v.Int(slot["priority"]) != 3 || v.At(slot, "engineering", "ExperimentalEffect") != "FutureEffect" {
		t.Fatal(slot)
	}
	result, err := Export(workspace, M{"provider": "edsy"})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(v.Str(result["text"]))
	if err != nil || len(parsed) != 1 || v.Str(v.At(parsed[0], "payload", "ShipID")) != "18446744073709551614" ||
		v.At(parsed[0], "header", "appCustomProperties", "kept") != true ||
		v.Int(v.At(v.Rows(v.At(parsed[0], "payload", "Modules"))[0], "FutureMetadata", "kept")) != 7 {
		t.Fatal(parsed, err)
	}
	encoded, ok := strings.CutPrefix(v.Str(result["url"]), "https://edsy.org/#/I=")
	if !ok {
		t.Fatal(result["url"])
	}
	encoded, _ = url.PathUnescape(encoded)
	payload, _ := base64.URLEncoding.DecodeString(encoded)
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	body, _ := io.ReadAll(reader)
	var importData M
	if err := json.Unmarshal(body, &importData); err != nil || importData["Ship"] != "asp" || importData["ShipID"] != nil || len(v.Rows(importData["Modules"])) != 1 {
		t.Fatal(string(body), err)
	}
}

func TestCoriolisDetailedKeepsItsOwnLayoutAndKnownLadenStats(t *testing.T) {
	source := `{"$schema":"https://coriolis.io/schemas/ship-loadout/4.json#","name":"Custom Asp","ship":"Asp Explorer","components":{"standard":{"frameShiftDrive":{"rating":"A","class":5,"priority":4,"modifications":{"mass":3251}}},"internal":[null,{"group":"New Module","class":3,"rating":"A"}]},"stats":{"unladenRange":43,"ladenRange":29,"powerDeployed":12},"references":[{"url":"https://coriolis.edcd.io/outfit/asp?code=example"}]}`
	observed := M{"Ship": "asp", "MaxJumpRange": 42, "Modules": []M{{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5"}}}
	saved, view, err := Update(nil, observed, M{"action": "import", "text": source})
	if err != nil || v.At(view, "comparison", "compatibleLayout") != false || v.At(view, "comparison", "stats", "ladenRange", "before") != nil ||
		v.Num(v.At(view, "comparison", "stats", "ladenRange", "after")) != 29 {
		t.Fatal(view, err)
	}
	export, err := Export(saved, M{"provider": "coriolis"})
	if err != nil || export["url"] != "https://coriolis.io/outfit/asp?code=example" ||
		!strings.Contains(v.Str(export["text"]), `"mass": 3251`) {
		t.Fatal(export, err)
	}
	if _, err := Parse(source + "{}"); err == nil {
		t.Fatal("additional JSON accepted")
	}
}

func TestBuildComparisonUsesStoredAndMovedModulesAndInvalidatesEditedMetrics(t *testing.T) {
	observed := M{"Ship": "asp", "UnladenMass": 350, "Modules": []M{
		{"Slot": "Slot01_Size5", "Item": "int_hyperdrive_size5_class5", "Engineering": M{"Level": 3}},
		{"Slot": "Slot02_Size5", "Item": "int_cargorack_size5_class1"},
	}}
	workspace, _, err := Update(nil, observed, M{"action": "baseline"})
	if err != nil {
		t.Fatal(err)
	}
	source := `{"Ship":"asp","UnladenMass":358,"Modules":[{"Slot":"Slot01_Size5","Item":"int_new_optional_module"},{"Slot":"Slot02_Size5","Item":"int_hyperdrive_size5_class5","Engineering":{"Level":3}}]}`
	workspace, view, err := Update(workspace, observed, M{"action": "import", "text": source, "storedModules": []M{{"Name": "int_new_optional_module"}}})
	if err != nil || len(v.Rows(v.At(view, "comparison", "shopping"))) != 0 || len(v.Rows(view["upgradeGoals"])) != 0 ||
		v.Num(v.At(view, "comparison", "stats", "unladenMass", "delta")) != 8 {
		t.Fatal(view, err)
	}
	id := v.At(view, "target", "id")
	workspace, view, err = Update(workspace, observed, M{"action": "replace", "id": id, "slot": "Slot01_Size5", "item": "int_another_module", "priority": 3})
	if err != nil || v.At(view, "target", "stats", "unladenMass") != nil || len(v.Rows(v.At(view, "comparison", "shopping"))) != 1 {
		t.Fatal(view, err)
	}
	if v.At(selectedBuild(workspace, baselineID), "payload", "UnladenMass") == nil || observed["UnladenMass"] != 350 {
		t.Fatal("editing the target changed the baseline")
	}
}

func TestObservedCraftChangesOnlyTheMatchingInstalledModule(t *testing.T) {
	loadout := M{"Ship": "asp", "MaxJumpRange": 40, "Modules": []M{
		{"Slot": "FrameShiftDrive", "Item": "int_hyperdrive_size5_class5", "Priority": 3},
		{"Slot": "CargoHatch", "Item": "modularcargobaydoor", "On": false},
	}}
	craft := M{"Slot": "FrameShiftDrive", "Module": "int_hyperdrive_size5_class5", "BlueprintName": "FSD_LongRange", "Level": 2, "Quality": 1}
	after := ApplyObservedCraft(loadout, craft)
	modules := v.Rows(after["Modules"])
	if after["MaxJumpRange"] != nil || v.Int(v.At(modules[0], "Engineering", "Level")) != 2 || v.Int(modules[0]["Priority"]) != 3 ||
		modules[1]["On"] != false || v.Rows(loadout["Modules"])[0]["Engineering"] != nil {
		t.Fatal(after)
	}
	craft["Module"] = "other"
	if result := ApplyObservedCraft(loadout, craft); result["MaxJumpRange"] != 40 {
		t.Fatal(result)
	}
}
