//go:build !windows

package main

import (
	"context"
	"fmt"
	"runtime"
)

func readOSVersion() string { return runtime.GOOS }

func runWindowsService(apiURL, token, stateDir string) error {
	return fmt.Errorf("Windows service mode is only supported on Windows")
}

// silence unused on non-windows
var _ = context.Background
