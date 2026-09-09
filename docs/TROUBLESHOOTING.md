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
