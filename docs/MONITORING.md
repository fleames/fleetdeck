# Monitoring

How FleetDeck collects, stores, and surfaces real host/Docker metrics.

## Pipeline

1. Agent samples host (gopsutil) + container stats (Docker Engine API) on an interval (default 10s).
2. Agent POSTs JSON to `/agent/v1/metrics` (HTTPS bearer). On failure it **spools to disk** (`metrics-buffer.jsonl` under the state dir, size/age capped) and reports `buffered_samples` / `oldest_buffer_age_sec` on heartbeat.
3. API authenticates the agent, applies **in-process backpressure** (max concurrent ingest txs), and **batch-inserts** host/container samples in one transaction.
4. Worker (hourly): DELETE past retention for raw / 5m / 1h; downsample raw→5m (last 2h) and 5m→1h (last 48h).
5. History API picks source by range: short → raw; medium → 5m; long → 1h (see `docs/API.md`). UI shows freshness (LIVE / RECENT / STALE / OFFLINE) — never fabricated samples.

## Retention

| Tier | Default (env) | Settings key |
|------|---------------|--------------|
| Raw | `METRICS_RAW_RETENTION_DAYS` (7) | `metrics.raw_retention_days` |
| 5m | `METRICS_5M_RETENTION_DAYS` (30) | `metrics.agg_5m_retention_days` |
| 1h | `METRICS_1H_RETENTION_DAYS` (365) | `metrics.agg_1h_retention_days` |

Settings values override env when ≥ 1. Implementation today: **row DELETE** on the DEFAULT partition (not `DROP PARTITION`). Monthly partition lifecycle is not automated yet.

**Restore:** Settings JSON alone does not bring metrics back. Use Postgres volume backup / `pg_dump`.

## What is not monitoring theater

- No demo/mock metric generators in production UI paths.
- Empty / loading / error states are explicit.
- Offline agents: last-known + freshness labels; samples are not invented.
