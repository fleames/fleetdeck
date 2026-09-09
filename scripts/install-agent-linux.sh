#!/usr/bin/env bash
# Install FleetDeck agent on Linux (amd64 or arm64).
# Usage:
#   sudo ./install-agent-linux.sh \
#     --api https://fleetdeck.example:8080 \
#     --token <enrollment-token> \
#     [--version 0.4.0-dev] \
#     [--binary /path/to/fleetdeck-agent_..._linux_amd64]
set -euo pipefail

API_URL=""
TOKEN=""
VERSION="0.4.2-dev"
BINARY=""
PREFIX="/usr/local"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api) API_URL="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --binary) BINARY="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,9p' "$0"
      exit 0
      ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi
if [[ -z "$API_URL" || -z "$TOKEN" ]]; then
  echo "--api and --token are required." >&2
  exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH_LABEL=amd64 ;;
  aarch64|arm64) ARCH_LABEL=arm64 ;;
  *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;;
esac

if [[ -z "$BINARY" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  CANDIDATE="${SCRIPT_DIR}/../dist/agent/${VERSION}/fleetdeck-agent_${VERSION}_linux_${ARCH_LABEL}"
  if [[ -f "$CANDIDATE" ]]; then
    BINARY="$CANDIDATE"
  else
    echo "Binary not found at $CANDIDATE" >&2
    echo "Copy the linux_${ARCH_LABEL} release binary here or pass --binary /path/to/binary" >&2
    exit 1
  fi
fi

install -d -m 0755 /etc/fleetdeck /var/lib/fleetdeck
if ! id fleetdeck >/dev/null 2>&1; then
  useradd --system --home /var/lib/fleetdeck --shell /usr/sbin/nologin fleetdeck
fi
install -m 0755 "$BINARY" "${PREFIX}/bin/fleetdeck-agent"
chown root:root "${PREFIX}/bin/fleetdeck-agent"

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

UNIT_SRC="$(cd "$(dirname "$0")" && pwd)/../deploy/agent/fleetdeck-agent.service"
if [[ -f "$UNIT_SRC" ]]; then
  install -m 0644 "$UNIT_SRC" /etc/systemd/system/fleetdeck-agent.service
else
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
fi
# SupplementaryGroups=docker only when the group exists (else systemd fails with status=216/GROUP)
sed -i '/^SupplementaryGroups=docker$/d' /etc/systemd/system/fleetdeck-agent.service
if getent group docker >/dev/null 2>&1; then
  usermod -aG docker fleetdeck || true
  sed -i '/^Group=fleetdeck$/a SupplementaryGroups=docker' /etc/systemd/system/fleetdeck-agent.service
fi

UNINSTALL_SVC="$(cd "$(dirname "$0")" && pwd)/../deploy/agent/fleetdeck-agent-uninstall.service"
UNINSTALL_PATH="$(cd "$(dirname "$0")" && pwd)/../deploy/agent/fleetdeck-agent-uninstall.path"
UPDATE_SVC="$(cd "$(dirname "$0")" && pwd)/../deploy/agent/fleetdeck-agent-update.service"
UPDATE_PATH="$(cd "$(dirname "$0")" && pwd)/../deploy/agent/fleetdeck-agent-update.path"
if [[ -f "$UNINSTALL_SVC" ]]; then
  install -m 0644 "$UNINSTALL_SVC" /etc/systemd/system/fleetdeck-agent-uninstall.service
else
  cat > /etc/systemd/system/fleetdeck-agent-uninstall.service <<'EOF'
[Unit]
Description=FleetDeck agent uninstall (oneshot)
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/uninstall
EOF
fi
if [[ -f "$UNINSTALL_PATH" ]]; then
  install -m 0644 "$UNINSTALL_PATH" /etc/systemd/system/fleetdeck-agent-uninstall.path
else
  cat > /etc/systemd/system/fleetdeck-agent-uninstall.path <<'EOF'
[Unit]
Description=Watch for FleetDeck agent uninstall request

[Path]
PathExists=/var/lib/fleetdeck/UNINSTALL_REQUESTED

[Install]
WantedBy=multi-user.target
EOF
fi
if [[ -f "$UPDATE_SVC" ]]; then
  install -m 0644 "$UPDATE_SVC" /etc/systemd/system/fleetdeck-agent-update.service
else
  cat > /etc/systemd/system/fleetdeck-agent-update.service <<'EOF'
[Unit]
Description=FleetDeck agent update (oneshot)
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/update
EOF
fi
if [[ -f "$UPDATE_PATH" ]]; then
  install -m 0644 "$UPDATE_PATH" /etc/systemd/system/fleetdeck-agent-update.path
else
  cat > /etc/systemd/system/fleetdeck-agent-update.path <<'EOF'
[Unit]
Description=Watch for FleetDeck agent update request

[Path]
PathExists=/var/lib/fleetdeck/UPDATE_REQUESTED

[Install]
WantedBy=multi-user.target
EOF
fi

# One-shot enroll as fleetdeck user
sudo -u fleetdeck "${PREFIX}/bin/fleetdeck-agent" \
  -api "$API_URL" \
  -token "$TOKEN" \
  -state-dir /var/lib/fleetdeck \
  -enroll

chown -R fleetdeck:fleetdeck /var/lib/fleetdeck
chmod 0700 /var/lib/fleetdeck
chmod 0600 /var/lib/fleetdeck/credentials.json 2>/dev/null || true

systemctl daemon-reload
systemctl enable --now fleetdeck-agent-uninstall.path
systemctl enable --now fleetdeck-agent-update.path
systemctl enable --now fleetdeck-agent.service
systemctl --no-pager --full status fleetdeck-agent.service || true

echo
echo "Installed. Check dashboard Servers page for this host (online + metrics)."
echo "Logs: journalctl -u fleetdeck-agent -f"
echo "Update/uninstall from panel, or: sudo fleetdeck-agent -update / -uninstall"
