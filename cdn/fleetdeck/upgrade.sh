#!/usr/bin/env bash
# FleetDeck agent upgrade — publish to your CDN prefix (see scripts/publish-cdn.ps1).
# For hosts on older agents (e.g. 0.4.0) that lack panel `agent.update`.
# Keeps enrollment credentials; does NOT require a new token.
#
# One-liner:
#   curl -fsSL https://cdn.example.com/fleetdeck/upgrade.sh | sudo bash
#
# Override CDN: FLEETDECK_CDN, FLEETDECK_CHANNEL
set -euo pipefail

CDN_BASE="${FLEETDECK_CDN:-https://cdn.example.com/fleetdeck}"
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

AGENT_BIN="/usr/local/bin/fleetdeck-agent"
STATE_DIR="/var/lib/fleetdeck"

# Stop the systemd unit (if any), then carefully signal any leftover processes for
# this binary/state-dir (manual sudo starts are not covered by systemctl restart).
stop_and_kill_agent_processes() {
  systemctl stop fleetdeck-agent.service 2>/dev/null || true

  local pids=()
  local pid exe cmdline stated

  if [[ -f "${STATE_DIR}/agent.lock" ]]; then
    pid="$(tr -d ' \n' <"${STATE_DIR}/agent.lock" 2>/dev/null || true)"
    if [[ "${pid}" =~ ^[0-9]+$ ]] && [[ "${pid}" -gt 1 ]] && kill -0 "${pid}" 2>/dev/null; then
      pids+=("${pid}")
    fi
  fi

  for pid in /proc/[0-9]*; do
    pid="${pid#/proc/}"
    [[ "${pid}" =~ ^[0-9]+$ ]] || continue
    [[ "${pid}" -gt 1 ]] || continue

    exe="$(readlink "/proc/${pid}/exe" 2>/dev/null || true)"
    exe="${exe% (deleted)}"
    cmdline="$(tr '\0' ' ' <"/proc/${pid}/cmdline" 2>/dev/null || true)"

    if [[ "${exe}" == "${AGENT_BIN}" ]]; then
      pids+=("${pid}")
      continue
    fi

    case "${cmdline}" in
      "${AGENT_BIN}"*|*/fleetdeck-agent\ *|fleetdeck-agent\ *)
        ;;
      *)
        continue
        ;;
    esac

    stated=""
    if [[ "${cmdline}" =~ -state-dir[= ]([^ ]+) ]]; then
      stated="${BASH_REMATCH[1]}"
    fi
    if [[ -z "${stated}" ]]; then
      stated="${STATE_DIR}"
    fi
    if [[ "${stated}" == "${STATE_DIR}" ]]; then
      pids+=("${pid}")
    fi
  done

  if [[ "${#pids[@]}" -eq 0 ]]; then
    return 0
  fi

  # Unique PIDs
  local -A seen=()
  local uniq=()
  for pid in "${pids[@]}"; do
    [[ -n "${seen[$pid]+x}" ]] && continue
    seen[$pid]=1
    uniq+=("${pid}")
  done

  echo "Stopping leftover fleetdeck-agent process(es): ${uniq[*]}"
  kill -TERM "${uniq[@]}" 2>/dev/null || true
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    local alive=()
    for pid in "${uniq[@]}"; do
      if kill -0 "${pid}" 2>/dev/null; then
        alive+=("${pid}")
      fi
    done
    [[ "${#alive[@]}" -eq 0 ]] && return 0
    uniq=("${alive[@]}")
    sleep 0.5
  done
  echo "Force-killing leftover fleetdeck-agent process(es): ${uniq[*]}"
  kill -KILL "${uniq[@]}" 2>/dev/null || true
  sleep 0.2
}

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
if [[ -x "${AGENT_BIN}" ]]; then
  if command -v strings >/dev/null 2>&1; then
    PREV_VER="$(strings "${AGENT_BIN}" 2>/dev/null | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?' | head -n1 || true)"
  fi
  PREV_VER="${PREV_VER:-unknown}"
fi

echo "Stopping running agent (systemd + any orphans sharing ${STATE_DIR}) …"
stop_and_kill_agent_processes

install -d -m 0755 /usr/local/bin
install -m 0755 "$BIN" "${AGENT_BIN}"

# Ensure update/uninstall helpers exist so future panel updates work.
# Always refresh the update wrapper so panel upgrades exec the staged binary
# (new stop/orphan-kill logic) instead of the pre-replace on-disk binary.
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
# Prefer pending-update.bin so the NEW binary's -update (stop + orphan kill + start) runs
# on the first panel upgrade after this wrapper is installed.
set -euo pipefail
sleep 2
PENDING=/var/lib/fleetdeck/pending-update.bin
if [[ -x "$PENDING" && -s "$PENDING" ]]; then
  exec "$PENDING" -update
fi
exec /usr/local/bin/fleetdeck-agent -update
EOF
chmod 0755 /usr/local/libexec/fleetdeck/update

# Refresh systemd helper units when missing (idempotent overwrite for update path bits).
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

cat > /etc/systemd/system/fleetdeck-agent-update.service <<'EOF'
[Unit]
Description=FleetDeck agent update (oneshot)
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/libexec/fleetdeck/update
EOF

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
  systemctl start fleetdeck-agent.service
  systemctl --no-pager --full status fleetdeck-agent.service || true
else
  echo "fleetdeck-agent.service not found — binary upgraded; start the agent manually." >&2
fi

NEW_VER="unknown"
if command -v strings >/dev/null 2>&1; then
  NEW_VER="$(strings "${AGENT_BIN}" 2>/dev/null | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?' | head -n1 || true)"
fi
NEW_VER="${NEW_VER:-unknown}"
echo
echo "Done. Agent upgraded ${PREV_VER} → ${NEW_VER}."
echo "Credentials at /var/lib/fleetdeck/credentials.json were left in place."
echo "Future panel updates: Dashboard → Servers → Update agent."
echo "Logs: journalctl -u fleetdeck-agent -f"
