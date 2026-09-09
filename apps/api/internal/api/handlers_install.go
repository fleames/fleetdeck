package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fleetdeck/fleetdeck/apps/api/internal/httpx"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleInstallScript(w http.ResponseWriter, r *http.Request) {
	cdn := strings.TrimRight(s.cfg.AgentCDNBase, "/")
	if cdn == "" {
		cdn = "https://cdn.tarkovbot.com/fleetdeck"
	}
	apiDefault := strings.TrimRight(s.cfg.APIPublicURL, "/")
	if apiDefault == "" {
		apiDefault = "https://agents.tarkovbot.com"
	}
	channel := s.cfg.AgentCDNChannel
	if channel == "" {
		channel = "latest"
	}

	script := fmt.Sprintf(`#!/usr/bin/env bash
# Prefer the CDN copy: curl -fsSL %s/install.sh | sudo bash -s -- --token TOKEN
# Seedbox / no sudo: curl -fsSL %s/install.sh | bash -s -- --user --token TOKEN
set -euo pipefail
CDN_BASE=%q
CHANNEL=%q
API_URL="${FLEETDECK_URL:-}"
TOKEN=""
INTERVAL="10s"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --token|-t) TOKEN="$2"; shift 2 ;;
    --api) API_URL="$2"; shift 2 ;;
    --interval) INTERVAL="$2"; shift 2 ;;
    --cdn) CDN_BASE="$2"; shift 2 ;;
    --channel) CHANNEL="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done
if [[ "$(id -u)" -ne 0 ]]; then echo "Run as root (sudo), or use CDN install.sh --user for seedboxes." >&2; exit 1; fi
if [[ -z "$TOKEN" ]]; then echo "Need --token" >&2; exit 1; fi
if [[ -z "$API_URL" ]]; then
  CFG="$(curl -fsSL "${CDN_BASE}/config.json" 2>/dev/null || true)"
  if [[ -n "$CFG" ]]; then
    API_URL="$(printf '%%s' "$CFG" | sed -n 's/.*"api_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  fi
fi
if [[ -z "$API_URL" ]]; then API_URL=%q; fi
API_URL="${API_URL%%/}"
ARCH="$(uname -m)"
case "$ARCH" in x86_64|amd64) ARCH_LABEL=amd64 ;; aarch64|arm64) ARCH_LABEL=arm64 ;; *) echo "Unsupported arch $ARCH" >&2; exit 1 ;; esac
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/fleetdeck-agent"
curl -fsSL "${CDN_BASE}/${CHANNEL}/linux-${ARCH_LABEL}" -o "$BIN"
chmod +x "$BIN"
if ! id fleetdeck >/dev/null 2>&1; then useradd --system --home /var/lib/fleetdeck --shell /usr/sbin/nologin fleetdeck; fi
install -d -m 0755 /etc/fleetdeck
install -d -m 0700 -o fleetdeck -g fleetdeck /var/lib/fleetdeck
install -m 0755 "$BIN" /usr/local/bin/fleetdeck-agent
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
umask 077
sudo -u fleetdeck /usr/local/bin/fleetdeck-agent -api "$API_URL" -token "$TOKEN" -state-dir /var/lib/fleetdeck -enroll
chown -R fleetdeck:fleetdeck /var/lib/fleetdeck
chmod 0700 /var/lib/fleetdeck
chmod 0600 /var/lib/fleetdeck/credentials.json 2>/dev/null || true
systemctl daemon-reload
systemctl enable --now fleetdeck-agent-uninstall.path
systemctl enable --now fleetdeck-agent-update.path
systemctl enable --now fleetdeck-agent.service
echo "Done. Logs: journalctl -u fleetdeck-agent -f"
`, cdn, cdn, cdn, channel, apiDefault)

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=install-fleetdeck-agent.sh")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(script))
}

func (s *Server) handleInstallAgentBinary(w http.ResponseWriter, r *http.Request) {
	arch := strings.ToLower(chi.URLParam(r, "arch"))
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	if arch != "amd64" && arch != "arm64" {
		httpx.Error(w, http.StatusNotFound, "not_found", "Unknown arch. Use amd64 or arm64.")
		return
	}

	path, err := s.resolveAgentBinary(arch)
	if err != nil {
		httpx.ErrorDetails(w, http.StatusNotFound, "agent_binary_missing",
			"Linux agent binary is not available on this API. Rebuild the API image with agent dist bundled.",
			map[string]any{"arch": arch, "agent_dist_dir": s.cfg.AgentDistDir},
			map[string]any{"technical": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=fleetdeck-agent-linux-"+arch)
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeFile(w, r, path)
}

func (s *Server) resolveAgentBinary(arch string) (string, error) {
	dir := s.cfg.AgentDistDir
	candidates := []string{
		filepath.Join(dir, "linux-"+arch),
		filepath.Join(dir, "fleetdeck-agent-linux-"+arch),
		filepath.Join(dir, fmt.Sprintf("fleetdeck-agent_0.4.0-dev_linux_%s", arch)),
		filepath.Join(dir, fmt.Sprintf("fleetdeck-agent_linux_%s", arch)),
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			name := e.Name()
			if strings.Contains(name, "linux_"+arch) && !strings.HasSuffix(name, ".exe") {
				candidates = append(candidates, filepath.Join(dir, name))
			}
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() && st.Size() > 0 {
			return c, nil
		}
	}
	return "", fmt.Errorf("no binary for linux/%s under %s", arch, dir)
}
