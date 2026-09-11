# API design

Base path: `/api/v1`  
Content type: `application/json`  
Errors: consistent envelope.

## Error envelope

```json
{
  "error": {
    "code": "agent_unreachable",
    "message": "The monitoring agent on NAS-01 did not respond.",
    "details": {
      "server_id": "...",
      "last_successful_connection": "2026-09-08T00:10:00Z"
    },
    "diagnostics": {
      "expandable": true,
      "technical": "context deadline exceeded"
    }
  }
}
```

Human `message` is primary. Technical detail is optional/expandable — never the only text.

## Pagination / filter / sort

```text
?cursor=...&limit=50
?sort=-last_seen_at
?q=postgres
?server_id=...&state=running&health=unhealthy
```

List responses:

```json
{
  "data": [ ... ],
  "meta": { "next_cursor": "...", "total": 87 }
}
```

## Core resources

### Overview

| Method | Path | Notes |
|--------|------|-------|
| GET | `/overview` | Health score, counts, top pressure — dashboard snapshot |
| GET | `/overview/self` | App self-metrics (ingest rate, latency, agents connected) |

### Servers

| Method | Path |
|--------|------|
| GET | `/servers` |
| POST | `/servers` |
| GET | `/servers/:id` |
| PATCH | `/servers/:id` |
| DELETE | `/servers/:id` |
| GET | `/servers/:id/metrics` |
| GET | `/servers/:id/metrics/history` |
| GET | `/servers/:id/docker` |
| GET | `/servers/:id/diagnostics` |
| GET | `/servers/:id/events` |
| POST | `/servers/compare` |

Query params for history: `range=15m|1h|6h|24h|7d|30d`

Response includes `source` (`raw`|`5m`|`1h`), `truncated` (true when lookback was capped to retention), and `since`. Mapping: `15m`/`1h`/`6h` → raw; `24h`/`7d` → `server_metrics_5m`; `30d` → `server_metrics_1h`.

### Agents & enrollment

| Method | Path |
|--------|------|
| GET | `/agents` |
| POST | `/agents/enrollment-tokens` |
| POST | `/agents/enroll` (agent-facing) |
| POST | `/agents/:id/rotate-credentials` |
| GET | `/agents/:id/health` |

### Docker

| Method | Path |
|--------|------|
| GET | `/docker/summary` |
| GET | `/containers` |
| POST | `/containers/clear-stale` | admin/operator; body `{confirm:true, server_id?}` — queue `docker rm` for exited/dead/created |
| GET | `/containers/:id` |
| GET | `/containers/:id/metrics/history` |
| GET | `/containers/:id/logs` |
| GET | `/containers/:id/env` | Sensitive keys masked; `?reveal=1` admin + audit |
| POST | `/containers/:id/actions/:action` | start\|stop\|restart\|pause\|unpause\|remove — body `{confirm:true}`; remove only for exited/dead/created |
| GET | `/images` |
| GET | `/volumes` |
| GET | `/networks` |
| GET | `/compose` |
| GET | `/compose/:id` |

Sensitive env vars in container detail: **masked by default**; `?reveal_env=1` requires elevated role + audit.

### Alerts / events / search

| Method | Path |
|--------|------|
| GET | `/alerts` |
| POST | `/alerts/:id/acknowledge` |
| POST | `/alerts/:id/resolve` |
| POST | `/alerts/:id/silence` |
| GET/POST/PATCH/DELETE | `/alert-rules` |
| GET | `/events` |
| GET | `/search?q=` |

### System

| Method | Path | Notes |
|--------|------|-------|
| GET/PATCH | `/settings` | general / metrics / alerts JSON blobs (`alerts.webhook_url`, `alerts.webhook_format`) |
| GET | `/secrets` | Admin; metadata only (no plaintext) |
| GET | `/secrets/status` | Admin; `{ configured, alert_signing }` |
| PUT | `/secrets/:kind/:name` | Admin; body `{ "value": "..." }` — allowlisted `kind=webhook` |
| DELETE | `/secrets/:kind/:name` | Admin; remove named secret |
| GET | `/audit-logs` | |
| GET | `/notifications` | In-app center (active alerts + notable events; per-user dismissals filtered) |
| POST | `/notifications/clear` | Dismiss all currently visible items for the caller |
| POST | `/notifications/:id/dismiss` | Dismiss one item (`alert-…` / `event-…`) for the caller |
| GET/PUT | `/dashboard/layout` | Per-user widget visibility + order |
| GET | `/export/:kind` | `servers`\|`containers`\|`alerts`\|`events`; `?format=json\|csv` |
| POST | `/backup` | Settings, server registry metadata, alert rules, layouts (no raw metrics) |
| POST | `/restore` | Requires `confirm=true`; merges settings / adds rules; no silent fleet overwrite |
| POST | `/auth/login` `/auth/logout` `/auth/bootstrap` | |
| GET | `/users` (admin) | List local users |
| POST | `/users` (admin) | Create admin/operator/viewer |

## Realtime

One authenticated WebSocket per browser session. Requires a valid `fleetdeck_session` cookie (same as REST). Unauthenticated connections receive `401`. When `COOKIE_SECURE=true`, a browser `Origin` on the `WEB_ORIGIN` allowlist is required (empty Origin rejected).

```text
GET /api/v1/realtime
```

Server → client message types (batched):

```text
server.updated
metrics.batch
container.updated
alert.upserted
event.created
agent.heartbeat
overview.delta
```

Client may subscribe to topics (`servers`, `alerts`, `server:{id}`) to limit fan-out.

## Agent-facing ingest

Separate route group with agent auth (not user session):

| Method | Path |
|--------|------|
| POST | `/agent/v1/heartbeat` |
| POST | `/agent/v1/metrics` |
| POST | `/agent/v1/inventory` |
| GET  | `/agent/v1/commands` | Optional `?wait=25s` long poll (capped at 30s) |
| POST | `/agent/v1/commands/:id/result` |

Payload size limits + schema version field required.

## Status codes

| Code | Use |
|------|-----|
| 200/201 | Success |
| 202 | Accepted async management command |
| 400 | Validation |
| 401 | Unauthenticated |
| 403 | Unauthorized |
| 404 | Missing |
| 409 | Conflict (duplicate enroll, etc.) |
| 429 | Rate limited |
| 503 | Dependency unavailable (DB) — never for single agent down on fleet list |

## Documentation artifact

OpenAPI 3.1 source of truth in `packages/shared/openapi.yaml` (Phase 2+), generating Go types and TS client.
