# Remote agents (tunnel + CDN)

Run the FleetDeck **control plane** on a host you manage (home lab, VPS, or workstation). Remote agents:

1. Download the binary from **your CDN** (object storage / R2 / any HTTPS prefix)
2. Dial your API through a **tunnel or reverse proxy** (e.g. Cloudflare Tunnel → `https://agents.example.com`)

```text
Remote agent ──HTTPS──► Tunnel / proxy ──► FleetDeck API (control plane)
```

Bring your own domains. Scripts under `scripts/` and `cdn/fleetdeck/` may include **optional maintainer CDN defaults** for the project's demo hosting — always set `API_PUBLIC_URL`, `AGENT_CDN_BASE`, and/or `FLEETDECK_CDN` to **your** endpoints for production.

## Automatic tunnel setup (recommended)

Create a Cloudflare API token once (https://dash.cloudflare.com/profile/api-tokens):

| Scope | Permission |
|-------|------------|
| Account → Cloudflare Tunnel | Edit |
| Zone → DNS | Edit |

Then on the control-plane host (Windows helper):

```powershell
$env:CLOUDFLARE_API_TOKEN = "your-api-token"
# optional if you have multiple accounts:
# $env:CLOUDFLARE_ACCOUNT_ID = "..."

.\scripts\setup-cloudflare-tunnel.ps1 -Hostname agents.example.com
```

That script will:

1. Create/reuse tunnel `fleetdeck-home`
2. Point your hostname → `http://api:8080` (Compose service)
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
| `R2_PUBLIC_BASE` | Optional; defaults to `AGENT_CDN_BASE` |
| `AGENT_CDN_BASE` | Public HTTPS prefix (e.g. `https://cdn.example.com/fleetdeck`) |

3. Build + publish. With the four required `R2_*` vars set, upload is **automatic**:

```powershell
.\scripts\publish-cdn.ps1 -Version 0.4.3-dev
# Skip upload:  .\scripts\publish-cdn.ps1 -NoUpload
# Force upload: .\scripts\publish-cdn.ps1 -Upload
```

`release-agent.ps1` also runs `publish-cdn.ps1` afterward (same auto-upload). Use `-NoCdn` on release to skip.

Objects land under your CDN prefix (`install.sh`, `config.json`, `latest/linux-amd64`, …).
The upload script curls `config.json` and `install.sh` on the public base URL afterward.

Verify API tunnel:

```bash
curl -fsS https://agents.example.com/healthz
```

## Enroll a remote host

Dashboard → **Servers → Add server** → copy the enrollment command (it uses your configured CDN base), for example:

```bash
curl -fsSL https://cdn.example.com/fleetdeck/install.sh | sudo bash -s -- --token 'TOKEN'
```

The control plane must stay online (Compose + cloudflared or your proxy).

### Shared host / no sudo

Use user mode — installs under `~/.local/bin` + `~/.fleetdeck`, no system user:

```bash
curl -fsSL https://cdn.example.com/fleetdeck/install.sh | bash -s -- --user --token 'TOKEN'
```

Non-root shells also auto-select user mode if you omit `--user`. Prefers `systemctl --user` when available; otherwise starts with `nohup` and prints cron `@reboot` instructions.

## Remove / uninstall

Dashboard → **Servers** → **Remove** (or server detail → Remove):

- **Online:** queues `agent.uninstall`, then deletes DB rows
- **Offline:** Force remove from panel (DB only) + host cleanup:

```bash
sudo fleetdeck-agent -uninstall
```

## Manual CDN upgrade (older agents)

```bash
curl -fsSL https://cdn.example.com/fleetdeck/upgrade.sh | sudo bash
```

Override CDN at runtime: `FLEETDECK_CDN=https://cdn.example.com/fleetdeck`.

## Troubleshooting

| Symptom | Check |
|---------|--------|
| Setup script API error | Token missing Tunnel Edit or DNS Edit; wrong account |
| `healthz` fails | `docker compose … --profile tunnel logs cloudflared` — bad token or API not up |
| Install fails mid-download | `AGENT_CDN_BASE` / `FLEETDECK_CDN` reachable; `SHA256SUMS` present for updates |
| Agent pending forever | `API_PUBLIC_URL` matches what agents dial; enrollment token unused |

Security:

- Treat `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_TUNNEL_TOKEN`, and `R2_SECRET_ACCESS_KEY` like passwords (`.env` is gitignored).
- Enrollment tokens are one-time secrets.

See also: [AGENT.md](AGENT.md), [DEPLOYMENT.md](DEPLOYMENT.md), [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
