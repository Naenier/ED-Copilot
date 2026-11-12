//go:build windows

package notifications

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const speechScript = `
$ErrorActionPreference = 'Stop'
[Console]::InputEncoding = [Text.UTF8Encoding]::new($false)
$spec = [Console]::In.ReadToEnd() | ConvertFrom-Json
Add-Type -AssemblyName System.Speech
$voice = New-Object System.Speech.Synthesis.SpeechSynthesizer
try {
    if ($spec.language) {
        $local = @($voice.GetInstalledVoices() | Where-Object { $_.Enabled -and $_.VoiceInfo.Culture.TwoLetterISOLanguageName -eq $spec.language }) | Select-Object -First 1
        if (-not $local) { exit 3 }
        $voice.SelectVoice($local.VoiceInfo.Name)
    }
    $voice.Rate = [int]$spec.rate
    $voice.Volume = [int]$spec.volume
    $voice.Speak([string]$spec.text)
} finally { $voice.Dispose() }
`

func Speak(ctx context.Context, text, language string, rate, volume int) error {
	code := utf16.Encode([]rune(speechScript))
	raw := make([]byte, len(code)*2)
	for i, c := range code {
		raw[i*2], raw[i*2+1] = byte(c), byte(c>>8)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	body, _ := json.Marshal(map[string]any{"text": text, "language": language, "rate": rate, "volume": volume})
	cmd.Stdin = strings.NewReader(string(body))
	if err := cmd.Run(); err != nil {
		return errors.New("локальный голос Windows недоступен")
	}
	return nil
}
