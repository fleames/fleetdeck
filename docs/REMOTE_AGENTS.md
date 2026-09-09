# Remote VPS agents (FleetDeck on your local PC)

FleetDeck stays on your **local PC**. Remote VPS agents:

1. Download the binary from **CDN** (`cdn.tarkovbot.com`)
2. Dial your API through a **Cloudflare Tunnel** (`https://agents.tarkovbot.com`)

```text
VPS agent ──HTTPS──► Cloudflare ──tunnel──► FleetDeck API on your PC
```

## Automatic setup (recommended)

Create a Cloudflare API token once (https://dash.cloudflare.com/profile/api-tokens):

| Scope | Permission |
|-------|------------|
| Account → Cloudflare Tunnel | Edit |
| Zone → DNS | Edit |

Then on your PC:

```powershell
$env:CLOUDFLARE_API_TOKEN = "your-api-token"
# optional if you have multiple accounts:
# $env:CLOUDFLARE_ACCOUNT_ID = "..."

.\scripts\setup-cloudflare-tunnel.ps1
```

That script will:

1. Create/reuse tunnel `fleetdeck-home`
2. Point `agents.tarkovbot.com` → `http://api:8080` (Compose service)
3. Create/update proxied DNS CNAME
4. Write `CLOUDFLARE_TUNNEL_TOKEN` + `API_PUBLIC_URL` into `.env`
5. Start FleetDeck with `.\scripts\start.ps1` (tunnel profile on)

Day-to-day start (token already in `.env`):

```powershell
.\scripts\start.ps1 -Build
```

Publish agent binaries to R2 (CDN):

1. Cloudflare Dashboard → **R2** → **Manage R2 API Tokens** → create a token with
   **Object Read & Write** on your public CDN bucket.
2. Put these in `.env` (see `.env.example`):

| Variable | Purpose |
|----------|---------|
| `R2_ACCOUNT_ID` | Cloudflare account ID |
| `R2_ACCESS_KEY_ID` | R2 API token access key |
| `R2_SECRET_ACCESS_KEY` | R2 API token secret |
| `R2_BUCKET` | Public CDN bucket name |
| `R2_PUBLIC_BASE` | Optional; default `https://cdn.tarkovbot.com/fleetdeck` |

3. Build + publish. With the four required `R2_*` vars set, upload is **automatic**:

```powershell
.\scripts\publish-cdn.ps1 -Version 0.4.2-dev
# Skip upload:  .\scripts\publish-cdn.ps1 -NoUpload
# Force upload: .\scripts\publish-cdn.ps1 -Upload
```

`release-agent.ps1` also runs `publish-cdn.ps1` afterward (same auto-upload). Use `-NoCdn` on release to skip.

Objects land under prefix `fleetdeck/` (`install.sh`, `config.json`, `latest/linux-amd64`, …).
The upload script curls `config.json` and `install.sh` on the public base URL afterward.

Verify API tunnel:

```powershell
curl.exe -sS https://agents.tarkovbot.com/healthz
```

## Enroll a VPS

Dashboard → **Servers → Add server** → copy:

```bash
curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | sudo bash -s -- --token 'TOKEN'
```

Your PC must stay online (Compose + cloudflared).

### Seedbox / no sudo

Shared hosts often have no `sudo` (and `su` is locked). Use user mode — installs under `~/.local/bin` + `~/.fleetdeck`, no system user:

```bash
curl -fsSL https://cdn.tarkovbot.com/fleetdeck/install.sh | bash -s -- --user --token 'TOKEN'
```

Non-root shells also auto-select user mode if you omit `--user`. Prefers `systemctl --user` when available; otherwise starts with `nohup` and prints cron `@reboot` instructions.

## Remove / uninstall

Dashboard → **Servers** → **Remove** (or server detail → Remove):

- **Online agent:** queues `agent.uninstall`, waits for the VPS to tear down systemd + files, then deletes DB rows.
- **Offline agent:** use **Force remove from panel** (DB only); clean the host with `sudo fleetdeck-agent -uninstall`.

## Update agent

Dashboard → **Servers** → **Update agent** (or server detail):

- Requires an **online** agent with **0.4.2-dev+** and `fleetdeck-agent-update.path` (included in current `install.sh` / `upgrade.sh`).
- Pulls `${CDN}/${channel}/linux-{arch}`, **requires** `SHA256SUMS` verify (fail closed), replaces binary, restarts — credentials stay.
- After releasing a new agent binary, republish CDN (uploads automatically when R2 is configured):

```powershell
.\scripts\publish-cdn.ps1 -Version 0.4.2-dev
```

Hosts on **0.4.0** (no `agent.update`): use the manual CDN upgrade (keeps credentials, installs update path units):

```bash
curl -fsSL https://cdn.tarkovbot.com/fleetdeck/upgrade.sh | sudo bash
```

The panel shows this one-liner when Update agent detects an unsupported agent version.

## Troubleshooting

| Symptom | Cause |
|---------|--------|
| Setup script API error | Token missing Tunnel Edit or DNS Edit; wrong account |
| `healthz` fails | `docker compose … --profile tunnel logs cloudflared` — bad token or API not up |
| Enrollment failed | Tunnel/API offline, or CDN `config.json` wrong `api_url` |
| CDN curl fails | Run `.\scripts\publish-cdn.ps1` (auto-upload if R2_* set); check bucket public access |

## Security

- Dashboard stays on `localhost:3000`; tunnel exposes the **API** only.
- Treat `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_TUNNEL_TOKEN`, and `R2_SECRET_ACCESS_KEY` like passwords (`.env` is gitignored).
- Enrollment tokens are one-time secrets.

## Legacy

Manual Zero Trust UI tunnel creation and `deploy/docker-compose.relay.yml` are optional; prefer `setup-cloudflare-tunnel.ps1`.
