# Deployment

## Docker Compose (recommended)

From the repository root:

```bash
cp .env.example .env
# Edit SESSION_SECRET to a long random value before any shared/production use.
docker compose -f deploy/docker-compose.yml up -d --build
```

Services:

| Service | Port | Notes |
|---------|------|-------|
| `web` | 3000 | Dashboard |
| `api` | 8080 | Monitoring API + agent ingest |
| `db` | 5433→5432 | PostgreSQL 16 |

Postgres is published on **5433** on the host to avoid colliding with other local Postgres instances.

## Environment

See `.env.example`. Required:

- `DATABASE_URL`
- `SESSION_SECRET` (≥ 32 chars)
- `WEB_ORIGIN` (dashboard origin for CORS/cookies)
- `API_PUBLIC_URL` (returned to agents on enroll)

Never commit `.env`, credentials, or private keys.

## Upgrades

1. Pull/build new images: `docker compose -f deploy/docker-compose.yml up -d --build`
2. API applies SQL migrations on startup (`schema_migrations`).
3. Restart agents only when the agent binary/protocol requires it — **no auto-update**.

## Backup

Minimum backup targets:

- PostgreSQL volume / logical dump (`pg_dump`)
- `.env` (secrets) stored separately and encrypted

Restore into an empty database, validate, then cut over — never silently overwrite production data.

## TLS (production)

Terminate TLS at a reverse proxy (Caddy, Traefik, nginx) or Cloudflare Tunnel. Point `WEB_ORIGIN` and `API_PUBLIC_URL` at the public HTTPS origins, set `COOKIE_SECURE=true`, and use a strong `SESSION_SECRET`.

When `COOKIE_SECURE=true`, the API also emits `Strict-Transport-Security`.

**Local Compose is HTTP by design** (`deploy/docker-compose.yml` exposes API/web on localhost without TLS). Do not expose Compose ports on the public internet. For shared/LAN use, put Caddy/nginx/Tunnel in front — there is no Compose `https` profile sidecar today (optional later); Tunnel via `--profile tunnel` is the supported remote path.

Example Caddy snippet:

```caddy
fleetdeck.example.com {
  reverse_proxy /api/* localhost:8080
  reverse_proxy /* localhost:3000
}
```

Agents should enroll with `-api https://fleetdeck.example.com` (or the API hostname you expose).

## Agent (remote hosts)

```powershell
$env:CLOUDFLARE_API_TOKEN = "..."
.\scripts\setup-cloudflare-tunnel.ps1 -Hostname agents.example.com   # once
.\scripts\publish-cdn.ps1 -Upload       # build + upload dist/cdn/fleetdeck/ to your CDN
.\scripts\start.ps1 -Build              # day-to-day
```

See [REMOTE_AGENTS.md](REMOTE_AGENTS.md).

## Windows autostart (logon)

Compose services use `restart: unless-stopped`, so containers come back when Docker Desktop starts — but Docker itself must be running after reboot.

1. In **Docker Desktop → Settings → General**, enable **Start Docker Desktop when you log in**.
2. Register a logon Scheduled Task that runs `scripts\start.ps1` (waits for `docker info`, then `compose up`):

```powershell
.\scripts\install-autostart.ps1
```

- Task name: **`FleetDeck-Autostart`** (current user, at logon)
- Remove: `.\scripts\uninstall-autostart.ps1`

Verify without rebooting:

```powershell
Get-ScheduledTask -TaskName 'FleetDeck-Autostart'
Start-ScheduledTask -TaskName 'FleetDeck-Autostart'
docker compose --env-file .env -f deploy/docker-compose.yml ps
```

After a reboot: sign in, wait for Docker Desktop, then check dashboard http://localhost:3000 and (if tunneled) `$API_PUBLIC_URL/healthz`.
