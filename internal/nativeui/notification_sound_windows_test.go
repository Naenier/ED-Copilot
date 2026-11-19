//go:build windows

package nativeui

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNotificationWaveIsShortSoftAndDistinct(t *testing.T) {
	waves := make([][]byte, 0, 3)
	for priority := 1; priority <= 3; priority++ {
		wave := notificationWave(priority)
		u16, u32 := binary.LittleEndian.Uint16, binary.LittleEndian.Uint32
		if string(wave[:4]) != "RIFF" || int(u32(wave[4:])) != len(wave)-8 ||
			string(wave[8:16]) != "WAVEfmt " || u16(wave[20:]) != 1 || u16(wave[22:]) != 1 ||
			u32(wave[24:]) != 22050 || u16(wave[34:]) != 16 || string(wave[36:40]) != "data" ||
			int(u32(wave[40:])) != len(wave)-44 {
			t.Fatalf("priority %d has an invalid PCM header", priority)
		}
		if samples := (len(wave) - 44) / 2; samples < 2205 || samples > 7720 {
			t.Errorf("priority %d has %d samples, outside 100–350 ms", priority, samples)
		}
		var peak int
		for i := 44; i < len(wave); i += 2 {
			sample := int(int16(u16(wave[i:])))
			peak = max(peak, sample, -sample)
		}
		if peak < 1000 || peak > 4000 || u16(wave[44:]) != 0 || u16(wave[len(wave)-2:]) != 0 {
			t.Errorf("priority %d has an unexpected amplitude or abrupt transition", priority)
		}
		for _, previous := range waves {
			if bytes.Equal(wave, previous) {
				t.Errorf("priority %d has the same cue as another priority", priority)
			}
		}
		waves = append(waves, wave)
	}
}
