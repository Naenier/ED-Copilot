//go:build windows

package nativeui

import (
	"math"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/win"
)

type hudSectionMetrics struct {
	Height, TitleHeight int
	Lines               []int
}

func (h *hudWindow) arrange(view p.HUDRect, vehicle string, dpi int, force bool) p.HUDLayout {
	width := p.HUDGeometry(view, vehicle, dpi, 0).Width
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
	heights := make([]int, len(h.document.Sections))
	metrics := make([]hudSectionMetrics, len(h.document.Sections))
	for i, section := range h.document.Sections {
		metrics[i] = h.measureSection(hdc, width, section)
		heights[i] = metrics[i].Height
	}
	win.ReleaseDC(h.hwnd, hdc)
	plan := p.LayoutHUDSections(view, vehicle, dpi, heights)
	if plan.Bounds[0].Width == 0 || plan.Bounds[0].Height == 0 {
		h.layoutView = p.HUDRect{}
		return plan
	}
	h.metrics, h.layoutView, h.layoutVehicle = metrics, view, vehicle
	return plan
}

func (h *hudWindow) hudPX(value int) int {
	return max(1, int(math.Round(float64(value)*float64(h.dpi)/96)))
}

func (h *hudWindow) measureSection(hdc win.HDC, width int, section p.HUDSection) hudSectionMetrics {
	if hdc == 0 || width <= 0 {
		return hudSectionMetrics{}
	}
	pad := h.hudPX(9)
	oldFont := win.SelectObject(hdc, win.HGDIOBJ(h.regular))
	defer win.SelectObject(hdc, oldFont)
	textHeight := func(text string, bold bool) int {
		font := h.regular
		if bold {
			font = h.bold
		}
		win.SelectObject(hdc, win.HGDIOBJ(font))
		r := win.RECT{Left: int32(pad), Right: int32(max(pad+10, width-pad))}
		win.DrawTextEx(hdc, wide(text), -1, &r, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_CALCRECT, nil)
		return min(max(h.hudPX(17), int(r.Bottom)), h.hudPX(52))
	}
	m := hudSectionMetrics{TitleHeight: textHeight(hudText(section.Title, 100), true), Lines: make([]int, len(section.Lines))}
	m.Height = 2*pad + m.TitleHeight
	if len(section.Lines) > 0 {
		m.Height += h.hudPX(4)
	}
	for i, line := range section.Lines {
		m.Lines[i] = textHeight(hudText(line.Text, 280), false)
		m.Height += m.Lines[i]
		if i > 0 {
			m.Height += h.hudPX(2)
		}
	}
	return m
}

// drawSection never draws a partial line into the cockpit's protected area.
// In the rare case of an oversized font/panel it points to the full app instead.
func (h *hudWindow) drawSection(hdc win.HDC, width, y, height int, section p.HUDSection, m hudSectionMetrics, clipped bool) {
	if hdc == 0 || width <= 0 || height <= 0 {
		return
	}
	pad := h.hudPX(9)
	bottom := y + height
	fillSolid(hdc, win.RECT{Left: 0, Top: int32(y), Right: int32(width), Bottom: int32(bottom)}, h.ink("panel"))
	fillSolid(hdc, win.RECT{Left: 0, Top: int32(y), Right: int32(h.hudPX(2)), Bottom: int32(bottom)}, h.ink("accent"))
	oldFont := win.SelectObject(hdc, win.HGDIOBJ(h.bold))
	defer win.SelectObject(hdc, oldFont)
	win.SetBkMode(hdc, win.TRANSPARENT)
	win.SetTextColor(hdc, h.ink("accent"))
	r := win.RECT{Left: int32(pad), Top: int32(y + pad), Right: int32(max(pad+1, width-pad)), Bottom: int32(min(bottom, y+pad+m.TitleHeight))}
	if r.Bottom > r.Top {
		win.DrawTextEx(hdc, wide(hudText(section.Title, 100)), -1, &r, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_END_ELLIPSIS, nil)
	}
	lineY := y + pad + m.TitleHeight + h.hudPX(4)
	lineBottom := bottom - pad
	if clipped {
		lineBottom -= h.hudPX(19)
	}
	win.SelectObject(hdc, win.HGDIOBJ(h.regular))
	for i, line := range section.Lines {
		if i >= len(m.Lines) || lineY+m.Lines[i] > lineBottom {
			break
		}
		win.SetTextColor(hdc, h.ink(line.Role))
		r = win.RECT{Left: int32(pad), Top: int32(lineY), Right: int32(max(pad+1, width-pad)), Bottom: int32(lineY + m.Lines[i])}
		win.DrawTextEx(hdc, wide(hudText(line.Text, 280)), -1, &r, win.DT_WORDBREAK|win.DT_NOPREFIX|win.DT_END_ELLIPSIS, nil)
		lineY += m.Lines[i] + h.hudPX(2)
	}
	if clipped && height >= pad*2+m.TitleHeight+h.hudPX(19) {
		win.SetTextColor(hdc, h.ink("muted"))
		r = win.RECT{Left: int32(pad), Top: int32(bottom - pad - h.hudPX(17)), Right: int32(max(pad+1, width-pad)), Bottom: int32(bottom - pad)}
		win.DrawTextEx(hdc, wide("Подробности в ED Copilot"), -1, &r, win.DT_SINGLELINE|win.DT_END_ELLIPSIS|win.DT_NOPREFIX, nil)
	}
}

// layout is also used by hidden-window GDI tests. Production layout measures
// the same sections and places them with LayoutHUDSections into two safe bands.
func (h *hudWindow) layout(hdc win.HDC, width int, draw bool) int {
	y := 0
	for _, section := range h.document.Sections {
		if y > 0 {
			y += h.hudPX(6)
		}
		m := h.measureSection(hdc, width, section)
		if draw {
			h.drawSection(hdc, width, y, m.Height, section, m, false)
		}
		y += m.Height
	}
	return y
}
