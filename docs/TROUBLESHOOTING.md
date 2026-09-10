# Troubleshooting

## Dashboard shows “API unavailable”

- Confirm API: `curl http://localhost:8080/healthz`
- Confirm Postgres is healthy (`deploy-db-1`) and `DATABASE_URL` points at the right host/port (dev default **5433**).
- Confirm `WEB_ORIGIN` matches the browser origin (cookies/CORS).

## Server stays Pending

- Enrollment token expired or already used (`max_uses`).
- Agent cannot reach `API_PUBLIC_URL` / `-api`.
- Check agent stderr for enroll/metrics errors.

## Metrics missing / “No metrics yet”

- Agent must complete inventory + metrics loops (Docker stats used to block; agent ≥ 0.2 uses short Docker timeouts).
- Confirm `last_metrics_at` on `GET /api/v1/servers`.
- Stale values are labeled Delayed/Stale/Offline — they are never presented as current without freshness text.

## Agent RAM grows to multi-GB / host OOM

- Agents **before 0.4.3-dev** created a new Docker `http.Transport` on every Engine API call (stats/inventory), which leaks idle connections and can push RES into gigabytes while CPU stays idle.
- Upgrade the agent (≥ **0.4.3-dev**): shared Docker HTTP client, capped response reads, heartbeat uses `/_ping` instead of full inventory every tick.
- Interim: `sudo systemctl restart fleetdeck-agent` frees memory until it grows again; prefer upgrading.

## Two `fleetdeck-agent` processes (root + fleetdeck)

- Systemd unit must run as `User=fleetdeck` (`/etc/systemd/system/fleetdeck-agent.service`). A second process with the same `-state-dir /var/lib/fleetdeck` as **root** is almost always a manual/`sudo` start or leftover — not the update oneshot (that exits).
- **Upgrade / panel Update agent** (current CDN `upgrade.sh` and agent ≥ **0.4.3-dev** apply path) stops the unit, kills orphans sharing the managed binary/state-dir, then starts a single systemd instance.
- Interim cleanup if duplicates are already running:
  ```bash
  pgrep -a fleetdeck-agent
  curl -fsSL https://cdn.tarkovbot.com/fleetdeck/upgrade.sh | sudo bash
  # or: sudo kill <root-pid> && sudo systemctl restart fleetdeck-agent
  ```
- Agents ≥ **0.4.3-dev** take an exclusive lock on `$state-dir/agent.lock`; a second instance exits immediately with an error instead of double-reporting and double RAM use. Upgrades must clear orphans first so the new unit can take the lock.

## Container logs timeout

- Agent must be running and polling `/agent/v1/commands` (every ~2s).
- Docker Engine must be reachable from the agent (named pipe on Windows, unix socket on Linux).
- API waits ~25s for the agent result.

## Alerts not firing

- Default rules are seeded on API start when `alert_rules` is empty.
- Evaluation runs about every 20s against **online** servers with fresh metrics.
- Check **Alerts** page rules table and **Events** for `alert.fired`.

## Offline servers

- Heartbeat older than `AGENT_OFFLINE_AFTER_SECONDS` (default 45) marks the server offline without blocking the rest of the UI.

## Metrics gaps after API outage

- Agents ≥ this build spool failed metric POSTs to `metrics-buffer.jsonl` under the state dir (size/age capped). Heartbeat may show `buffered_samples` / `oldest_buffer_age_sec` on the agent row.
- When the API returns, the agent flushes oldest-first. Gaps still appear as chart nulls (no invented points).

## Retention deleted too much / history short

- Confirm Settings retention days and `METRICS_*_RETENTION_DAYS` env defaults.
- Worker DROPs fully-aged monthly partitions and DELETEs DEFAULT/agg leftovers hourly; Settings restore does not undelete metrics — restore from Postgres backup.
- See [MONITORING.md](./MONITORING.md) and [DATABASE.md](./DATABASE.md).

## Login rate limit

- Login/enroll counters live in Postgres (`rate_limit_buckets`) so they survive API restarts and apply across replicas. If the DB write fails, the process falls back to an in-memory limiter. Behind Cloudflare Tunnel, client IP uses `CF-Connecting-IP` when present.