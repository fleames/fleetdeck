package api

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
)

var (
	agentPayloadMu    sync.Mutex
	agentPayloadCache = map[string]string{} // arch -> gzip+base64 of binary only
)

type installRequest struct {
	Token  string `json:"token"`
	Arch   string `json:"arch"`
	APIURL string `json:"api_url"`
	Name   string `json:"name"`
}

func (s *Server) parseInstallRequest(r *http.Request) (installRequest, error) {
	var body installRequest
	if err := httpx.Decode(r, &body); err != nil {
		return body, err
	}
	body.Token = strings.TrimSpace(body.Token)
	body.APIURL = strings.TrimRight(strings.TrimSpace(body.APIURL), "/")
	body.Arch = strings.ToLower(strings.TrimSpace(body.Arch))
	body.Name = strings.TrimSpace(body.Name)
	if body.Arch == "" {
		body.Arch = "amd64"
	}
	if body.Arch == "x86_64" {
		body.Arch = "amd64"
	}
	if body.Arch == "aarch64" {
		body.Arch = "arm64"
	}
	if body.Arch != "amd64" && body.Arch != "arm64" {
		return body, fmt.Errorf("arch must be amd64 or arm64")
	}
	if body.Token == "" {
		return body, fmt.Errorf("token is required")
	}
	if body.APIURL == "" {
		body.APIURL = strings.TrimRight(s.cfg.APIPublicURL, "/")
	}
	if body.APIURL == "" {
		return body, fmt.Errorf("api_url is required")
	}
	return body, nil
}

func (s *Server) handleInstallOneline(w http.ResponseWriter, r *http.Request) {
	body, err := s.parseInstallRequest(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	script, size, err := s.buildOfflineInstallScript(body.Arch, body.APIURL, body.Token)
	if err != nil {
		httpx.ErrorDetails(w, http.StatusNotFound, "agent_binary_missing",
			"Could not build offline installer (agent binary missing on API).",
			map[string]any{"arch": body.Arch},
			map[string]any{"technical": err.Error()})
		return
	}
	cmd := wrapScriptAsChunkedOneline(script)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"arch":          body.Arch,
		"api_url":       body.APIURL,
		"command":       cmd,
		"command_bytes": len(cmd),
		"payload_bytes": size,
		"note":          "Offline paste one-liner. Prefer downloading the .sh bundle and scp'ing it to the VPS.",
	})
}

// handleInstallBundle returns a portable self-extracting .sh (binary embedded).
// Transfer with scp from your LAN PC — the VPS never needs to download from FleetDeck.
func (s *Server) handleInstallBundle(w http.ResponseWriter, r *http.Request) {
	body, err := s.parseInstallRequest(r)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	script, _, err := s.buildOfflineInstallScript(body.Arch, body.APIURL, body.Token)
	if err != nil {
		httpx.ErrorDetails(w, http.StatusNotFound, "agent_binary_missing",
			"Could not build offline installer (agent binary missing on API).",
			map[string]any{"arch": body.Arch},
			map[string]any{"technical": err.Error()})
		return
	}
	name := body.Name
	if name == "" {
		name = "server"
	}
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	filename := fmt.Sprintf("fleetdeck-install-%s-%s.sh", safe, body.Arch)
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-FleetDeck-API-URL", body.APIURL)
	w.Header().Set("X-FleetDeck-Arch", body.Arch)
	_, _ = w.Write([]byte(script))
}

