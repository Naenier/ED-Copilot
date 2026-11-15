//go:build windows

package nativeui

import (
	"reflect"
	"testing"
	"time"
	"unsafe"
)

func TestGamepadCaptureWaitsForNeutralAndKeepsChordOrder(t *testing.T) {
	var capture padCapture
	const x, right = 0x4000, 0x0008
	press := func(mask uint16) (string, []string) {
		return capture.sample(xinputGamepad{buttons: mask}, false, time.Time{})
	}
	press(x)
	if key, _ := press(0); key != "" {
		t.Fatal("captured button that was already held when listening began")
	}
	for _, mask := range []uint16{x, x | right} {
		if key, _ := press(mask); key != "" {
			t.Fatal("captured an incomplete chord before release")
		}
	}
	if key, modifiers := press(x); key != "GamePad_DPadRight" || !reflect.DeepEqual(modifiers, []string{"GamePad_FaceLeft"}) {
		t.Fatalf("expected X + D-pad right, got %q %v", key, modifiers)
	}
	capture = padCapture{}
	for _, mask := range []uint16{0, 0x0100, 0x0100 | x} {
		press(mask)
	}
	if key, modifiers := press(0); key != "GamePad_FaceLeft" || !reflect.DeepEqual(modifiers, []string{"GamePad_LBumper"}) {
		t.Fatalf("expected LB + X, got %q %v", key, modifiers)
	}
}

func TestGamepadCaptureHandlesAnalogueInputAndNoise(t *testing.T) {
	if unsafe.Sizeof(xinputState{}) != 16 {
		t.Fatal("unexpected XINPUT_STATE size")
	}
	var capture padCapture
	start := time.Now()
	capture.sample(xinputGamepad{lx: 28000}, true, start)
	capture.sample(xinputGamepad{lx: 800}, true, start.Add(time.Millisecond))
	state := xinputGamepad{lx: 21000, ry: -30000}
	if key, _ := capture.sample(state, true, start.Add(5*time.Millisecond)); key != "" {
		t.Fatal("captured a transitory axis")
	}
	if key, _ := capture.sample(state, true, start.Add(54*time.Millisecond)); key != "GamePad_RStickY" {
		t.Fatal("expected dominant axis, got", key)
	}
	capture = padCapture{}
	capture.sample(xinputGamepad{lt: 5}, false, start)
	capture.sample(xinputGamepad{rt: 180}, false, start)
	if key, _ := capture.sample(xinputGamepad{rt: 110}, false, start); key != "" {
		t.Fatal("trigger hysteresis did not filter noise")
	}
	if key, _ := capture.sample(xinputGamepad{rt: 50}, false, start); key != "Pos_GamePad_RTrigger" {
		t.Fatal("expected digital trigger, got", key)
	}
}
