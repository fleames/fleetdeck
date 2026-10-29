package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var rmmCDNChannelRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

const (
	rmmPendingName = "pending-update.exe"
	rmmServiceName = "FleetDeckRMM"
)

func pendingUpdatePath(stateDir string) string {
	return filepath.Join(stateDir, rmmPendingName)
}

// stageRMMUpdateFromCDN downloads fleetdeck-rmm.exe (fallback windows-amd64) into stateDir.
func stageRMMUpdateFromCDN(cdnBase, channel, stateDir string) (string, error) {
	cdnBase = strings.TrimRight(strings.TrimSpace(cdnBase), "/")
	channel = strings.TrimSpace(channel)
	if cdnBase == "" {
		return "", fmt.Errorf("cdn_base is required")
	}
	if channel == "" {
		channel = "latest"
	}
	if !rmmCDNChannelRe.MatchString(channel) {
		return "", fmt.Errorf("invalid CDN channel")
	}
	if strings.Contains(cdnBase, "://") {
		// Reject unexpected schemes / userinfo that could redirect update fetches.
		if !strings.HasPrefix(cdnBase, "https://") && !strings.HasPrefix(cdnBase, "http://") {
			return "", fmt.Errorf("cdn_base must be http(s)")
		}
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return "", err
	}

	// Prefer extensionless key: custom-domain cache has served stale .exe while
	// windows-amd64 stayed current after R2 overwrite.
	candidates := []string{"windows-amd64", "fleetdeck-rmm.exe"}
	var lastErr error
	pending := pendingUpdatePath(stateDir)
	tmp := pending + ".part"
	for _, name := range candidates {
		binURL := cdnBase + "/" + channel + "/" + name
		_ = os.Remove(tmp)
		if err := downloadFile(binURL, tmp); err != nil {
			lastErr = fmt.Errorf("%s: %w", binURL, err)
			continue
		}
		st, err := os.Stat(tmp)
		if err != nil || st.Size() < 100000 {
			_ = os.Remove(tmp)
			lastErr = fmt.Errorf("%s: download too small", binURL)
			continue
		}
		// PE MZ check
		f, err := os.Open(tmp)
		if err != nil {
			_ = os.Remove(tmp)
			lastErr = err
			continue
		}
		hdr := make([]byte, 2)
		_, _ = f.Read(hdr)
		f.Close()
		if hdr[0] != 'M' || hdr[1] != 'Z' {
			_ = os.Remove(tmp)
			lastErr = fmt.Errorf("%s: not a Windows PE binary", binURL)
			continue
		}

		sumsURL := cdnBase + "/" + channel + "/SHA256SUMS"
		sum, err := fetchSHA256For(sumsURL, name)
		if err != nil {
			// Also accept the other artifact name if hashes are identical.
			if name == "fleetdeck-rmm.exe" {
				sum, err = fetchSHA256For(sumsURL, "windows-amd64")
			}
			if err != nil {
				_ = os.Remove(tmp)
				lastErr = fmt.Errorf("checksum: %w", err)
				continue
			}
		}
		got, err := fileSHA256(tmp)
		if err != nil {
			_ = os.Remove(tmp)
			lastErr = err
			continue
		}
		if !strings.EqualFold(got, sum) {
			_ = os.Remove(tmp)
			lastErr = fmt.Errorf("SHA256 mismatch for %s (want %s got %s)", name, sum, got)
			continue
		}

		_ = os.Remove(pending)
		if err := os.Rename(tmp, pending); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		_ = os.Chmod(pending, 0o755)
		return binURL, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no RMM binary candidate succeeded")
	}
	return "", lastErr
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

func installBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Clean(exe), nil
}
