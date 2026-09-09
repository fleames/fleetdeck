#!/usr/bin/env bash
# FleetDeck agent installer — hosted on CDN (cdn.tarkovbot.com).
#
# Root / VPS (sudo):
#   curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | sudo bash -s -- --token 'TOKEN'
#
# Non-root / seedbox (no sudo):
#   curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | bash -s -- --user --token 'TOKEN'
#   # or omit --user: non-root automatically selects user mode
set -euo pipefail

CDN_BASE="${FLEETDECK_CDN:-https://cdn.tarkovbot.com/fleetdeck}"
CHANNEL="${FLEETDECK_CHANNEL:-latest}"
API_URL="${FLEETDECK_URL:-}"
TOKEN=""
INTERVAL="10s"
USER_MODE=""
FORCE_SYSTEM=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --token|-t) TOKEN="$2"; shift 2 ;;
    --api) API_URL="$2"; shift 2 ;;
    --interval) INTERVAL="$2"; shift 2 ;;
    --cdn) CDN_BASE="$2"; shift 2 ;;
    --channel) CHANNEL="$2"; shift 2 ;;
    --user) USER_MODE=1; shift ;;
    --system) FORCE_SYSTEM=1; USER_MODE=0; shift ;;
    -h|--help)
      cat <<EOF
Usage:
  # Root install (systemd system unit)
  curl -fsSL ${CDN_BASE}/install.sh | sudo bash -s -- --token TOKEN [--api API_URL]

  # User / seedbox install (no sudo)
  curl -fsSL ${CDN_BASE}/install.sh | bash -s -- --user --token TOKEN [--api API_URL]

Non-root shells auto-select --user. User mode installs to:
  binary:  \$HOME/.local/bin/fleetdeck-agent
  state:   \$HOME/.fleetdeck
  env:     \$HOME/.fleetdeck/agent.env
EOF
      exit 0
      ;;
    *) echo "Unknown arg: $1" >&2; exit 1 ;;
  esac
done

if [[ -z "$TOKEN" ]]; then
  echo "Missing --token (create one in FleetDeck → Servers)." >&2
  exit 1
fi

# Resolve install mode: non-root => user; root defaults to system unless --user.
if [[ -z "$USER_MODE" ]]; then
  if [[ "$(id -u)" -ne 0 ]]; then
    USER_MODE=1
  else
    USER_MODE=0
  fi
fi

if [[ "$FORCE_SYSTEM" -eq 1 && "$(id -u)" -ne 0 ]]; then
  echo "System install requires root (sudo). On seedboxes use --user instead." >&2
  exit 1
fi

if [[ "$USER_MODE" -eq 0 && "$(id -u)" -ne 0 ]]; then
  echo "Run as root (sudo), or pass --user for a home-directory install." >&2
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

echo "Checking API ${API_URL}/healthz …"
if ! curl -fsS --max-time 10 "${API_URL}/healthz" >/dev/null; then
  echo "API not reachable. Is FleetDeck running with Cloudflare Tunnel up?" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# User / non-root install (seedboxes, shared hosts)
