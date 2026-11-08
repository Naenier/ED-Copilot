//go:build !windows

package updater

import (
	"context"
	"errors"
)

func installedExecutable(string) bool       { return false }
func launchDetached(string, []string) error { return errors.New("automatic installer is Windows-only") }

type platformHelper struct{}

func (platformHelper) installed(string) bool { return false }
func (platformHelper) fileVersion(string) (string, error) {
	return "", errors.New("Windows-only version resource")
}
func platformStartupGate(context.Context) (func(), error) {
	return nil, errors.New("Windows-only startup gate")
}
func (platformHelper) lock() (func(), error) { return nil, errors.New("Windows-only helper") }

func (platformHelper) waitParent(context.Context, int, string) error {
	return errors.New("Windows-only helper")
}
func (platformHelper) waitDesktop(context.Context) error { return errors.New("Windows-only helper") }
func (platformHelper) install(context.Context, string, []string) (int, error) {
	return -1, errors.New("Windows-only helper")
}
func (platformHelper) restart(string, []string) error { return errors.New("Windows-only helper") }
