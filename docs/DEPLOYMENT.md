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

## Agent (remote VPS)

```powershell
$env:CLOUDFLARE_API_TOKEN = "..."
.\scripts\setup-cloudflare-tunnel.ps1   # once
.\scripts\publish-cdn.ps1 -Upload       # build + upload dist/cdn/fleetdeck/ to R2
.\scripts\start.ps1 -Build              # day-to-day
```

See [REMOTE_AGENTS.md](REMOTE_AGENTS.md).
