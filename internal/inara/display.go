package inara

import "strings"

// displayError localizes known service messages only in the public status.
// The stored response and retry/send decisions retain their original values.
func displayError(raw, key string) string {
	message := safeMessage(raw, key)
	if strings.EqualFold(strings.TrimSpace(message), "This application has no access allowed.") {
		return "Inara пока не разрешила доступ для ED Copilot. Ключ сохранён; отправка возобновится после разрешения сервиса."
	}
	return message
}
