//go:build windows

package nativeui

import (
	"fmt"
	"math"

	"github.com/Naenier/ED-Copilot/internal/localization"
	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/win"
)

type hudSectionMetrics struct {
	Height, ValueWidth int
	Lines              []int
}

func (h *hudWindow) arrange(view p.HUDRect, vehicle string, dpi int, force bool) p.HUDLayout {
	bands := p.HUDSafeBands(view, vehicle, dpi)
	width := bands[0].Width
	if width <= 0 {
		h.layoutView = p.HUDRect{}
		return p.HUDLayout{}
	}
	if !force && view == h.layoutView && vehicle == h.layoutVehicle {
		return h.plan
	}
	hdc := win.GetDC(h.hwnd)
	if hdc == 0 {
		h.layoutView = p.HUDRect{}
		return p.HUDLayout{}
	}
	defer win.ReleaseDC(h.hwnd, hdc)

	sections := h.document.Sections
	heights := make([]int, len(sections))
	metrics := make([]hudSectionMetrics, len(sections))
	var deliverySide []int
	for i, section := range sections {
		metrics[i] = h.measureSection(hdc, width, section)
		heights[i] = metrics[i].Height
		if section.Kind == "delivery" || section.Kind == "unload" {
			deliverySide = append(deliverySide, i)
		}
	}
	var shoppingSide []int
	if h.document.Activity == "colonization" && len(deliverySide) > 0 {
		for i, section := range sections {
			if section.Kind == "purchase" {
				shoppingSide = append(shoppingSide, i)
			}
		}
	}
	if len(shoppingSide) > 0 && len(deliverySide) > 1 {
		h.fitDeliveryCards(bands[0].Height, deliverySide, sections, metrics, heights)
	}
	plan := p.LayoutHUDSections(view, vehicle, dpi, heights, shoppingSide...)
	for i := range plan.Panels {
		panel := &plan.Panels[i]
		panel.Clipped = panel.Bounds.Height < metrics[panel.Index].Height
	}
	h.renderSections, h.metrics, h.layoutView, h.layoutVehicle = sections, metrics, view, vehicle
	return plan
}

// Keep a usable preview of each local action together when the right column
// can show at least one row per card. The purchase card can then use the left.
func (h *hudWindow) fitDeliveryCards(height int, cards []int, sections []p.HUDSection, metrics []hudSectionMetrics, heights []int) {
	available := height - (len(cards)-1)*h.hudPX(7)
	full := 0
	for _, i := range cards {
		full += heights[i]
	}
	if full <= available {
		return
	}
	minimum := make([]int, len(cards))
	remaining := available
	for index, i := range cards {
		if len(metrics[i].Lines) == 0 {
			return
		}
		firstRow := 2*h.hudPX(8) + h.headerHeight(sections[i]) + h.hudPX(6) + metrics[i].Lines[0]
		minimum[index] = min(heights[i], firstRow+h.hudPX(20))
		remaining -= minimum[index]
	}
	if remaining < 0 {
		return
	}
	for index, i := range cards {
		extra := min(remaining, heights[i]-minimum[index])
		heights[i] = minimum[index] + extra
		remaining -= extra
	}
}

func (h *hudWindow) hudPX(value int) int {
	return max(1, int(math.Round(float64(value)*float64(h.dpi)/96)))
}

func (h *hudWindow) headerHeight(section p.HUDSection) int {
	height := h.hudPX(17)
	if section.Subtitle != "" {
		height += h.hudPX(16)
	}
	return height
}

