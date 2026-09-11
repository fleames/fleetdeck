# Agent

## Role

The FleetDeck agent runs on each monitored host. It:

- Collects system metrics (CPU, memory, disk, network, load, uptime, host identity)
- Collects Docker inventory and container stats via the **local** Docker Engine API
- Authenticates to the Monitoring API with enrollment-issued credentials
- Polls for commands (logs, inspect, start/stop/restart/pause/unpause/**remove**, **uninstall**, **update**)
- **Dials out** — no inbound management port is required on the host

The Docker socket stays on the host and is never exposed to browsers.

---

## Update (panel)

From the dashboard **Servers** list or server detail, admins/operators can **Update agent**:

1. If the agent is online, FleetDeck queues `agent.update` with `cdn_base` + `channel` from `AGENT_CDN_BASE` / `AGENT_CDN_CHANNEL` (configure your CDN base + `latest`).
2. The agent (as `fleetdeck`, under `NoNewPrivileges`) downloads `${cdn}/${channel}/linux-{amd64|arm64}` into `/var/lib/fleetdeck/pending-update.bin`, **requires** a matching entry in `${cdn}/${channel}/SHA256SUMS` (fail closed if missing or mismatch), then reports success.
3. It writes `/var/lib/fleetdeck/UPDATE_REQUESTED`.
4. `fleetdeck-agent-update.path` runs a root oneshot that **prefers exec'ing the staged `pending-update.bin -update`** (so the new binary's apply logic runs). That path **stops** `fleetdeck-agent.service`, kills leftover processes for `/usr/local/bin/fleetdeck-agent` / `/var/lib/fleetdeck` (manual root starts), replaces the binary, then **starts** the unit — **credentials under `/var/lib/fleetdeck` are not touched**.
5. The new process heartbeats with the new `agent_version`.

If the agent is offline, the API returns a clear conflict: wait until online (no offline/force update).

Panel update requires agent **0.4.2-dev+** with the update path unit. Older hosts (e.g. **0.4.0**) should use the manual CDN upgrade (no new enrollment token):

```bash
curl -fsSL https://cdn.example.com/fleetdeck/upgrade.sh | sudo bash
# or: FLEETDECK_CDN=https://cdn.example.com/fleetdeck curl -fsSL "$FLEETDECK_CDN/upgrade.sh" | sudo bash
```

That stops systemd + orphans, replaces the binary, refreshes update/uninstall helpers (including the pending-bin update wrapper), starts the service, and leaves `/var/lib/fleetdeck/credentials.json` alone.

---

## Uninstall (panel or host)

From the dashboard **Servers** list or server detail, admins/operators can **Remove**:

1. If the agent is online, FleetDeck queues `agent.uninstall`.
2. The agent reports success, then writes `/var/lib/fleetdeck/UNINSTALL_REQUESTED`.
3. `fleetdeck-agent-uninstall.path` runs a root oneshot that:
   - `systemctl stop/disable fleetdeck-agent`
   - removes unit files, `/usr/local/bin/fleetdeck-agent`, `/etc/fleetdeck`, `/var/lib/fleetdeck`
   - removes the `fleetdeck` system user when it still uses the standard home
4. FleetDeck then deletes the server row (CASCADE: agent, credentials, inventory, metrics).

If the agent is offline, use **Force remove from panel** (DB only). Manual host cleanup:

```bash
sudo fleetdeck-agent -uninstall
```

Panel remove requires agent **0.4.1-dev+** with the uninstall path unit (re-run the installer if needed).

---

## CDN install (remote hosts)

Run the control plane on a host you manage. Expose the API with a **Cloudflare Tunnel** (or reverse proxy), publish CDN artifacts to **your** prefix, then:

```bash
curl -fsSL https://cdn.example.com/fleetdeck/install.sh | sudo bash -s -- --token 'TOKEN'
```

Shared host / no sudo:

```bash
curl -fsSL https://cdn.example.com/fleetdeck/install.sh | bash -s -- --user --token 'TOKEN'
```

Setup: [REMOTE_AGENTS.md](REMOTE_AGENTS.md) — run `.\scripts\setup-cloudflare-tunnel.ps1 -Hostname agents.example.com` once. The control plane must stay online.

After shipping agent **0.4.2-dev+**, republish CDN so remote hosts get the update helper units:

```powershell
.\scripts\publish-cdn.ps1 -Version 0.4.3-dev -Upload
```

---

## Flags / env

| Flag / env | Purpose |
|------------|---------|
| `-api` / `FLEETDECK_URL` | API base URL |
| `-token` / `FLEETDECK_ENROLLMENT_TOKEN` | One-time enrollment token |
| `-state-dir` / `FLEETDECK_STATE_DIR` | Credential directory |
| `-interval` | Metrics interval (default 10s) |
| `-enroll` | Enroll then exit |
| `-uninstall` | Root Linux uninstall (systemd + files); optional `-delay` |
| `-update` | Root apply of staged `/var/lib/fleetdeck/pending-update.bin` + restart; optional `-delay` |

## Build / release packages

```bash
cd apps/agent
go build -o ../../bin/fleetdeck-agent ./cmd/fleetdeck-agent

# Multi-arch (includes linux/amd64 + linux/arm64)
./scripts/release-agent.ps1 -Version 0.4.3-dev
# or: VERSION=0.4.3-dev ./scripts/release-agent.sh
```

See [RELEASE.md](RELEASE.md). Unit files: `deploy/agent/fleetdeck-agent.service`, `fleetdeck-agent-uninstall.path` / `.service`, `fleetdeck-agent-update.path` / `.service`. Installer: `scripts/install-agent-linux.sh`.

## Security notes

- Treat agent secrets like host root credentials (`chmod 600` credentials file).
- One token enrolls one server binding; do not reuse tokens across hosts.
- Do not mount the Docker socket into the FleetDeck API/web containers.
- Agent version: `0.4.3-dev`.
- Remote update never deletes `credentials.json`.
