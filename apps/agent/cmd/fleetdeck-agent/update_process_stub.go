//go:build !linux

package main

// stopManagedAgentProcesses is a no-op off Linux (agent update is Linux-only).
func stopManagedAgentProcesses(binaryPath, stateDir string, excludePID int) {}
