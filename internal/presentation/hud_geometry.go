package presentation

import (
	"math"
	"strings"
)

// HUDPlacement keeps a section whole and orders the most useful panels on the
// right first. A section is clipped only when it cannot fit an empty safe band.
type HUDPlacement struct {
	Index, Column int
	Bounds        HUDRect
	Clipped       bool
}

type HUDLayout struct {
	Bands, Bounds [2]HUDRect // right, then left; physical desktop coordinates
	Panels        []HUDPlacement
	Hidden        int
	Overflow      HUDRect
}

func hudScale(dpi int) float64 {
	if dpi < 72 || dpi > 480 {
		dpi = 96
	}
	return float64(dpi) / 96
}

// HUDSafeBands protect the cockpit's lower instruments, weapon readouts and
// upper communications. SRV and Nomad use the same conservative cockpit band.
// On foot retains the useful upper-right origin and protects the lower HUD.
func HUDSafeBands(view HUDRect, vehicle string, dpi int) [2]HUDRect {
	var bands [2]HUDRect
	if view.Width <= 0 || view.Height <= 0 {
		return bands
	}
	scale := hudScale(dpi)
	pad := min(max(8, int(math.Round(14*scale))), min(view.Width, view.Height)/4)
	width := min(int(math.Round(330*scale)), min(max(0, view.Width-2*pad), int(float64(view.Width)*.22)))
	if width < 100 { // a tiny/minimized client cannot support readable panels
		return bands
	}
	top := int(math.Round(float64(view.Height) * .25))
	rightBottom := int(math.Floor(float64(view.Height) * .53))
	leftBottom := int(math.Floor(float64(view.Height) * .48))
	if strings.EqualFold(vehicle, "foot") {
		top = pad
		rightBottom = int(math.Floor(float64(view.Height) * .70))
		leftBottom = int(math.Floor(float64(view.Height) * .65))
	}
	top = min(max(0, top), max(0, view.Height-pad))
	bands[0] = HUDRect{X: view.X + view.Width - pad - width, Y: view.Y + top, Width: width, Height: max(0, min(view.Height-pad, rightBottom)-top)}
	bands[1] = HUDRect{X: view.X + pad, Y: view.Y + top, Width: width, Height: max(0, min(view.Height-pad, leftBottom)-top)}
	return bands
}

// HUDGeometry remains the single-column measurement contract for callers.
// Content can never extend down into the protected instrument area.
func HUDGeometry(view HUDRect, vehicle string, dpi, contentHeight int) HUDRect {
	r := HUDSafeBands(view, vehicle, dpi)[0]
	r.Height = min(max(0, contentHeight), r.Height)
	return r
}

// LayoutHUDSections packs measured GDI heights into the safe side bands. It
// never squeezes panels into the middle or moves them over fuel/landing gear.
// If both bands fill, a visible count replaces low-priority overflow panels.
func LayoutHUDSections(view HUDRect, vehicle string, dpi int, heights []int) HUDLayout {
	plan := HUDLayout{Bands: HUDSafeBands(view, vehicle, dpi)}
	if plan.Bands[0].Width == 0 || plan.Bands[0].Height == 0 {
		plan.Hidden = len(heights)
		return plan
	}
	gap := max(1, int(math.Round(6*hudScale(dpi))))
	footerHeight := min(plan.Bands[1].Height, max(18, int(math.Round(24*hudScale(dpi)))))
	pack := func(reserve bool) HUDLayout {
		next := HUDLayout{Bands: plan.Bands}
		used := [2]int{}
		column := 0
		for i, measured := range heights {
			if measured <= 0 {
				continue
			}
			for column < 2 {
				limit := next.Bands[column].Height
				if reserve && column == 1 {
					limit = max(0, limit-footerHeight-gap)
				}
				start := used[column]
				if start > 0 {
					start += gap
				}
				if measured <= limit-start || (used[column] == 0 && limit > 0) {
					height := min(measured, limit-start)
					band := next.Bands[column]
					next.Panels = append(next.Panels, HUDPlacement{Index: i, Column: column, Bounds: HUDRect{X: band.X, Y: band.Y + start, Width: band.Width, Height: height}, Clipped: measured > height})
					used[column] = start + height
					break
				}
				column++
			}
			if column >= 2 {
				next.Hidden++
			}
		}
		for col, band := range next.Bands {
			if used[col] > 0 {
				next.Bounds[col] = HUDRect{X: band.X, Y: band.Y, Width: band.Width, Height: used[col]}
			}
		}
		if reserve && next.Hidden > 0 && footerHeight > 0 {
			band := next.Bands[1]
			next.Overflow = HUDRect{X: band.X, Y: band.Y + band.Height - footerHeight, Width: band.Width, Height: footerHeight}
			next.Bounds[1] = band
		}
		return next
	}
	plan = pack(false)
	if plan.Hidden > 0 {
		plan = pack(true)
	}
	return plan
}