func (s *Server) buildOfflineInstallScript(arch, apiURL, token string) (script string, payloadBytes int, err error) {
	binB64GZ, err := s.cachedAgentGzipB64(arch)
	if err != nil {
		return "", 0, err
	}
	esc := func(v string) string {
		return strings.ReplaceAll(v, `'`, `'\''`)
	}
	script = fmt.Sprintf(`#!/usr/bin/env bash
# FleetDeck offline agent installer (self-contained — no download from FleetDeck)
# Usage on VPS:  sudo bash fleetdeck-install-*.sh
# Transfer from LAN:  scp fleetdeck-install-*.sh user@vps:/tmp/ && ssh user@vps 'sudo bash /tmp/fleetdeck-install-*.sh'
set -euo pipefail
API_URL='%s'
TOKEN='%s'
INTERVAL='10s'
while [[ $# -gt 0 ]]; do
  case "$1" in
    --token|-t) TOKEN="$2"; shift 2 ;;
    --api) API_URL="$2"; shift 2 ;;
    --interval) INTERVAL="$2"; shift 2 ;;
    *) shift ;;
  esac
done
if [[ "$(id -u)" -ne 0 ]]; then echo "Re-run with sudo." >&2; exit 1; fi
if [[ -z "$TOKEN" || -z "$API_URL" ]]; then echo "token and api required" >&2; exit 1; fi
echo "Installing FleetDeck agent → $API_URL"
if ! id fleetdeck >/dev/null 2>&1; then useradd --system --home /var/lib/fleetdeck --shell /usr/sbin/nologin fleetdeck; fi
install -d -m 0755 /etc/fleetdeck
install -d -m 0700 -o fleetdeck -g fleetdeck /var/lib/fleetdeck
AGENT_B64_GZ='%s'
echo "$AGENT_B64_GZ" | base64 -d | gzip -d > /usr/local/bin/fleetdeck-agent
chmod 0755 /usr/local/bin/fleetdeck-agent
install -d -m 0755 /usr/local/libexec/fleetdeck
cat > /usr/local/libexec/fleetdeck/uninstall <<'EOF'
#!/bin/bash
set -euo pipefail
sleep 2
exec /usr/local/bin/fleetdeck-agent -uninstall
EOF
chmod 0755 /usr/local/libexec/fleetdeck/uninstall
cat > /usr/local/libexec/fleetdeck/update <<'EOF'
#!/bin/bash
set -euo pipefail
sleep 2
PENDING=/var/lib/fleetdeck/pending-update.bin
if [[ -x "$PENDING" && -s "$PENDING" ]]; then
  exec "$PENDING" -update
fi
exec /usr/local/bin/fleetdeck-agent -update
EOF
chmod 0755 /usr/local/libexec/fleetdeck/update
cat > /etc/fleetdeck/agent.env <<EOF
FLEETDECK_URL=${API_URL}
EOF
chmod 0640 /etc/fleetdeck/agent.env
chown root:fleetdeck /etc/fleetdeck/agent.env
cat > /etc/systemd/system/fleetdeck-agent.service <<'EOF'
[Unit]
Description=FleetDeck monitoring agent
After=network-online.target docker.service
Wants=network-online.target
[Service]
Type=simple
User=fleetdeck
Group=fleetdeck
EnvironmentFile=-/etc/fleetdeck/agent.env
ExecStart=/usr/local/bin/fleetdeck-agent -api ${FLEETDECK_URL} -state-dir /var/lib/fleetdeck -interval 10s
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/fleetdeck
[Install]
WantedBy=multi-user.target
EOF
sed -i "s|-interval 10s|-interval ${INTERVAL}|g" /etc/systemd/system/fleetdeck-agent.service
if getent group docker >/dev/null 2>&1; then usermod -aG docker fleetdeck || true; sed -i '/^Group=fleetdeck$/a SupplementaryGroups=docker' /etc/systemd/system/fleetdeck-agent.service; fi
cat > /etc/systemd/system/fleetdeck-agent-uninstall.service <<'EOF'
[Unit]
Description=FleetDeck agent uninstall (oneshot)
After=network.target
[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/uninstall
EOF
cat > /etc/systemd/system/fleetdeck-agent-uninstall.path <<'EOF'
[Unit]
Description=Watch for FleetDeck agent uninstall request
[Path]
PathExists=/var/lib/fleetdeck/UNINSTALL_REQUESTED
[Install]
WantedBy=multi-user.target
EOF
cat > /etc/systemd/system/fleetdeck-agent-update.service <<'EOF'
[Unit]
Description=FleetDeck agent update (oneshot)
After=network.target
[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/update
EOF
cat > /etc/systemd/system/fleetdeck-agent-update.path <<'EOF'
[Unit]
Description=Watch for FleetDeck agent update request
[Path]
PathExists=/var/lib/fleetdeck/UPDATE_REQUESTED
[Install]
WantedBy=multi-user.target
EOF
echo "Enrolling…"
umask 077
if ! sudo -u fleetdeck /usr/local/bin/fleetdeck-agent -api "$API_URL" -token "$TOKEN" -state-dir /var/lib/fleetdeck -enroll; then
  echo "Enrollment failed. The VPS must be able to REACH $API_URL (VPN/Tailscale/tunnel)." >&2
  echo "Installer binary is local; only enrollment/metrics need network to FleetDeck." >&2
  exit 1
fi
chown -R fleetdeck:fleetdeck /var/lib/fleetdeck
chmod 0700 /var/lib/fleetdeck
chmod 0600 /var/lib/fleetdeck/credentials.json 2>/dev/null || true
systemctl daemon-reload
systemctl enable --now fleetdeck-agent-uninstall.path
systemctl enable --now fleetdeck-agent-update.path
systemctl enable --now fleetdeck-agent.service
echo "FleetDeck agent installed and running."
echo "Logs: journalctl -u fleetdeck-agent -f"
`, esc(apiURL), esc(token), binB64GZ)
	return script, len(binB64GZ), nil
}

func wrapScriptAsChunkedOneline(script string) string {
	outer := base64.StdEncoding.EncodeToString([]byte(script))
	const chunk = 60000
	var b strings.Builder
	b.WriteString(`f=$(mktemp); `)
	for i := 0; i < len(outer); i += chunk {
		end := i + chunk
		if end > len(outer) {
			end = len(outer)
		}
		b.WriteString(`echo -n '`)
		b.WriteString(outer[i:end])
		b.WriteString(`' >>"$f"; `)
	}
	b.WriteString(`base64 -d "$f" | sudo bash -s; rm -f "$f"`)
	return b.String()
}

func (s *Server) cachedAgentGzipB64(arch string) (string, error) {
	agentPayloadMu.Lock()
	defer agentPayloadMu.Unlock()
	if v, ok := agentPayloadCache[arch]; ok {
		return v, nil
	}
	path, err := s.resolveAgentBinary(arch)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var gz bytes.Buffer
	w, err := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(raw); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	enc := base64.StdEncoding.EncodeToString(gz.Bytes())
	agentPayloadCache[arch] = enc
	return enc, nil
}
