//go:build !windows

package main

import "fmt"

func triggerRMMUpdate(stateDir string) error {
	return fmt.Errorf("RMM agent update apply is only supported on Windows")
}
