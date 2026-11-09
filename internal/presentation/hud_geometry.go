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
	bottom := int(math.Floor(float64(view.Height) * .53))
	if strings.EqualFold(vehicle, "foot") {
		top = pad
		bottom = int(math.Floor(float64(view.Height) * .70))
	}
	top = min(max(0, top), max(0, view.Height-pad))
	height := max(0, min(view.Height-pad, bottom)-top)
	bands[0] = HUDRect{X: view.X + view.Width - pad - width, Y: view.Y + top, Width: width, Height: height}
	bands[1] = HUDRect{X: view.X + pad, Y: view.Y + top, Width: width, Height: height}
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
func LayoutHUDSections(view HUDRect, vehicle string, dpi int, heights []int, preferLeft ...int) HUDLayout {
	plan := HUDLayout{Bands: HUDSafeBands(view, vehicle, dpi)}
	if plan.Bands[0].Width == 0 || plan.Bands[0].Height == 0 {
		plan.Hidden = len(heights)
		return plan
	}
	gap := max(1, int(math.Round(7*hudScale(dpi))))
	footerHeight := min(plan.Bands[1].Height, max(18, int(math.Round(24*hudScale(dpi)))))
	left := make(map[int]bool, len(preferLeft))
	for _, index := range preferLeft {
		left[index] = true
	}
	pack := func(reserve bool) HUDLayout {
		next := HUDLayout{Bands: plan.Bands}
		used := [2]int{}
		for i, measured := range heights {
			if measured <= 0 {
				continue
			}
			order := [2]int{0, 1}
			if left[i] {
				order = [2]int{1, 0}
			}
			column, top, height := -1, 0, 0
			for _, side := range order {
				limit := next.Bands[side].Height
				if reserve && side == 1 {
					limit = max(0, limit-footerHeight-gap)
				}
				start := used[side]
				if start > 0 {
					start += gap
				}
				if measured <= limit-start {
					column, top, height = side, start, measured
					break
				}
				// Keep a candidate for an oversized panel only while a whole
				// band is empty. A full fit on either side wins.
				if used[side] == 0 && limit > height {
					column, top, height = side, 0, limit
				}
			}
			if column < 0 {
				next.Hidden++
				continue
			}
			band := next.Bands[column]
			next.Panels = append(next.Panels, HUDPlacement{Index: i, Column: column, Bounds: HUDRect{X: band.X, Y: band.Y + top, Width: band.Width, Height: height}, Clipped: measured > height})
			used[column] = top + height
		}
		for col, band := range next.Bands {
			if used[col] > 0 {
				next.Bounds[col] = HUDRect{X: band.X, Y: band.Y, Width: band.Width, Height: used[col]}
			}
		}
		if reserve && next.Hidden > 0 && footerHeight > 0 {
			band := next.Bands[1]
			top := used[1]
			if top > 0 {
				top += gap
			}
			next.Overflow = HUDRect{X: band.X, Y: band.Y + top, Width: band.Width, Height: footerHeight}
			next.Bounds[1] = HUDRect{X: band.X, Y: band.Y, Width: band.Width, Height: top + footerHeight}
		}
		return next
	}
	plan = pack(false)
	if plan.Hidden > 0 {
		plan = pack(true)
	}
	return plan
}
