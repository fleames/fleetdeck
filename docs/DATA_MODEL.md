# Data model

UTC for all stored timestamps. Display in user-configured timezone.

## Entities (relational)

### Identity & access

```text
users
  id, email, display_name, password_hash, role, created_at, updated_at, last_login_at

sessions
  id, user_id, token_hash, expires_at, created_at, ip, user_agent, revoked_at

api_tokens (optional automation)
  id, user_id, name, token_hash, scopes, created_at, last_used_at, revoked_at
```

### Fleet registry

```text
servers
  id, name, hostname, primary_address, os_name, os_version, arch,
  status, health_state, agent_id, docker_available, last_seen_at,
  last_metrics_at, maintenance, created_at, updated_at,
  labels (jsonb), notes

agents
  id, server_id, agent_version, os, arch,
  credential_id, enrolled_at, last_heartbeat_at,
  last_latency_ms, status, resource_cpu_pct, resource_rss_bytes

enrollment_tokens
  id, token_hash, label, expires_at, max_uses, uses, created_by, revoked_at

agent_credentials
  id, agent_id, public_id, secret_hash, created_at, rotated_at, revoked_at
```

### Docker inventory (latest snapshot + history of changes via events)

```text
docker_hosts
  id, server_id, docker_version, api_version, swarm_mode, daemon_healthy, updated_at

containers
  id, server_id, container_id, name, image_ref, image_id,
  state, health, started_at, created_at, restart_count,
  compose_project, compose_service, labels (jsonb), ports (jsonb),
  updated_at, last_seen_at

images
  id, server_id, image_id, repository, tag, size_bytes, created_at, dangling, updated_at

volumes
  id, server_id, name, driver, mountpoint, size_bytes_nullable, unused, updated_at

networks
  id, server_id, network_id, name, driver, scope, updated_at

compose_projects
  id, server_id, project_name, status, config_files (jsonb), updated_at

container_mounts / container_networks  (join tables as needed)
```

### Alerts & observability

```text
alert_rules
  id, name, enabled, severity, scope_type, scope_ids (jsonb),
  metric, operator, threshold, duration_seconds, cooldown_seconds,
  created_at, updated_at

alert_instances
  id, rule_id, severity, status (active|acknowledged|resolved|silenced),
  server_id, container_id, first_seen_at, last_seen_at, resolved_at,
  message, context (jsonb)

infrastructure_events
  id, ts, kind, severity, server_id, container_id, message, context (jsonb)

notifications (derived feed: alerts + notable events; not a stored table)
notification_dismissals
  user_id, notification_id (alert-… / event-…), dismissed_at

audit_logs
  id, ts, user_id, action, target_type, target_id, result, ip, context (jsonb)

settings
  key, value (jsonb), updated_at

dashboard_layouts
  id, user_id, name, layout (jsonb), is_default
```

### Secrets

```text
secrets
  id, kind, name, ciphertext, nonce, key_version, created_at, rotated_at
```

Never store agent secrets or passwords in plaintext columns.

---

## Metrics (time-series)

Prefer **narrow metric tables** with server_id + time as leading keys, partitioned by time.

### High-frequency (default collect 5–15s)

```text
server_metrics_raw (
  ts timestamptz,
  server_id uuid,
  cpu_pct real,
  load1 real, load5 real, load15 real,
  mem_used_bytes bigint, mem_available_bytes bigint, mem_cached_bytes bigint,
  swap_used_bytes bigint,
  disk_used_bytes bigint, disk_total_bytes bigint,  -- or separate filesystem series
  net_rx_bps bigint, net_tx_bps bigint,
  net_rx_errs bigint, net_tx_errs bigint,
  uptime_seconds bigint
)

container_metrics_raw (
  ts, server_id, container_id,
  cpu_pct, mem_used_bytes, mem_limit_bytes,
  net_rx_bps, net_tx_bps,
  blk_read_bps, blk_write_bps,
  pids int
)
```

Optional: `cpu_core_metrics_raw`, `filesystem_metrics_raw`, `sensor_metrics_raw` for temps.

### Aggregates

```text
server_metrics_5m / server_metrics_1h
container_metrics_5m / container_metrics_1h
```

Store avg/max/min/p95 as needed (start with avg + max).

### Default retention (configurable)

| Tier | Retention |
|------|-----------|
| Raw | 7 days |
| 5-minute aggregates | 30 days |
| 1-hour aggregates | 1 year |

Downsample job must be idempotent and observable (self-metrics).

---

## Indexes (minimum)

- `servers(status)`, `servers(last_seen_at)`
- `containers(server_id, state)`, `containers(compose_project)`
- `alert_instances(status, severity, last_seen_at)`
- `infrastructure_events(ts DESC)`, GIN/trigram later for search
- Metrics: BRIN or range partition on `ts`; composite `(server_id, ts DESC)`

---

## Health score (explainable)

Computed server-side from weighted, documented factors — never opaque “AI”:

| Factor | Example penalty |
|--------|-----------------|
| Offline server | heavy |
| Critical alert | heavy |
| Warning alert | medium |
| Unhealthy container | medium |
| Stale metrics | medium |
| Disk > 90% | medium |
| Agent outdated (optional) | light |

API returns `{ score, max, factors: [{ code, impact, detail }] }`.

---

## Freshness model

Every metric payload and list row exposes `last_updated` / `last_seen_at`.

UI classes:

| Class | Meaning |
|-------|---------|
| Fresh | within 2× collection interval |
| Delayed | within offline threshold |
| Stale | beyond delayed; still connected recently |
| Offline | heartbeat missed beyond threshold |

Stale values must never render as if current without freshness labeling.
