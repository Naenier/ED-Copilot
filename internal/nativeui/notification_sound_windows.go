//go:build windows

package nativeui

import (
	"encoding/binary"
	"errors"
	"math"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var playNotificationWave = windows.NewLazySystemDLL("winmm.dll").NewProc("PlaySoundW")

type notificationSoundSource interface {
	TakeNotificationSound() int
}

type notificationSoundPlayer struct {
	backend  Controller
	source   notificationSoundSource
	previews chan struct{}
}

func (u *Window) startNotificationSounds() {
	if u.options.Smoke || u.options.Demo || u.options.ReadmeScreenshots || u.options.Soak != nil {
		return
	}
	source, ok := u.backend.(notificationSoundSource)
	if !ok {
		return
	}
	player := &notificationSoundPlayer{backend: u.backend, source: source, previews: make(chan struct{}, 1)}
	u.notificationSounds = player
	go player.run(u.stop)
}

func (p *notificationSoundPlayer) preview() error {
	if p == nil {
		return errors.New("Проверка звука недоступна в этом режиме.")
	}
	select {
	case p.previews <- struct{}{}:
	default:
		// One preview is already queued.
	}
	return nil
}

func (p *notificationSoundPlayer) run(stop <-chan struct{}) {
	const sndMemory = 0x0004
	const sndNoDefault = 0x0002
	ticker := time.NewTicker(450 * time.Millisecond)
	defer ticker.Stop()
	waves := [4][]byte{}
	failed := false
	for {
		var priority int
		select {
		case <-stop:
			return
		case <-p.previews:
			priority = 2
		case <-ticker.C:
			priority = p.source.TakeNotificationSound()
		}
		if priority < 1 || priority >= len(waves) {
			continue
		}
		select {
		case <-stop:
			return
		default:
		}
		if waves[priority] == nil {
			waves[priority] = notificationWave(priority)
		}
		var ok uintptr
		if playNotificationWave.Find() == nil {
			// Synchronous playback owns the buffer only for the duration of
			// the call. It runs off the UI, journal and HUD update threads.
			ok, _, _ = playNotificationWave.Call(uintptr(unsafe.Pointer(&waves[priority][0])), 0, sndMemory|sndNoDefault)
			runtime.KeepAlive(waves[priority])
		}
		if ok == 0 && !failed {
			recordWebOperation(p.backend, "ui", "notification.sound", time.Time{}, nil, errors.New("Не удалось воспроизвести сигнал."))
		}
		failed = ok == 0
	}
}

func notificationWave(priority int) []byte {
	const rate = 22050
	const attack = 0.018
	const release = 0.048
	type note struct {
		start, length, frequency float64
	}
	notes := []note{{0, 0.155, 590}}
	switch priority {
	case 2:
		notes = []note{{0, 0.120, 520}, {0.147, 0.135, 690}}
	case 3:
		notes = []note{{0, 0.116, 790}, {0.143, 0.155, 580}}
	}
	last := notes[len(notes)-1]
	count := int(math.Ceil((last.start + last.length) * rate))
	out := make([]byte, 44+2*count)
	u16, u32 := binary.LittleEndian.PutUint16, binary.LittleEndian.PutUint32
	copy(out, "RIFF")
	u32(out[4:], uint32(len(out)-8))
	copy(out[8:], "WAVEfmt ")
	u32(out[16:], 16)
	u16(out[20:], 1) // PCM
	u16(out[22:], 1) // Mono
	u32(out[24:], rate)
	u32(out[28:], 2*rate)
	u16(out[32:], 2)
	u16(out[34:], 16)
	copy(out[36:], "data")
	u32(out[40:], uint32(2*count))
	for _, n := range notes {
		for i := int(math.Ceil(n.start * rate)); i < count; i++ {
			t := float64(i)/rate - n.start
			if t >= n.length {
				break
			}
			amplitude := min(t/attack, (n.length-t)/release, 1)
			envelope := 0.5 - 0.5*math.Cos(math.Pi*amplitude)
			phase := 2 * math.Pi * n.frequency * t
			sample := (math.Sin(phase) + 0.12*math.Sin(2*phase)) * envelope * 3400
			u16(out[44+2*i:], uint16(int16(sample)))
		}
	}
	return out
}
