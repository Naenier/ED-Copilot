package presentation

import "testing"

func TestHUDSideBandsProtectCockpitInstrumentsAcrossVehiclesAndMonitors(t *testing.T) {
	view := HUDRect{X: -1920, Y: 100, Width: 1920, Height: 1080}
	bands := HUDSafeBands(view, "ship", 96)
	if bands != ([2]HUDRect{{X: -344, Y: 370, Width: 330, Height: 302}, {X: -1906, Y: 370, Width: 330, Height: 302}}) {
		t.Fatal(bands)
	}
	for _, vehicle := range []string{"srv", "Nomad", "Scorpion", "unknown", ""} {
		if got := HUDSafeBands(view, vehicle, 96); got != bands {
			t.Fatalf("%s: %+v", vehicle, got)
		}
	}
	if got := HUDGeometry(view, "ship", 96, 99999); got != bands[0] {
		t.Fatal(got)
	}
	if got := HUDGeometry(view, "ship", 96, 80); got.Height != 80 {
		t.Fatal(got)
	}
	foot := HUDSafeBands(view, "foot", 96)
	if foot[0].Y != view.Y+14 || foot[0].Height <= bands[0].Height {
		t.Fatal(foot)
	}
	for _, viewport := range []HUDRect{{Width: 1280, Height: 720}, {Width: 1920, Height: 1080}, {Width: 2560, Height: 1440}, {X: 4000, Y: -2160, Width: 3840, Height: 2160}} {
		for _, dpi := range []int{0, 72, 96, 144, 192, 480, 999} {
			for side, band := range HUDSafeBands(viewport, "ship", dpi) {
				limit := viewport.Y + int(float64(viewport.Height)*.53)
				if band.X < viewport.X || band.Y < viewport.Y || band.X+band.Width > viewport.X+viewport.Width || band.Y+band.Height > limit {
					t.Fatalf("cockpit overlap: view=%+v dpi=%d side=%d band=%+v", viewport, dpi, side, band)
				}
				if band.Width > int(float64(viewport.Width)*.22) {
					t.Fatal("panel escaped side corridor", band)
				}
			}
		}
	}
	for _, tiny := range []HUDRect{{}, {Width: 120, Height: 100}, {Width: -1, Height: 100}} {
		if got := HUDGeometry(tiny, "ship", 96, 999); got.Width != 0 || got.Height != 0 {
			t.Fatal("unreadable tiny-client HUD", got)
		}
	}
}

func TestHUDLayoutKeepsPriorityOnRightAndMovesWholePanelsToLeft(t *testing.T) {
	view := HUDRect{Width: 1920, Height: 1080}
	plan := LayoutHUDSections(view, "ship", 96, []int{70, 130, 80, 90, 70})
	if plan.Hidden != 0 || len(plan.Panels) != 5 {
		t.Fatal(plan)
	}
	for i, panel := range plan.Panels {
		wantColumn := 0
		if i >= 3 {
			wantColumn = 1
		}
		if panel.Index != i || panel.Column != wantColumn || panel.Clipped {
			t.Fatal("priority/order changed", plan)
		}
		assertHUDPanelInside(t, panel.Bounds, plan.Bands[panel.Column])
		if i > 0 && plan.Panels[i-1].Column == panel.Column && panel.Bounds.Y <= plan.Panels[i-1].Bounds.Y+plan.Panels[i-1].Bounds.Height {
			t.Fatal("panels overlap", plan)
		}
	}
	if plan.Bounds[0].Height != 294 || plan.Bounds[1].Height != 167 {
		t.Fatal("unused space inflated window", plan.Bounds)
	}
	short := LayoutHUDSections(view, "ship", 96, []int{60, 70})
	if short.Bounds[1] != (HUDRect{}) || short.Hidden != 0 {
		t.Fatal("left column shown unnecessarily", short)
	}
}

func TestHUDLayoutUsesAvailableSpaceAfterMovingAShoppingCardLeft(t *testing.T) {
	view := HUDRect{Width: 1920, Height: 1080}
	plan := LayoutHUDSections(view, "ship", 96, []int{90, 240, 90}, 1)
	if plan.Hidden != 0 || len(plan.Panels) != 3 {
		t.Fatal("panels disappeared despite sufficient space", plan)
	}
	for i, want := range []int{0, 1, 0} {
		if plan.Panels[i].Column != want || plan.Panels[i].Clipped {
			t.Fatal("shopping should stay apart from the two immediate actions", plan)
		}
	}
}

func TestHUDOverflowIsExplicitAndNeverOccupiesTheLowerGameHUD(t *testing.T) {
	for _, viewport := range []HUDRect{{Width: 1280, Height: 720}, {Width: 1920, Height: 1080}, {X: -3840, Y: -2160, Width: 3840, Height: 2160}} {
		for _, dpi := range []int{96, 144, 192} {
			plan := LayoutHUDSections(viewport, "ship", dpi, []int{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100})
			if plan.Hidden <= 0 || plan.Overflow.Height <= 0 {
				t.Fatal("overflow is silent", plan)
			}
			assertHUDPanelInside(t, plan.Overflow, plan.Bands[1])
			seen := map[int]bool{}
			for _, panel := range plan.Panels {
				if seen[panel.Index] {
					t.Fatal("duplicate section", plan)
				}
				seen[panel.Index] = true
				assertHUDPanelInside(t, panel.Bounds, plan.Bands[panel.Column])
				if panel.Column == 1 && panel.Bounds.Y+panel.Bounds.Height > plan.Overflow.Y {
					t.Fatal("footer hides panel text", plan)
				}
			}
			if len(plan.Panels)+plan.Hidden != 12 {
				t.Fatal("section lost", plan)
			}
		}
	}
	large := LayoutHUDSections(HUDRect{Width: 1920, Height: 1080}, "ship", 96, []int{9999, 60})
	if len(large.Panels) != 2 || !large.Panels[0].Clipped || large.Panels[1].Column != 1 || large.Panels[1].Clipped {
		t.Fatal(large)
	}
}

func assertHUDPanelInside(t *testing.T, rect, band HUDRect) {
	t.Helper()
	if rect.Width <= 0 || rect.Height <= 0 || rect.X < band.X || rect.Y < band.Y || rect.X+rect.Width > band.X+band.Width || rect.Y+rect.Height > band.Y+band.Height {
		t.Fatalf("unsafe panel: %+v band=%+v", rect, band)
	}
}
