//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// stopManagedAgentProcesses stops systemd's unit (if present), then terminates any
// leftover fleetdeck-agent processes for the managed binary/state-dir — including
// manual root starts that systemctl restart would leave behind.
// excludePID should be the current -update process (and is never signaled).
func stopManagedAgentProcesses(binaryPath, stateDir string, excludePID int) {
	if out, err := exec.Command("systemctl", "stop", "fleetdeck-agent.service").CombinedOutput(); err != nil {
		// Unit may be absent on partial installs; continue to orphan cleanup.
		fmt.Fprintf(os.Stderr, "update: systemctl stop fleetdeck-agent: %v (%s)\n", err, strings.TrimSpace(string(out)))
	}

	exclude := map[int]struct{}{excludePID: {}, os.Getpid(): {}}
	if ppid := os.Getppid(); ppid > 1 {
		exclude[ppid] = struct{}{}
	}

	pids := findAgentPIDs(binaryPath, stateDir, exclude)
	if len(pids) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "update: stopping %d leftover fleetdeck-agent process(es): %v\n", len(pids), pids)
	signalPIDs(pids, syscall.SIGTERM)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		alive := livingPIDs(pids)
		if len(alive) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
		pids = alive
	}
	alive := livingPIDs(pids)
	if len(alive) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "update: SIGKILL leftover fleetdeck-agent process(es): %v\n", alive)
	signalPIDs(alive, syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
}

func findAgentPIDs(binaryPath, stateDir string, exclude map[int]struct{}) []int {
	binaryPath = filepath.Clean(binaryPath)
	stateDir = filepath.Clean(stateDir)
	seen := map[int]struct{}{}
	var out []int

	add := func(pid int) {
		if pid <= 1 {
			return
		}
		if _, skip := exclude[pid]; skip {
			return
		}
		if _, ok := seen[pid]; ok {
			return
		}
		seen[pid] = struct{}{}
		out = append(out, pid)
	}

	if pid, ok := readLockPID(filepath.Join(stateDir, "agent.lock")); ok {
		if matchesAgentProcess(pid, binaryPath, stateDir) {
			add(pid)
		}
	}

	ents, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(ent.Name())
		if err != nil {
			continue
		}
		if _, skip := exclude[pid]; skip {
			continue
		}
		if matchesAgentProcess(pid, binaryPath, stateDir) {
			add(pid)
		}
	}
	return out
}

func readLockPID(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	line := strings.TrimSpace(string(b))
	if line == "" {
		return 0, false
	}
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	pid, err := strconv.Atoi(line)
	if err != nil || pid <= 1 {
		return 0, false
	}
	return pid, true
}

func matchesAgentProcess(pid int, binaryPath, stateDir string) bool {
	exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err == nil {
		exeClean := strings.TrimSuffix(exe, " (deleted)")
		if filepath.Clean(exeClean) == binaryPath {
			return true
		}
	}

	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(cmdline) == 0 {
		return false
	}
	return cmdlineMatchesAgent(cmdline, binaryPath, stateDir)
}

func signalPIDs(pids []int, sig syscall.Signal) {
	for _, pid := range pids {
		_ = syscall.Kill(pid, sig)
	}
}

func livingPIDs(pids []int) []int {
	var alive []int
	for _, pid := range pids {
		if err := syscall.Kill(pid, 0); err == nil {
			alive = append(alive, pid)
		}
	}
	return alive
}
