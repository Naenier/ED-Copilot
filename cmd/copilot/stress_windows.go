//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

// stressJournal extends only a freshly generated, disposable journal. It never
// reads the player's journal directory or enables network integrations.
func stressJournal(dir string) error {
	f, err := os.OpenFile(filepath.Join(dir, "Journal.synthetic.01.log"), os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	at := time.Now().UTC().Add(30 * time.Second)
	sequence := 0
	write := func(row v.M) error {
		row["timestamp"] = v.ISO(at.Add(time.Duration(sequence) * time.Millisecond))
		sequence++
		b, err := json.Marshal(row)
		if err != nil {
			return err
		}
		_, err = f.Write(append(b, '\n'))
		return err
	}
	for system := 0; system < 300; system++ {
		name := fmt.Sprintf("Synthetic performance %03d", system)
		address := int64(900000000 + system)
		if err := write(v.M{"event": "FSDJump", "StarSystem": name, "SystemAddress": address, "StarPos": []float64{float64(system * 2), 3, 0}, "JumpDist": 2}); err != nil {
			return err
		}
		for body := 1; body <= 10; body++ {
			if err := write(v.M{"event": "Scan", "BodyID": body, "BodyName": fmt.Sprintf("%s %d", name, body), "SystemAddress": address, "PlanetClass": "Rocky body", "Landable": true, "SurfaceGravity": 9.80665, "ReserveLevel": "PristineResources", "Materials": []v.M{{"Name": "iron", "Percent": 20}, {"Name": "polonium", "Percent": 1}}, "Rings": []v.M{{"Name": fmt.Sprintf("%s %d A Ring", name, body), "RingClass": "eRingClass_Metallic"}}}); err != nil {
				return err
			}
		}
	}
	return write(v.M{"event": "Location", "StarSystem": "Тестовая колония А", "SystemAddress": 10000001, "StarPos": []float64{0, 0, 0}, "Docked": true, "StationName": "Строительный склад Альфа", "MarketID": 42})
}
