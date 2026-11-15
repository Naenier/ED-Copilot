//go:build windows

package nativeui

import (
	"context"
	"errors"
	"time"
	"unsafe"

	v "github.com/Naenier/ED-Copilot/internal/value"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

type xinputGamepad struct {
	buttons uint16
	lt, rt  uint8
	lx, ly  int16
	rx, ry  int16
}

type xinputState struct {
	packet uint32
	pad    xinputGamepad
}

var xinputGetState = windows.NewLazySystemDLL("xinput1_4.dll").NewProc("XInputGetState")

// XInput reports simultaneous buttons as a mask. Prefer a face button as the
// modifier if a combination arrives within the same sample.
var gamepadInputs = [...]struct {
	mask uint32
	key  string
}{
	{0x1000, "GamePad_FaceDown"},
	{0x2000, "GamePad_FaceRight"},
	{0x4000, "GamePad_FaceLeft"},
	{0x8000, "GamePad_FaceUp"},
	{0x0100, "GamePad_LBumper"},
	{0x0200, "GamePad_RBumper"},
	{1 << 16, "Pos_GamePad_LTrigger"},
	{1 << 17, "Pos_GamePad_RTrigger"},
	{0x0040, "GamePad_LThumb"},
	{0x0080, "GamePad_RThumb"},
	{0x0010, "GamePad_Start"},
	{0x0020, "GamePad_Back"},
	{0x0001, "GamePad_DPadUp"},
	{0x0002, "GamePad_DPadDown"},
	{0x0004, "GamePad_DPadLeft"},
	{0x0008, "GamePad_DPadRight"},
}

func xinputButtons(p xinputGamepad, previous uint32) uint32 {
	buttons := uint32(p.buttons) & 0xf3ff // Ignore the reserved bits.
	for i, value := range [...]uint8{p.lt, p.rt} {
		bit := uint32(1) << (16 + i)
		// Hysteresis keeps trigger noise from ending a chord early.
		if value >= 120 || previous&bit != 0 && value >= 85 {
			buttons |= bit
		}
	}
	return buttons
}

func xinputAxis(p xinputGamepad) (string, int) {
	axes := [...]struct {
		key   string
		value int
	}{
		{"GamePad_LStickX", int(p.lx)}, {"GamePad_LStickY", int(p.ly)},
		{"GamePad_RStickX", int(p.rx)}, {"GamePad_RStickY", int(p.ry)},
		{"GamePad_LTrigger", int(p.lt) * 128}, {"GamePad_RTrigger", int(p.rt) * 128},
	}
	var key string
	largest := 0
	for _, axis := range axes {
		magnitude := max(axis.value, -axis.value)
		if magnitude > largest {
			key, largest = axis.key, magnitude
		}
	}
	return key, largest
}

type padCapture struct {
	armed       bool
	pressed     uint32
	order       []string
	axis        string
	axisStarted time.Time
}

func (c *padCapture) sample(p xinputGamepad, axisMode bool, now time.Time) (string, []string) {
	if axisMode {
		axis, magnitude := xinputAxis(p)
		if !c.armed {
			c.armed = magnitude <= 8500
			return "", nil
		}
		if magnitude < 19500 {
			c.axis = ""
			return "", nil
		}
		if c.axis != axis {
			c.axis, c.axisStarted = axis, now
		}
		if now.Sub(c.axisStarted) >= 45*time.Millisecond {
			return axis, []string{}
		}
		return "", nil
	}
	buttons := xinputButtons(p, c.pressed)
	if !c.armed {
		c.pressed = buttons
		c.armed = buttons == 0
		return "", nil
	}
	if depth := len(c.order); depth > 0 && c.pressed&^buttons != 0 {
		modifiers := append(make([]string, 0, depth-1), c.order[:depth-1]...)
		return c.order[depth-1], modifiers
	}
	changed := buttons &^ c.pressed
	for _, input := range gamepadInputs {
		if changed&input.mask != 0 {
			c.order = append(c.order, input.key)
		}
	}
	c.pressed = buttons
	return "", nil
}

func listenForGamepad(ctx context.Context, window win.HWND, axisMode bool) M {
	if xinputGetState.Find() != nil {
		return M{"unavailable": true}
	}
	var pads [4]padCapture
	started := time.Now()
	lastConnected := started
	ticker := time.NewTicker(8 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return M{"cancelled": true}
		case now := <-ticker.C:
			if win.GetForegroundWindow() != window {
				return M{"cancelled": true}
			}
			for i := range pads {
				var input xinputState
				code, _, _ := xinputGetState.Call(uintptr(i), uintptr(unsafe.Pointer(&input)))
				if code != 0 {
					pads[i] = padCapture{}
					continue
				}
				lastConnected = now
				if key, mods := pads[i].sample(input.pad, axisMode, now); key != "" && ctx.Err() == nil {
					return M{"key": key, "modifiers": mods, "controller": i}
				}
			}
			if now.Sub(lastConnected) >= 2*time.Second {
				return M{"unavailable": true}
			}
			if now.Sub(started) >= 20*time.Second {
				return M{"timedOut": true}
			}
		}
	}
}

// The WebView starts capture only from the binding dialog. XInput is sampled
// without hooks, and leaving the foreground ends capture.
func (m *modernShell) captureControl(req webRequest) {
	id := v.Str(req.Params["captureId"])
	if len(id) == 0 || len(id) > 96 {
		m.reply(req.ID, nil, errors.New("Откройте назначение заново."))
		return
	}
	if req.Op == "controls.capture.cancel" {
		if id == m.controlCaptureID {
			m.cancelControlCapture()
		}
		m.reply(req.ID, M{"ok": true}, nil)
		return
	}
	kind := v.Str(req.Params["kind"])
	if kind != "axis" && kind != "button" {
		m.reply(req.ID, nil, errors.New("Откройте назначение заново."))
		return
	}
	m.cancelControlCapture()
	ctx, stop := context.WithCancel(context.Background())
	m.controlCaptureID = id
	m.stopControlCapture = stop
	window := m.owner.main.Handle()
	m.runReadOnlyResponse(req.ID, func() (M, error) {
		defer stop()
		return listenForGamepad(ctx, window, kind == "axis"), nil
	})
}

func (m *modernShell) cancelControlCapture() {
	if m.stopControlCapture != nil {
		m.stopControlCapture()
	}
	m.stopControlCapture = nil
	m.controlCaptureID = ""
}
