//go:build windows

package nativeui

import (
	"reflect"
	"runtime"
	"testing"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/walk"
)

func TestNativeHUDKeepsColonyActionsAndAmountsInSafeBands(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer nativeTestControls(t)()
	main, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer main.Dispose()
	state := M{"connection": M{"gameRunning": false}}
	h, err := newHUD(&Window{main: main, state: state, backend: &platformController{state: state}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.close()

	purchases := []p.HUDLine{}
	for _, name := range []string{"СММ-композит", "Жидкий кислород", "Керамокомпозиты", "Конструкционные регуляторы", "Полимеры", "Стабилизаторы поверхности", "Строительные синтезаторы"} {
		purchases = append(purchases, p.HUDLine{Text: name, Value: "6 776 т", Role: "text"})
	}
	h.document = p.HUDDocument{
		Activity: "colonization", Language: "ru",
		Sections: []p.HUDSection{
			{Kind: "delivery", Title: "Доставить", Subtitle: "Synthetic Colony", Lines: []p.HUDLine{{Text: "Алюминий", Value: "1 232 т", Role: "good"}}},
			{Kind: "unload", Title: "Разгрузить с флотоносца", Subtitle: "SYN-001", Lines: []p.HUDLine{{Text: "Сталь", Value: "990 т", Role: "text"}, {Text: "Титан", Value: "510 т", Role: "text"}}},
			{Kind: "purchase", Title: "Купить", More: 4, Lines: purchases},
		},
	}
	for _, dpi := range []int{96, 144} {
		h.setFonts(dpi)
		view := p.HUDRect{X: -1920, Y: 100, Width: 1920, Height: 1080}
		for _, vehicle := range []string{"ship", "srv", "foot"} {
			plan := h.arrange(view, vehicle, dpi, true)
			if len(plan.Panels) != 3 || plan.Hidden != 0 {
				t.Fatal(vehicle, dpi, "working cards disappeared", plan)
			}
			for i, panel := range plan.Panels {
				wantColumn := 0
				if i == 2 {
					wantColumn = 1
				}
				band := plan.Bands[panel.Column]
				if panel.Column != wantColumn || panel.Clipped || panel.Bounds.X != band.X || panel.Bounds.Y < band.Y || panel.Bounds.Y+panel.Bounds.Height > band.Y+band.Height {
					t.Fatal(vehicle, dpi, "card escaped its safe band", panel, band)
				}
				if h.metrics[i].ValueWidth <= 0 || len(h.metrics[i].Lines) != len(h.document.Sections[i].Lines) {
					t.Fatal("quantity column was not measured", h.metrics[i])
				}
			}
			if !reflect.DeepEqual(h.renderSections, h.document.Sections) {
				t.Fatal("native layout changed action grouping")
			}
		}
	}
}