func (h *hudWindow) measureSection(hdc win.HDC, width int, section p.HUDSection) hudSectionMetrics {
	if hdc == 0 || width <= 0 {
		return hudSectionMetrics{}
	}
	pad, rowHeight, rowGap := h.hudPX(10), h.hudPX(17), h.hudPX(1)
	innerWidth := max(1, width-2*pad)
	m := hudSectionMetrics{Lines: make([]int, len(section.Lines))}
	old := win.SelectObject(hdc, win.HGDIOBJ(h.bold))
	defer win.SelectObject(hdc, old)
	for _, row := range section.Lines {
		if row.Value == "" {
			continue
		}
		var r win.RECT
		win.DrawTextEx(hdc, wide(hudText(row.Value, 120)), -1, &r, win.DT_SINGLELINE|win.DT_NOPREFIX|win.DT_CALCRECT, nil)
		m.ValueWidth = max(m.ValueWidth, min(int(r.Right-r.Left), innerWidth/2))
	}
	win.SelectObject(hdc, win.HGDIOBJ(h.regular))
	m.Height = 2*h.hudPX(8) + h.headerHeight(section) + h.hudPX(6)
	for i, row := range section.Lines {
		available := innerWidth
		if row.Value != "" {
			available = max(1, innerWidth-m.ValueWidth-h.hudPX(8))
		}
		r := win.RECT{Right: int32(available)}
		win.DrawTextEx(hdc, wide(hudText(row.Text, 240)), -1, &r, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_CALCRECT, nil)
		m.Lines[i] = min(max(rowHeight, int(r.Bottom)), 2*rowHeight)
		m.Height += m.Lines[i] + rowGap
	}
	if len(m.Lines) > 0 {
		m.Height -= rowGap
	}
	if section.More > 0 {
		m.Height += h.hudPX(20)
	}
	return m
}

func fillRounded(hdc win.HDC, x, y, width, height, radius int, ink win.COLORREF) {
	if width <= 0 || height <= 0 {
		return
	}
	brush := win.CreateBrushIndirect(&win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: ink})
	if brush == 0 {
		return
	}
	oldBrush := win.SelectObject(hdc, win.HGDIOBJ(brush))
	oldPen := win.SelectObject(hdc, win.GetStockObject(win.NULL_PEN))
	win.RoundRect(hdc, int32(x), int32(y), int32(x+width), int32(y+height), int32(2*radius), int32(2*radius))
	win.SelectObject(hdc, oldPen)
	win.SelectObject(hdc, oldBrush)
	win.DeleteObject(win.HGDIOBJ(brush))
}

// Windows COLORREF stores the red component in the low byte.
func mixHUDColor(base, tint win.COLORREF, percent uint32) win.COLORREF {
	var mixed uint32
	for _, shift := range []uint{0, 8, 16} {
		a, b := uint32(base)>>shift&255, uint32(tint)>>shift&255
		mixed |= ((a*(100-percent) + b*percent) / 100) << shift
	}
	return win.COLORREF(mixed)
}

func (h *hudWindow) sectionAccent(kind string) win.COLORREF {
	switch kind {
	case "delivery":
		return h.ink("good")
	case "purchase":
		return h.ink("warn")
	default:
		return h.ink("accent")
	}
}

func (h *hudWindow) cardBackground(hdc win.HDC, width, y, height int, accent win.COLORREF) win.COLORREF {
	base := mixHUDColor(h.ink("panel"), accent, 4)
	if base == h.key {
		base ^= 1
	}
	border := mixHUDColor(base, accent, 21)
	radius := h.hudPX(9)
	fillRounded(hdc, 0, y, width, height, radius, border)
	fillRounded(hdc, 1, y+1, width-2, height-2, max(1, radius-1), base)
	return base
}

func hudDrawText(hdc win.HDC, text string, x, y, right, bottom int, ink win.COLORREF, flags uint32) {
	if text == "" || x >= right || y >= bottom {
		return
	}
	rect := win.RECT{Left: int32(x), Top: int32(y), Right: int32(right), Bottom: int32(bottom)}
	win.SetTextColor(hdc, ink)
	win.DrawTextEx(hdc, wide(text), -1, &rect, flags|win.DT_NOPREFIX|win.DT_END_ELLIPSIS, nil)
}

