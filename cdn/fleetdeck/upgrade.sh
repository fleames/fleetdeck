#!/usr/bin/env bash
# FleetDeck agent upgrade — hosted on CDN (cdn.tarkovbot.com).
# For hosts on older agents (e.g. 0.4.0) that lack panel `agent.update`.
# Keeps enrollment credentials; does NOT require a new token.
#
# One-liner:
#   curl -fsSL https://cdn.tarkovbot.com/fleetdeck/upgrade.sh | sudo bash
set -euo pipefail

CDN_BASE="${FLEETDECK_CDN:-https://cdn.tarkovbot.com/fleetdeck}"
CHANNEL="${FLEETDECK_CHANNEL:-latest}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cdn) CDN_BASE="$2"; shift 2 ;;
    --channel) CHANNEL="$2"; shift 2 ;;
    -h|--help)
      echo "Usage: curl -fsSL ${CDN_BASE}/upgrade.sh | sudo bash [-- --cdn URL] [-- --channel CHANNEL]"
      echo "Upgrades /usr/local/bin/fleetdeck-agent from CDN; preserves /var/lib/fleetdeck/credentials.json."
      exit 0
      ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root (sudo)." >&2
  exit 1
fi

if [[ ! -f /var/lib/fleetdeck/credentials.json ]]; then
  echo "No /var/lib/fleetdeck/credentials.json — this host is not enrolled." >&2
  echo "Use install.sh with an enrollment token instead:" >&2
  echo "  curl -fsSL ${CDN_BASE}/install.sh | sudo bash -s -- --token 'TOKEN'" >&2
  exit 1
fi

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
SUMS_URL="${CDN_BASE}/${CHANNEL}/SHA256SUMS"

echo "Downloading agent from ${URL} …"
curl -fsSL "$URL" -o "$BIN"
chmod +x "$BIN"

if SUMS="$(curl -fsSL "$SUMS_URL" 2>/dev/null || true)"; then
  if [[ -n "$SUMS" ]]; then
    EXPECTED="$(printf '%s\n' "$SUMS" | awk -v f="linux-${ARCH_LABEL}" '$2 == f { print $1; exit }')"
    if [[ -n "$EXPECTED" ]]; then
      if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL="$(sha256sum "$BIN" | awk '{ print $1 }')"
      elif command -v shasum >/dev/null 2>&1; then
        ACTUAL="$(shasum -a 256 "$BIN" | awk '{ print $1 }')"
      else
        ACTUAL=""
      fi
      if [[ -n "$ACTUAL" && "$ACTUAL" != "$EXPECTED" ]]; then
        echo "SHA256 mismatch for linux-${ARCH_LABEL} (got ${ACTUAL}, expected ${EXPECTED})." >&2
        exit 1
      fi
      if [[ -n "$ACTUAL" ]]; then
        echo "SHA256 verified."
      fi
    fi
  fi
fi

PREV_VER="unknown"
if [[ -x /usr/local/bin/fleetdeck-agent ]]; then
  if command -v strings >/dev/null 2>&1; then
    PREV_VER="$(strings /usr/local/bin/fleetdeck-agent 2>/dev/null | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?' | head -n1 || true)"
  fi
  PREV_VER="${PREV_VER:-unknown}"
fi

install -d -m 0755 /usr/local/bin
install -m 0755 "$BIN" /usr/local/bin/fleetdeck-agent

# Ensure update/uninstall helpers exist so future panel updates work.
install -d -m 0755 /usr/local/libexec/fleetdeck
if [[ ! -x /usr/local/libexec/fleetdeck/uninstall ]]; then
  cat > /usr/local/libexec/fleetdeck/uninstall <<'EOF'
#!/bin/bash
# Triggered by fleetdeck-agent-uninstall.path; delay so agent can report command result.
set -euo pipefail
sleep 2
exec /usr/local/bin/fleetdeck-agent -uninstall
EOF
  chmod 0755 /usr/local/libexec/fleetdeck/uninstall
fi

if [[ ! -x /usr/local/libexec/fleetdeck/update ]]; then
  cat > /usr/local/libexec/fleetdeck/update <<'EOF'
#!/bin/bash
# Triggered by fleetdeck-agent-update.path; delay so agent can report command result.
# Applies staged binary from /var/lib/fleetdeck/pending-update.bin (credentials untouched).
set -euo pipefail
sleep 2
exec /usr/local/bin/fleetdeck-agent -update
EOF
  chmod 0755 /usr/local/libexec/fleetdeck/update
fi

if [[ ! -f /etc/systemd/system/fleetdeck-agent-uninstall.service ]]; then
  cat > /etc/systemd/system/fleetdeck-agent-uninstall.service <<'EOF'
[Unit]
Description=FleetDeck agent uninstall (oneshot)
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/uninstall
EOF
fi

if [[ ! -f /etc/systemd/system/fleetdeck-agent-uninstall.path ]]; then
  cat > /etc/systemd/system/fleetdeck-agent-uninstall.path <<'EOF'
[Unit]
Description=Watch for FleetDeck agent uninstall request

[Path]
PathExists=/var/lib/fleetdeck/UNINSTALL_REQUESTED

[Install]
WantedBy=multi-user.target
EOF
fi

if [[ ! -f /etc/systemd/system/fleetdeck-agent-update.service ]]; then
  cat > /etc/systemd/system/fleetdeck-agent-update.service <<'EOF'
[Unit]
Description=FleetDeck agent update (oneshot)
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/update
EOF
fi

if [[ ! -f /etc/systemd/system/fleetdeck-agent-update.path ]]; then
  cat > /etc/systemd/system/fleetdeck-agent-update.path <<'EOF'
[Unit]
Description=Watch for FleetDeck agent update request

[Path]
PathExists=/var/lib/fleetdeck/UPDATE_REQUESTED

[Install]
WantedBy=multi-user.target
EOF
fi

# Preserve credentials ownership
if id fleetdeck >/dev/null 2>&1; then
  chown -R fleetdeck:fleetdeck /var/lib/fleetdeck 2>/dev/null || true
  chmod 0600 /var/lib/fleetdeck/credentials.json 2>/dev/null || true
fi

systemctl daemon-reload
systemctl enable --now fleetdeck-agent-uninstall.path 2>/dev/null || true
systemctl enable --now fleetdeck-agent-update.path 2>/dev/null || true

if systemctl list-unit-files fleetdeck-agent.service >/dev/null 2>&1; then
  systemctl restart fleetdeck-agent.service
  systemctl --no-pager --full status fleetdeck-agent.service || true
else
  echo "fleetdeck-agent.service not found — binary upgraded; start the agent manually." >&2
fi

NEW_VER="unknown"
if command -v strings >/dev/null 2>&1; then
  NEW_VER="$(strings /usr/local/bin/fleetdeck-agent 2>/dev/null | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?' | head -n1 || true)"
fi
NEW_VER="${NEW_VER:-unknown}"
echo
echo "Done. Agent upgraded ${PREV_VER} → ${NEW_VER}."
echo "Credentials at /var/lib/fleetdeck/credentials.json were left in place."
echo "Future panel updates: Dashboard → Servers → Update agent."
echo "Logs: journalctl -u fleetdeck-agent -f"
