package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	updateRequestPath = "/var/lib/fleetdeck/UPDATE_REQUESTED"
	updatePendingPath = "/var/lib/fleetdeck/pending-update.bin"
	updatePathUnit    = "/etc/systemd/system/fleetdeck-agent-update.path"
	agentBinaryPath   = "/usr/local/bin/fleetdeck-agent"
)

// runApplyUpdate installs a staged binary and restarts the agent service. Must run as root.
// Does not touch enrollment credentials under /var/lib/fleetdeck.
//
// Order matters: stop the unit and kill orphans (manual root starts sharing the
// state-dir) before replacing the binary and starting again. A plain
// `systemctl restart` leaves non-systemd copies running and — with agent.lock —
// can also block the new unit from starting.
func runApplyUpdate(delay time.Duration) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("update is only supported on Linux")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("update requires root (use sudo fleetdeck-agent -update)")
	}
	if delay > 0 {
		time.Sleep(delay)
	}
	_ = os.Remove(updateRequestPath)

	st, err := os.Stat(updatePendingPath)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return fmt.Errorf("no pending update binary at %s", updatePendingPath)
	}

	stopManagedAgentProcesses(agentBinaryPath, filepath.Dir(updatePendingPath), os.Getpid())

	if err := replaceBinary(updatePendingPath, agentBinaryPath); err != nil {
		return err
	}
	_ = os.Remove(updatePendingPath)

	if out, err := exec.Command("systemctl", "start", "fleetdeck-agent.service").CombinedOutput(); err != nil {
		return fmt.Errorf("start fleetdeck-agent: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	fmt.Println("FleetDeck agent binary updated; leftover processes cleared; service started.")
	return nil
}

func replaceBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chown(tmp, 0, 0)
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func validateUpdateReady() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("update is only supported on Linux systemd hosts")
	}
	if _, err := os.Stat(updatePathUnit); err != nil {
		return fmt.Errorf("host missing fleetdeck-agent-update.path (re-run installer). Panel update requires agent 0.4.2-dev+")
	}
	return nil
}

func linuxArchLabel() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64", nil
	case "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported arch %s", runtime.GOARCH)
	}
}

// stageUpdateFromCDN downloads linux-{arch} into the state dir (writable under NoNewPrivileges).
func stageUpdateFromCDN(cdnBase, channel string) (string, error) {
	cdnBase = strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	channel = strings.TrimSpace(channel)
	if cdnBase == "" {
		return "", fmt.Errorf("cdn_base is required")
	}
	if channel == "" {
		channel = "latest"
	}
	arch, err := linuxArchLabel()
	if err != nil {
		return "", err
	}
	name := "linux-" + arch
	binURL := cdnBase + "/" + channel + "/" + name
	if err := os.MkdirAll(filepath.Dir(updatePendingPath), 0o700); err != nil {
		return "", err
	}

	tmp := updatePendingPath + ".part"
	_ = os.Remove(tmp)
	if err := downloadFile(binURL, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("download %s: %w", binURL, err)
	}

	sumsURL := cdnBase + "/" + channel + "/SHA256SUMS"
	sum, err := fetchSHA256For(sumsURL, name)
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("checksum: %w", err)
	}
	got, err := fileSHA256(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if !strings.EqualFold(got, sum) {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("SHA256 mismatch for %s (want %s got %s)", name, sum, got)
	}

	_ = os.Remove(updatePendingPath)
	if err := os.Rename(tmp, updatePendingPath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	_ = os.Chmod(updatePendingPath, 0o755)
	return binURL, nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 3 * time.Minute}
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

// fetchSHA256For returns the expected hash for artifact. Missing SHA256SUMS or
// an unlisted artifact fails closed (CDN updates require checksums).
func fetchSHA256For(sumsURL, artifact string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Get(sumsURL)
	if err != nil {
		return "", fmt.Errorf("fetch SHA256SUMS: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("SHA256SUMS missing at %s (required for agent update)", sumsURL)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching SHA256SUMS", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash, file := fields[0], fields[len(fields)-1]
		file = strings.TrimPrefix(file, "*")
		if filepath.Base(file) == artifact {
			if len(hash) != 64 {
				return "", fmt.Errorf("invalid sha256 in SHA256SUMS")
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("%s not listed in SHA256SUMS", artifact)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// triggerUpdate creates the flag file watched by fleetdeck-agent-update.path.
func triggerUpdate() error {
	if err := os.MkdirAll("/var/lib/fleetdeck", 0o700); err != nil {
		return err
	}
	return os.WriteFile(updateRequestPath, []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o600)
}
