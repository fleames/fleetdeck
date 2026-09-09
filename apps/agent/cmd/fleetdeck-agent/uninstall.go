package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

const uninstallRequestPath = "/var/lib/fleetdeck/UNINSTALL_REQUESTED"
const uninstallPathUnit = "/etc/systemd/system/fleetdeck-agent-uninstall.path"

// runUninstall removes the FleetDeck agent from this host. Must run as root on Linux.
func runUninstall(delay time.Duration) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("uninstall is only supported on Linux")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("uninstall requires root (use sudo fleetdeck-agent -uninstall)")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	_ = os.Remove(uninstallRequestPath)

	steps := []struct {
		name string
		fn   func() error
	}{
		{"stop service", func() error {
			_ = exec.Command("systemctl", "stop", "fleetdeck-agent.service").Run()
			_ = exec.Command("systemctl", "disable", "fleetdeck-agent.service").Run()
			_ = exec.Command("systemctl", "stop", "fleetdeck-agent-uninstall.path").Run()
			_ = exec.Command("systemctl", "disable", "fleetdeck-agent-uninstall.path").Run()
			_ = exec.Command("systemctl", "stop", "fleetdeck-agent-update.path").Run()
			_ = exec.Command("systemctl", "disable", "fleetdeck-agent-update.path").Run()
			return nil
		}},
		{"remove units", func() error {
			_ = os.Remove("/etc/systemd/system/fleetdeck-agent.service")
			_ = os.Remove("/etc/systemd/system/fleetdeck-agent-uninstall.service")
			_ = os.Remove("/etc/systemd/system/fleetdeck-agent-uninstall.path")
			_ = os.Remove("/etc/systemd/system/fleetdeck-agent-update.service")
			_ = os.Remove("/etc/systemd/system/fleetdeck-agent-update.path")
			_ = exec.Command("systemctl", "daemon-reload").Run()
			_ = exec.Command("systemctl", "reset-failed", "fleetdeck-agent.service").Run()
			return nil
		}},
		{"remove binary", func() error {
			_ = os.Remove("/usr/local/bin/fleetdeck-agent")
			_ = os.Remove("/usr/local/libexec/fleetdeck/uninstall")
			_ = os.Remove("/usr/local/libexec/fleetdeck/update")
			_ = os.Remove("/etc/sudoers.d/fleetdeck-agent")
			return nil
		}},
		{"remove config/state", func() error {
			_ = os.RemoveAll("/etc/fleetdeck")
			_ = os.RemoveAll("/var/lib/fleetdeck")
			return nil
		}},
		{"remove user", tryRemoveFleetdeckUser},
	}

	var errs []string
	for _, s := range steps {
		if err := s.fn(); err != nil {
			errs = append(errs, s.name+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	fmt.Println("FleetDeck agent uninstalled.")
	return nil
}

func tryRemoveFleetdeckUser() error {
	u, err := user.Lookup("fleetdeck")
	if err != nil {
		return nil
	}
	if u.HomeDir != "/var/lib/fleetdeck" && u.HomeDir != "/nonexistent" {
		fmt.Println("Leaving user fleetdeck in place (non-standard home).")
		return nil
	}
	if out, err := exec.Command("userdel", "fleetdeck").CombinedOutput(); err != nil {
		fmt.Printf("Leaving user fleetdeck in place: %v (%s)\n", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func validateUninstallReady() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("uninstall is only supported on Linux systemd hosts")
	}
	if _, err := os.Stat(uninstallPathUnit); err != nil {
		return fmt.Errorf("host missing fleetdeck-agent-uninstall.path (re-run installer, or sudo fleetdeck-agent -uninstall). Use panel force-remove to delete records only")
	}
	return nil
}

// triggerUninstall creates the flag file watched by fleetdeck-agent-uninstall.path.
// The oneshot helper sleeps briefly then runs -uninstall as root.
func triggerUninstall() error {
	if err := os.MkdirAll("/var/lib/fleetdeck", 0o700); err != nil {
		return err
	}
	return os.WriteFile(uninstallRequestPath, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}
