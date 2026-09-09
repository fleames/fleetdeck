#!/usr/bin/env bash
# FleetDeck agent installer — hosted on CDN (cdn.tarkovbot.com).
# VPS one-liner (API URL comes from CDN config.json by default):
#   curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | sudo bash -s -- --token 'TOKEN'
set -euo pipefail

CDN_BASE="${FLEETDECK_CDN:-https://cdn.tarkovbot.com/fleetdeck}"
CHANNEL="${FLEETDECK_CHANNEL:-latest}"
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
    -h|--help)
      echo "Usage: curl -fsSL ${CDN_BASE}/install.sh | sudo bash -s -- --token TOKEN [--api API_URL]"
      exit 0
      ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi
if [[ -z "$TOKEN" ]]; then
  echo "Missing --token (create one in FleetDeck → Servers)." >&2
  exit 1
fi

if [[ -z "$API_URL" ]]; then
  CFG="$(curl -fsSL "${CDN_BASE}/config.json" 2>/dev/null || true)"
  if [[ -n "$CFG" ]]; then
    API_URL="$(printf '%s' "$CFG" | sed -n 's/.*"api_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  fi
fi
if [[ -z "$API_URL" ]]; then
  API_URL="https://agents.tarkovbot.com"
fi
API_URL="${API_URL%/}"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH_LABEL=amd64 ;;
  aarch64|arm64) ARCH_LABEL=arm64 ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
BIN="$TMP/fleetdeck-agent"
URL="${CDN_BASE}/${CHANNEL}/linux-${ARCH_LABEL}"
echo "Downloading agent from ${URL} …"
curl -fsSL "$URL" -o "$BIN"
chmod +x "$BIN"

if ! id fleetdeck >/dev/null 2>&1; then
  useradd --system --home /var/lib/fleetdeck --shell /usr/sbin/nologin fleetdeck
fi
if getent group docker >/dev/null 2>&1; then
  usermod -aG docker fleetdeck || true
fi

install -d -m 0755 /etc/fleetdeck
install -d -m 0700 -o fleetdeck -g fleetdeck /var/lib/fleetdeck
install -m 0755 "$BIN" /usr/local/bin/fleetdeck-agent

install -d -m 0755 /usr/local/libexec/fleetdeck
cat > /usr/local/libexec/fleetdeck/uninstall <<'EOF'
#!/bin/bash
# Triggered by fleetdeck-agent-uninstall.path; delay so agent can report command result.
set -euo pipefail
sleep 2
exec /usr/local/bin/fleetdeck-agent -uninstall
EOF
chmod 0755 /usr/local/libexec/fleetdeck/uninstall

cat > /usr/local/libexec/fleetdeck/update <<'EOF'
#!/bin/bash
# Triggered by fleetdeck-agent-update.path; delay so agent can report command result.
# Applies staged binary from /var/lib/fleetdeck/pending-update.bin (credentials untouched).
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
SupplementaryGroups=docker
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

echo "Checking API ${API_URL}/healthz …"
if ! curl -fsS --max-time 10 "${API_URL}/healthz" >/dev/null; then
  echo "API not reachable. Is FleetDeck running with Cloudflare Tunnel up?" >&2
  exit 1
fi

echo "Enrolling with ${API_URL} …"
if ! sudo -u fleetdeck /usr/local/bin/fleetdeck-agent \
  -api "$API_URL" \
  -token "$TOKEN" \
  -state-dir /var/lib/fleetdeck \
  -enroll; then
  echo "Enrollment failed — is FleetDeck + Cloudflare Tunnel online on your PC?" >&2
  exit 1
fi

chown -R fleetdeck:fleetdeck /var/lib/fleetdeck
chmod 0600 /var/lib/fleetdeck/credentials.json 2>/dev/null || true

systemctl daemon-reload
systemctl enable --now fleetdeck-agent-uninstall.path
systemctl enable --now fleetdeck-agent-update.path
systemctl enable --now fleetdeck-agent.service
systemctl --no-pager --full status fleetdeck-agent.service || true
echo
echo "Done. Agent installed from CDN; metrics dial ${API_URL}."
echo "Logs: journalctl -u fleetdeck-agent -f"
echo "Update/uninstall from panel, or: sudo fleetdeck-agent -update / -uninstall"
