package main

import (
	"bytes"
	"path"
	"strings"
)

const defaultManagedStateDir = "/var/lib/fleetdeck"

// cmdlineMatchesAgent reports whether a /proc/pid/cmdline payload is a fleetdeck-agent
// for the managed binary and state directory.
// Paths are treated as Unix (slash-separated) so matching is correct when unit-tested on Windows.
func cmdlineMatchesAgent(cmdline []byte, binaryPath, stateDir string) bool {
	args := nullSplit(cmdline)
	if len(args) == 0 {
		return false
	}
	arg0 := args[0]
	base0 := path.Base(arg0)
	binBase := path.Base(binaryPath)
	if base0 != binBase && base0 != "fleetdeck-agent" {
		return false
	}
	if strings.HasPrefix(arg0, "/") {
		if path.Clean(arg0) != path.Clean(binaryPath) {
			// Different install prefix (e.g. user ~/.local/bin) — leave alone.
			return false
		}
	}

	stated := stateDirFromArgs(args)
	if stated == "" {
		// No -state-dir: only treat as managed if default state dir is the target.
		return path.Clean(stateDir) == defaultManagedStateDir
	}
	return path.Clean(stated) == path.Clean(stateDir)
}

func stateDirFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-state-dir" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "-state-dir=") {
			return strings.TrimPrefix(a, "-state-dir=")
		}
	}
	return ""
}

func nullSplit(b []byte) []string {
	b = bytes.TrimRight(b, "\x00")
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		out = append(out, string(p))
	}
	return out
}
