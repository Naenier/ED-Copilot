//go:build windows

package nativeui

import (
	"math"

	p "github.com/Naenier/ED-Copilot/internal/presentation"
	"github.com/lxn/win"
)

func radarPen(hdc win.HDC, style uint32, ink win.COLORREF, draw func()) {
	pen := win.ExtCreatePen(style, 1, &win.LOGBRUSH{LbStyle: win.BS_SOLID, LbColor: ink}, 0, nil)
	if pen == 0 {
		return
	}
	old := win.SelectObject(hdc, win.HGDIOBJ(pen))
	draw()
	win.SelectObject(hdc, old)
	win.DeleteObject(win.HGDIOBJ(pen))
}

func (h *hudWindow) drawSurfaceRadar(hdc win.HDC, left, top, width, height int, radar *p.HUDRadar, base win.COLORREF) {
	centerX, centerY := left+width/2, top+height/2
	radius := min(width, height)/2 - h.hudPX(7)
	if radius < h.hudPX(20) || radar.Scale <= 0 {
		return
	}
	saved := win.SaveDC(hdc)
	if saved == 0 {
		return
	}
	defer win.RestoreDC(hdc, saved)
	win.IntersectClipRect(hdc, int32(centerX-radius), int32(top), int32(centerX+radius+1), int32(top+height))
	win.SelectObject(hdc, win.GetStockObject(win.NULL_BRUSH))
	ink := func(role string) win.COLORREF {
		color := h.ink(role)
		if radar.Stale {
			color = mixHUDColor(base, color, 50)
		}
		return color
	}
	circle := func(x, y, r int) {
		win.Ellipse(hdc, int32(x-r), int32(y-r), int32(x+r+1), int32(y+r+1))
	}
	pixel := func(value float64) int {
		return int(math.Round(max(-8, min(8, value/radar.Scale)) * float64(radius)))
	}
	line := func(x, y, toX, toY int) {
		win.MoveToEx(hdc, x, y, nil)
		win.LineTo(hdc, int32(toX), int32(toY))
	}
	radarPen(hdc, win.PS_SOLID, mixHUDColor(base, ink("muted"), 44), func() {
		circle(centerX, centerY, radius)
		circle(centerX, centerY, radius/2)
		line(centerX-radius, centerY, centerX+radius, centerY)
		line(centerX, centerY-radius, centerX, centerY+radius)
	})
	radarPen(hdc, win.PS_SOLID, ink("accent"), func() {
		for i := 1; i < len(radar.Trail); i++ {
			a, b := radar.Trail[i-1], radar.Trail[i]
			if !b.Break && math.Hypot(a.X, a.Y) <= radar.Scale && math.Hypot(b.X, b.Y) <= radar.Scale {
				line(centerX+pixel(a.X), centerY+pixel(a.Y), centerX+pixel(b.X), centerY+pixel(b.Y))
			}
		}
	})
	for _, point := range radar.Points {
		distance := math.Hypot(point.X, point.Y)
		if distance > radar.Scale+max(point.Exclusion, point.Uncertainty) && !point.Target {
			continue
		}
		x, y := centerX+pixel(point.X), centerY+pixel(point.Y)
		if point.Exclusion > 0 {
			radarPen(hdc, win.PS_DOT, ink("warn"), func() { circle(x, y, max(h.hudPX(3), pixel(point.Exclusion))) })
		}
		if point.Uncertainty > 0 {
			radarPen(hdc, win.PS_DOT, ink("muted"), func() { circle(x, y, max(h.hudPX(3), pixel(point.Uncertainty))) })
		}
		if point.Target && distance > radar.Scale {
			x = centerX + int(point.X/distance*float64(radius-h.hudPX(5)))
			y = centerY + int(point.Y/distance*float64(radius-h.hudPX(5)))
		} else if distance > radar.Scale {
			continue
		}
		color, style := ink("text"), uint32(win.PS_SOLID)
		switch {
		case point.Target:
			color = ink("good")
		case point.Kind == "ship" || point.Kind == "srv":
			color = ink("accent")
		case point.Kind == "sample":
			color = ink("warn")
		case point.Uncertainty > 0:
			color, style = ink("muted"), win.PS_DOT
		}
		size := h.hudPX(3)
		radarPen(hdc, style, color, func() {
			if point.Kind == "ship" || point.Kind == "srv" || point.Basis == "scanned-from" {
				line(x-size, y-size, x+size, y-size)
				line(x+size, y-size, x+size, y+size)
				line(x+size, y+size, x-size, y+size)
				line(x-size, y+size, x-size, y-size)
			} else {
				circle(x, y, size)
			}
			if point.Target {
				circle(x, y, size+h.hudPX(3))
			}
		})
	}
	radarPen(hdc, win.PS_SOLID, ink("text"), func() {
		size := h.hudPX(5)
		line(centerX-size, centerY+size, centerX, centerY-size)
		line(centerX, centerY-size, centerX+size, centerY+size)
		if radar.HeadingUp {
			theta := radar.Heading * math.Pi / 180
			dx, dy := -math.Sin(theta), -math.Cos(theta)
			line(centerX+int(dx*float64(radius-6)), centerY+int(dy*float64(radius-6)),
				centerX+int(dx*float64(radius)), centerY+int(dy*float64(radius)))
		}
	})
}
