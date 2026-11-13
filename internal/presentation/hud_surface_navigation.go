package presentation

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	v "github.com/Naenier/ED-Copilot/internal/value"
)

func hudSurfaceNavigation(doc *HUDDocument, state M) {
	if doc.Activity != "flight" {
		return
	}
	nav := v.Obj(state["surfaceNavigation"])
	target := v.Obj(nav["target"])
	targetPair := hudCoordinatePair(target)
	status := v.Str(nav["status"])
	if targetPair == "" || status == "no-target" {
		return
	}
	guidance := v.Obj(nav["guidance"])
	lines := make([]HUDLine, 0, 4)
	switch status {
	case "wrong-body":
		lines = append(lines, hudWarn("Цель на другом небесном теле"))
	case "active":
		if bearing, ok := v.Number(guidance["bearing"]); ok && bearing >= 0 && bearing < 360 {
			course := fmt.Sprintf("%03d°", int(math.Round(bearing))%360)
			lines = append(lines, hudValue("Курс", course, "good"))
		} else {
			lines = append(lines, hudMuted("Курс не определён"))
		}
	case "arrived":
		lines = append(lines, HUDLine{Text: "Точка достигнута", Role: "good"})
	default:
		lines = append(lines, hudMuted("Ждём координаты поверхности"))
	}
	lines = append(lines, hudText("Цель · "+targetPair))
	if currentPair := hudCoordinatePair(v.Obj(nav["current"])); currentPair != "" {
		lines = append(lines, hudMuted("Сейчас · "+currentPair))
	}
	if status == "active" || status == "arrived" {
		if distance, ok := v.Number(guidance["distanceM"]); ok && distance >= 0 {
			value, unit := distance, " м"
			if distance >= 1000 {
				value, unit = distance/1000, " км"
			}
			formatted := Number(value)
			if Language(state) == "en" {
				formatted = localizedNumber(formatted)
			}
			lines = append(lines, hudValue("По поверхности", formatted+unit, "text"))
		}
	}
	hudCard(doc, "surface-navigation", "ПОЛЁТ ПО КООРДИНАТАМ", v.Text(target["bodyName"], target["systemName"]), 0, lines...)
}

func hudCoordinatePair(point M) string {
	latitude, latOK := v.Number(point["latitude"])
	longitude, lonOK := v.Number(point["longitude"])
	if !latOK || !lonOK || math.Abs(latitude) > 90 || math.Abs(longitude) > 180 {
		return ""
	}
	format := func(value float64) string {
		text := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 6, 64), "0"), ".")
		if text == "-0" {
			return "0"
		}
		return text
	}
	return format(latitude) + " / " + format(longitude)
}
