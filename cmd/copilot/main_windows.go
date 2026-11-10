//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/Naenier/ED-Copilot/internal/app"
	"github.com/Naenier/ED-Copilot/internal/nativeui"
	"github.com/Naenier/ED-Copilot/internal/profile"
	"github.com/Naenier/ED-Copilot/internal/updater"
	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/walk"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func main() {
	if handled, err := updater.HandleHelper(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		return
	}
	dataDir := flag.String("data-dir", "", "Отдельная папка профиля")
	journalDir := flag.String("journal-dir", "", "Папка игровых журналов")
	version := flag.Bool("version", false, "Версия приложения")
	smoke := flag.Bool("smoke", false, "Проверить нативные экраны в новом временном профиле")
	soak := flag.Duration("smoke-soak", 0, "Измерить задержку оверлея и память на синтетическом профиле (15s–1h)")
	headless := flag.Bool("smoke-headless", false, "Проверить модель и сохранение без окна")
	demo := flag.Bool("demo", false, "Открыть демонстрацию с новым временным профилем и без сети")
	readmeScreens := flag.Bool("readme-screenshots", false, "Capture README pages in an isolated offline profile")
	readmeLanguage := flag.String("readme-language", "", "README capture language: en or ru; only with --readme-screenshots")
	stress := flag.Bool("stress", false, "Большая синтетическая история для --demo или --smoke")
	report := flag.String("smoke-report", "", "Файл отчёта smoke-сценария")
	flag.Parse()
	if *version {
		fmt.Printf("ED Copilot %s · %s · native Windows\n", app.Version, runtime.Version())
		return
	}
	captureLanguage, err := readmeCaptureLanguage(*readmeScreens, *readmeLanguage)
	if err != nil {
		fail(err, "")
		return
	}
	soakRequested := false
	flag.Visit(func(f *flag.Flag) { soakRequested = soakRequested || f.Name == "smoke-soak" })
	if soakRequested && (*soak < 15*time.Second || *soak > time.Hour || *smoke || *headless || *demo || *readmeScreens) {
		fail(errors.New("--smoke-soak принимает длительность от 15s до 1h и запускается отдельно от других сценариев"), "")
		return
	}
	isolated := *smoke || *soak != 0 || *headless || *demo || *readmeScreens
	if *stress && !isolated {
		fail(fmt.Errorf("--stress используется только с --demo или --smoke"), "")
		return
	}
	if !isolated {
		gateContext, cancelGate := context.WithTimeout(context.Background(), 2*time.Minute)
		releaseGate, gateErr := updater.AcquireStartupGate(gateContext)
		cancelGate()
		if gateErr != nil {
			fail(fmt.Errorf("ED Copilot is being updated; please try launching again shortly: %w", gateErr), "")
			return
		}
		release, acquired, err := nativeui.AcquireDesktopInstance()
		releaseGate()
		if err != nil {
			fail(err, "")
			return
		}
		if !acquired {
			nativeui.ActivateExistingDesktopInstance()
			return
		}
		defer release()
	}
	if isolated {
		root, err := os.MkdirTemp("", "ed-copilot-native-smoke-")
		if err != nil {
			fail(err, "")
			return
		}
		*dataDir = filepath.Join(root, "profile")
		*journalDir = filepath.Join(root, "journals")
		if err = smokeJournal(*journalDir); err != nil {
			fail(err, root)
			return
		}
		if *stress {
			if err = stressJournal(*journalDir); err != nil {
				fail(err, root)
				return
			}
		}
		if *report == "" {
			*report = filepath.Join(root, "smoke-report.json")
		}
	}
	if *dataDir == "" {
		*dataDir = profile.DefaultDir()
	}
	var smokeFixture *nativeui.SmokeFixture
	var soakFixture *nativeui.SoakFixture
	if *smoke || *soak != 0 {
		journalFile := filepath.Join(*journalDir, "Journal.synthetic.01.log")
		fixture, readErr := os.ReadFile(journalFile)
		if readErr != nil {
			fail(readErr, *dataDir)
			return
		}
		name := "standard"
		if *stress {
			name = "large"
		}
		smokeFixture = &nativeui.SmokeFixture{
			Name: name, JournalEvents: bytes.Count(fixture, []byte{'\n'}), JournalBytes: len(fixture), Started: time.Now(),
		}
		if *soak != 0 {
			soakFixture = &nativeui.SoakFixture{Duration: *soak, JournalFile: journalFile, Fixture: *smokeFixture}
			smokeFixture = nil
		}
	}
	options := app.Options{DataDir: *dataDir, JournalDir: *journalDir, DisableNetwork: isolated}
	engine, err := app.New(options)
	if smokeFixture != nil {
		smokeFixture.BackendElapsed = time.Since(smokeFixture.Started)
	}
	var recovery *profile.RecoveryError
	if !isolated && errors.As(err, &recovery) && !errors.Is(err, profile.ErrUnsupportedSettingsVersion) && len(recovery.Generations) != 0 {
		restored, restoreErr := nativeui.RestoreProfileOnStartup(*dataDir, recovery)
		if restoreErr != nil {
			fail(restoreErr, *dataDir)
			return
		}
		if !restored {
			return
		}
		engine, err = app.New(options)
	}
	if err != nil {
		fail(err, *dataDir)
		return
	}
	defer engine.Close()
	if isolated {
		// A fresh production profile starts in English. The existing bilingual
		// smoke suite starts explicitly in Russian and exercises both choices.
		if engine.Language() != "en" {
			fail(fmt.Errorf("fresh profile must default to English"), *dataDir)
			return
		}
		if *smoke || *headless || *readmeScreens {
			language := "ru"
			if *readmeScreens {
				language = captureLanguage
			}
			if _, err = engine.Do("/api/settings", v.M{"language": language}); err != nil {
				fail(err, *dataDir)
				return
			}
		}
		if err = smokePlans(engine); err != nil {
			fail(err, *dataDir)
			return
		}
		if err = engine.SeedDemoShowcase(); err != nil {
			fail(err, *dataDir)
			return
		}
		if err = engine.SeedDemoExobiology(); err != nil {
			fail(err, *dataDir)
			return
		}
		if err = engine.SeedDemoCommanderAndPolitics(); err != nil {
			fail(err, *dataDir)
			return
		}
		if soakFixture != nil {
			if _, err = engine.Do("/api/settings", v.M{"overlayEnabled": true, "activity": "flight"}); err != nil {
				fail(err, *dataDir)
				return
			}
		}
		if *headless {
			state := engine.Snapshot()
			result := v.M{"ok": len(v.Rows(v.At(state, "colonization", "projects"))) == 2 && v.At(state, "commander", "ship") == "Panther Clipper Mk II", "version": app.Version, "ship": v.At(state, "commander", "ship"), "colonies": len(v.Rows(v.At(state, "colonization", "projects"))), "native": true, "networkDisabled": true}
			b, _ := json.MarshalIndent(result, "", "  ")
			_ = os.MkdirAll(filepath.Dir(*report), 0700)
			if err = os.WriteFile(*report, b, 0600); err != nil {
				fail(err, *dataDir)
			}
			return
		}
	}
	var controller nativeui.Controller = engine
	if *readmeScreens {
		controller, err = newReadmeController(engine)
		if err != nil {
			fail(err, *dataDir)
			return
		}
	}
	err = nativeui.Run(controller, nativeui.Options{Smoke: *smoke || *readmeScreens, Demo: *demo, SmokeReport: *report, ReadmeScreenshots: *readmeScreens, SmokeFixture: smokeFixture, Soak: soakFixture})
	if err != nil {
		if saveErr := engine.CloseWithSave(); saveErr != nil && !isolated {
			nativeui.ResolveProfileSaveFailure(nil, engine, engine.Language(), saveErr)
		}
		fail(err, *dataDir)
		return
	}
	if saveErr := engine.CloseWithSave(); !isolated && saveErr == nil {
		if err := engine.ApplyPendingUpdate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
func fail(err error, dir string) {
	if dir != "" {
		_ = os.MkdirAll(dir, 0700)
		_ = os.WriteFile(filepath.Join(dir, "startup-error.txt"), []byte(err.Error()), 0600)
	}
	fmt.Fprintln(os.Stderr, err)
	walk.MsgBox(nil, "ED Copilot", err.Error(), walk.MsgBoxIconError)
	os.Exit(1)
}
func smokeJournal(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	at := time.Now().UTC()
	rows := []v.M{
		{"event": "Fileheader", "gameversion": "4.4.2", "build": "synthetic"},
		{"event": "LoadGame", "FID": "FNATIVE-SYNTHETIC", "Commander": "Тестовый командир", "Ship": "panthermkii", "ShipID": 1, "ShipName": "Звёздный попутчик", "ShipIdent": "CP-200", "CargoCapacity": 400, "FuelLevel": 32, "FuelCapacity": 64, "Credits": 1234567890, "Loan": 0, "Horizons": true, "Odyssey": true},
		{"event": "Location", "StarSystem": "Тестовая колония А", "SystemAddress": 10000001, "StarPos": []float64{0, 0, 0}, "Docked": true, "StationName": "Строительный склад Альфа", "MarketID": 42},
		{"event": "ColonisationConstructionDepot", "MarketID": 42, "ConstructionProgress": 0.25, "ConstructionComplete": false, "ConstructionFailed": false, "ResourcesRequired": []v.M{{"Name": "$aluminium_name;", "Name_Localised": "Алюминий", "RequiredAmount": 1000, "ProvidedAmount": 250}, {"Name": "$steel_name;", "Name_Localised": "Сталь", "RequiredAmount": 500, "ProvidedAmount": 100}}},
		{"event": "Location", "StarSystem": "Тестовая колония Б", "SystemAddress": 10000002, "StarPos": []float64{10, 0, 0}, "Docked": true, "StationName": "Строительный склад Бета", "MarketID": 43},
		{"event": "ColonisationConstructionDepot", "MarketID": 43, "ConstructionProgress": 0.1, "ResourcesRequired": []v.M{{"Name": "$steel_name;", "RequiredAmount": 800, "ProvidedAmount": 100}}},
		{"event": "Location", "StarSystem": "Тестовая колония А", "SystemAddress": 10000001, "StarPos": []float64{0, 0, 0}, "Docked": true, "StationName": "Строительный склад Альфа", "MarketID": 42},
		{"event": "Cargo", "Vessel": "Ship", "Inventory": []v.M{{"Name": "aluminium", "Name_Localised": "Алюминий", "Count": 120, "Stolen": 0}, {"Name": "steel", "Name_Localised": "Сталь", "Count": 40, "Stolen": 0}, {"Name": "drones", "Count": 10}}},
		{"event": "Materials", "Raw": []v.M{{"Name": "polonium", "Count": 25}, {"Name": "carbon", "Count": 100}}, "Manufactured": []v.M{{"Name": "chemicalprocessors", "Count": 35}}, "Encoded": []v.M{{"Name": "dataminedwake", "Count": 10}}},
		{"event": "CarrierStats", "CarrierID": 1001, "Callsign": "NAT-001", "Name": "Ковчег", "FuelLevel": 750, "JumpRangeCurr": 500, "JumpRangeMax": 500, "SpaceUsage": v.M{"TotalCapacity": 25000, "Cargo": 3000, "Crew": 1000, "ShipPacks": 0, "ModulePacks": 0, "FreeSpace": 21000}, "Finance": v.M{"CarrierBalance": 3000000000, "AvailableBalance": 2800000000}},
		{"event": "CarrierLocation", "CarrierID": 1001, "StarSystem": "Тестовая колония А", "SystemAddress": 10000001},
		{"event": "MissionAccepted", "MissionID": 9001, "LocalisedName": "Снабжение новой станции", "DestinationSystem": "Тестовая колония Б", "DestinationStation": "Строительный склад Бета", "Expiry": v.ISO(at.Add(24 * time.Hour)), "Reward": 2500000},
		{"event": "NavRoute", "Route": []v.M{{"StarSystem": "Тестовая колония А", "SystemAddress": 10000001}, {"StarSystem": "Тестовая колония Б", "SystemAddress": 10000002}}},
	}
	file, err := os.Create(filepath.Join(dir, "Journal.synthetic.01.log"))
	if err != nil {
		return err
	}
	defer file.Close()
	for i, row := range rows {
		row["timestamp"] = v.ISO(at.Add(time.Duration(i) * time.Second))
		b, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err = file.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}
func smokePlans(e *app.Engine) error {
	do := func(body v.M) error { _, err := e.Do("/api/colonization", body); return err }
	for i, name := range []string{"Тестовая колония А", "Тестовая колония Б"} {
		if err := do(v.M{"action": "create-project", "targetSystem": name, "name": name, "status": "building", "originSystem": "Тестовая исходная система", "faction": "Тестовая фракция", "cargoCapacity": 400}); err != nil {
			return err
		}
		project := v.Obj(v.At(e.Snapshot(), "colonization", "activeProject"))
		if err := do(v.M{"action": "add-building", "projectId": project["id"], "typeId": "coriolis", "name": "Основной порт", "isPrimary": true}); err != nil {
			return err
		}
		project = v.Obj(v.At(e.Snapshot(), "colonization", "activeProject"))
		building := v.Rows(project["buildings"])[0]
		if err := do(v.M{"action": "link-depot", "projectId": project["id"], "buildingId": building["id"], "marketId": fmt.Sprint(42 + i)}); err != nil {
			return err
		}
	}
	projects := v.Rows(v.At(e.Snapshot(), "colonization", "projects"))
	if err := do(v.M{"action": "select-project", "id": projects[0]["id"]}); err != nil {
		return err
	}
	_, err := e.Do("/api/objectives", v.M{"action": "add", "title": "Доставить алюминий в первую колонию", "kind": "colonization", "system": "Тестовая колония А", "priority": "high"})
	if err != nil {
		return err
	}
	_, err = e.Do("/api/settings", v.M{"overlayEnabled": false, "activity": "colonization"})
	return err
}