func (h *hudWindow) drawSection(hdc win.HDC, width, y, height int, section p.HUDSection, m hudSectionMetrics, clipped bool) {
	if hdc == 0 || width <= 0 || height <= 0 {
		return
	}
	pad, insetY, rowGap := h.hudPX(10), h.hudPX(8), h.hudPX(1)
	right, bottom := width-pad, y+height
	accent := h.sectionAccent(section.Kind)
	base := h.cardBackground(hdc, width, y, height, accent)
	oldFont := win.SelectObject(hdc, win.HGDIOBJ(h.bold))
	defer win.SelectObject(hdc, oldFont)
	win.SetBkMode(hdc, win.TRANSPARENT)

	titleY := y + insetY
	titleEnd := min(bottom, titleY+h.hudPX(17))
	if titleEnd-titleY >= h.hudPX(14) {
		fillRounded(hdc, pad, titleY+h.hudPX(3), h.hudPX(4), h.hudPX(12), h.hudPX(2), accent)
	}
	hudDrawText(hdc, hudText(section.Title, 100), pad+h.hudPX(10), titleY, right, titleEnd, h.ink("text"), win.DT_SINGLELINE)
	win.SelectObject(hdc, win.HGDIOBJ(h.regular))
	hudDrawText(hdc, hudText(section.Subtitle, 160), pad+h.hudPX(10), titleEnd, right, min(bottom, titleEnd+h.hudPX(16)), h.ink("muted"), win.DT_SINGLELINE)

	lineY := titleY + h.headerHeight(section) + h.hudPX(6)
	lineBottom := bottom - insetY
	footerHeight := h.hudPX(20)
	if (clipped || section.More > 0) && lineBottom-lineY >= footerHeight {
		lineBottom -= footerHeight
	}
	if lineY < lineBottom {
		dividerY := lineY - h.hudPX(4)
		fillSolid(hdc, win.RECT{Left: int32(pad), Top: int32(dividerY), Right: int32(right), Bottom: int32(dividerY + 1)}, mixHUDColor(base, h.ink("text"), 14))
	}
	drawn := 0
	for i, row := range section.Lines {
		if i >= len(m.Lines) || lineY+m.Lines[i] > lineBottom {
			break
		}
		labelRight, labelInk := right, h.ink(row.Role)
		if row.Value != "" {
			labelRight -= m.ValueWidth + h.hudPX(8)
			labelInk = h.ink("text")
			valueInk := h.ink(row.Role)
			if row.Role == "text" && (section.Kind == "delivery" || section.Kind == "unload" || section.Kind == "purchase") {
				valueInk = accent
			}
			win.SelectObject(hdc, win.HGDIOBJ(h.bold))
			hudDrawText(hdc, hudText(row.Value, 120), right-m.ValueWidth, lineY, right, lineY+m.Lines[i], valueInk, win.DT_SINGLELINE|win.DT_RIGHT)
			win.SelectObject(hdc, win.HGDIOBJ(h.regular))
		}
		hudDrawText(hdc, hudText(row.Text, 240), pad, lineY, labelRight, lineY+m.Lines[i], labelInk, win.DT_WORDBREAK)
		lineY += m.Lines[i] + rowGap
		drawn++
	}
	remaining := section.More + len(section.Lines) - drawn
	footerTop := bottom - insetY - h.hudPX(16)
	if remaining > 0 && footerTop >= lineY {
		label := fmt.Sprintf("%s %d", localization.T(h.document.Language, "Ещё"), remaining)
		hudDrawText(hdc, label, pad, footerTop, right, bottom-insetY, h.ink("muted"), win.DT_SINGLELINE)
	}
}

func (h *hudWindow) drawOverflow(hdc win.HDC, width, y, height, count int) {
	h.cardBackground(hdc, width, y, height, h.ink("muted"))
	oldFont := win.SelectObject(hdc, win.HGDIOBJ(h.regular))
	defer win.SelectObject(hdc, oldFont)
	win.SetBkMode(hdc, win.TRANSPARENT)
	text := fmt.Sprintf("%s · %d", localization.T(h.document.Language, "Скрыто панелей"), count)
	pad := h.hudPX(10)
	hudDrawText(hdc, text, pad, y, width-pad, y+height, h.ink("muted"), win.DT_SINGLELINE|win.DT_VCENTER)
}

// Hidden-window tests and the compositor use the same measured card height.
func (h *hudWindow) layout(hdc win.HDC, width int, draw bool) int {
	y := 0
	for _, section := range h.document.Sections {
		if y > 0 {
			y += h.hudPX(7)
		}
		m := h.measureSection(hdc, width, section)
		if draw {
			h.drawSection(hdc, width, y, m.Height, section, m, false)
		}
		y += m.Height
	}
	return y
}
