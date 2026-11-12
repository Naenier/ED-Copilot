//go:build !windows

package notifications

import (
	"context"
	"errors"
)

func Speak(context.Context, string, string, int, int) error {
	return errors.New("Windows speech is unavailable")
}
