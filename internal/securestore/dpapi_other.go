//go:build !windows

package securestore

import "errors"

func available() bool { return false }
func protect([]byte) ([]byte, error) {
	return nil, errors.New("Требуется защищённое хранилище Windows.")
}
func unprotect([]byte) ([]byte, error) {
	return nil, errors.New("Требуется защищённое хранилище Windows.")
}