# ---------------------------------------------------------------------------
install_user() {
  local home_dir="${HOME:-}"
  if [[ -z "$home_dir" ]]; then
    home_dir="$(getent passwd "$(id -un)" 2>/dev/null | cut -d: -f6 || true)"
  fi
  if [[ -z "$home_dir" || ! -d "$home_dir" ]]; then
    echo "Cannot resolve HOME for user install." >&2
    exit 1
  fi

  local bin_dir="${home_dir}/.local/bin"
  local state_dir="${home_dir}/.fleetdeck"
  local agent_bin="${bin_dir}/fleetdeck-agent"
  local env_file="${state_dir}/agent.env"
  local start_script="${state_dir}/start.sh"
  local stop_script="${state_dir}/stop.sh"
  local unit_dir="${home_dir}/.config/systemd/user"
  local use_systemd=0

  echo "User install → binary ${agent_bin}, state ${state_dir}"

  mkdir -p "$bin_dir" "$state_dir"
  chmod 0700 "$state_dir"
  install -m 0755 "$BIN" "$agent_bin"

  cat > "$env_file" <<EOF
FLEETDECK_URL=${API_URL}
FLEETDECK_STATE_DIR=${state_dir}
EOF
  chmod 0600 "$env_file"

  # Docker: optional — probe socket; never fail install
  if [[ -S /var/run/docker.sock ]]; then
    if docker info >/dev/null 2>&1 || [[ -r /var/run/docker.sock ]]; then
      echo "Docker socket looks reachable for $(id -un) (containers may appear in panel)."
    else
      echo "Docker socket present but not accessible — agent will run without Docker metrics."
    fi
  else
    echo "No Docker socket — agent will run without Docker metrics (OK on seedboxes)."
  fi

  echo "Enrolling with ${API_URL} …"
  if ! "$agent_bin" \
    -api "$API_URL" \
    -token "$TOKEN" \
    -state-dir "$state_dir" \
    -enroll; then
    echo "Enrollment failed — is FleetDeck + Cloudflare Tunnel online on your PC?" >&2
    exit 1
  fi
  chmod 0600 "${state_dir}/credentials.json" 2>/dev/null || true

  cat > "$start_script" <<EOF
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${state_dir}"
BIN="${agent_bin}"
# shellcheck disable=SC1090
[[ -f "\${STATE_DIR}/agent.env" ]] && set -a && source "\${STATE_DIR}/agent.env" && set +a
API_URL="\${FLEETDECK_URL:-${API_URL}}"
mkdir -p "\${STATE_DIR}"
cd "\${STATE_DIR}"
nohup "\$BIN" -api "\$API_URL" -state-dir "\${STATE_DIR}" -interval ${INTERVAL} \\
  >>"\${STATE_DIR}/agent.log" 2>&1 &
echo \$! >"\${STATE_DIR}/agent.pid"
echo "fleetdeck-agent started (pid \$(cat "\${STATE_DIR}/agent.pid"))"
EOF
  chmod 0755 "$start_script"

  cat > "$stop_script" <<EOF
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${state_dir}"
if [[ -f "\${STATE_DIR}/agent.pid" ]]; then
  PID="\$(cat "\${STATE_DIR}/agent.pid" 2>/dev/null || true)"
  if [[ -n "\${PID}" ]] && kill -0 "\${PID}" 2>/dev/null; then
    kill "\${PID}" 2>/dev/null || true
    echo "Stopped fleetdeck-agent (pid \${PID})"
  fi
  rm -f "\${STATE_DIR}/agent.pid"
else
  pkill -f "${agent_bin}" 2>/dev/null || true
  echo "Attempted stop via pkill"
fi
EOF
  chmod 0755 "$stop_script"

  if command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
    use_systemd=1
  fi

  if [[ "$use_systemd" -eq 1 ]]; then
    mkdir -p "$unit_dir"
    cat > "${unit_dir}/fleetdeck-agent.service" <<EOF
[Unit]
Description=FleetDeck monitoring agent (user)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=-${env_file}
ExecStart=${agent_bin} -api \${FLEETDECK_URL} -state-dir ${state_dir} -interval ${INTERVAL}
Restart=always
RestartSec=5
WorkingDirectory=${state_dir}

[Install]
WantedBy=default.target
EOF
    systemctl --user daemon-reload
    systemctl --user enable --now fleetdeck-agent.service
    systemctl --user --no-pager --full status fleetdeck-agent.service || true
    echo
    echo "Done. User systemd unit enabled (fleetdeck-agent.service)."
    echo "Logs: journalctl --user -u fleetdeck-agent -f"
    echo "Note: for reboot persistence without login, an admin may need: loginctl enable-linger $(id -un)"
    echo "Panel remote update/uninstall expects a root install; on seedboxes use force-remove + ${stop_script}"
  else
    # Stop any stale process, then start via nohup
    if [[ -x "$stop_script" ]]; then
      "$stop_script" >/dev/null 2>&1 || true
    fi
    "$start_script"
    echo
    echo "Done. No systemd --user — agent started with nohup."
    echo "  Start:  ${start_script}"
    echo "  Stop:   ${stop_script}"
    echo "  Logs:   ${state_dir}/agent.log"
    echo "  Binary: ${agent_bin}"
    echo
    echo "Survive reboot (pick one):"
    echo "  nohup:  ${start_script}"
    echo "  cron:   (crontab -e)  @reboot ${start_script}"
    echo
    echo "Panel remote update/uninstall expects a root install; on seedboxes use force-remove in the panel,"
    echo "then run: ${stop_script} && rm -rf ${state_dir} ${agent_bin}"
  fi

  # Ensure ~/.local/bin is mentioned if not on PATH
  case ":${PATH}:" in
    *":${bin_dir}:"*) ;;
    *) echo "Tip: add ${bin_dir} to PATH (e.g. export PATH=\"${bin_dir}:\$PATH\")." ;;
  esac
}

# ---------------------------------------------------------------------------
# Root / system install
# ---------------------------------------------------------------------------
install_system() {
  if ! id fleetdeck >/dev/null 2>&1; then
    useradd --system --home /var/lib/fleetdeck --shell /usr/sbin/nologin fleetdeck
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
  # SupplementaryGroups=docker only when the group exists (else systemd fails with status=216/GROUP)
  if getent group docker >/dev/null 2>&1; then
    usermod -aG docker fleetdeck || true
    sed -i '/^Group=fleetdeck$/a SupplementaryGroups=docker' /etc/systemd/system/fleetdeck-agent.service
  fi

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
}

if [[ "$USER_MODE" -eq 1 ]]; then
  install_user
else
  install_system
fi
