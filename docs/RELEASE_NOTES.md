# FleetDeck 0.4.0-dev — release notes (pre-1.0)

## 0.4.2-dev

- Panel **Update agent** (`POST /api/v1/servers/{id}/update-agent`) queues `agent.update`
- Agent downloads CDN `linux-{arch}`, **required** SHA256SUMS verify (fail closed), privileged path unit replaces binary + restarts (credentials preserved)
- Installers install `fleetdeck-agent-update.path` / `.service` + `/usr/local/libexec/fleetdeck/update`
- CDN **`upgrade.sh`** for manual 0.4.0 → current upgrades (keeps credentials; installs missing path units)
- Dashboard overview server cards show **Update available** when `agent_version` ≠ panel `current_agent_version`
- Panel Update UI shows copyable manual upgrade one-liner when the agent is too old for `agent.update`
- Republish CDN after release: `.\scripts\publish-cdn.ps1 -Version 0.4.2-dev -Upload`

## What’s included

- Local-first monitoring API (Go), dashboard (Next.js), agent (Go), PostgreSQL
- Real host + Docker metrics via dial-out agents (no fake production data)
- Alerts, events, retention/aggregation workers, realtime WebSocket updates
- Container logs, management actions (confirm + audit), env mask/reveal
- Export CSV/JSON, backup/restore, notifications center, self-metrics
- Theme (dark/light), search (Ctrl+K), topology, server compare
- Admin user management with admin/operator/viewer roles
- Per-user dashboard widget layout customization
- Docker Compose images for API + web; agent runs on monitored hosts

## Deploy

```bash
cp .env.example .env   # set a strong SESSION_SECRET
docker compose -f deploy/docker-compose.yml up -d --build
```

Then open the web UI, bootstrap the first admin, enroll an agent.

## Known gaps (not 1.0 yet)

- Broader HTTP integration / failure-path CI suite
- Free-form dashboard widget grid / topology graph polish
- Dependency CVE scanning in CI
- Signed release binaries and upgrade channel

See `docs/DEFINITION_OF_DONE.md` and `docs/SECURITY_REVIEW.md`.
