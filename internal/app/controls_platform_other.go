//go:build !windows

package app

func eliteMayBeRunning() bool     { return true }
func eliteControlSchemes() string { return "" }
