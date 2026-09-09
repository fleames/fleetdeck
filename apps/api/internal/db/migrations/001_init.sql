-- +migrate Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL DEFAULT '',
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'admin' CHECK (role IN ('admin', 'operator', 'viewer')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at   TIMESTAMPTZ
);

CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash      TEXT NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip              TEXT NOT NULL DEFAULT '',
    user_agent      TEXT NOT NULL DEFAULT '',
    revoked_at      TIMESTAMPTZ
);

CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE servers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    hostname            TEXT NOT NULL DEFAULT '',
    primary_address     TEXT NOT NULL DEFAULT '',
    os_name             TEXT NOT NULL DEFAULT '',
    os_version          TEXT NOT NULL DEFAULT '',
    arch                TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'online', 'offline', 'maintenance')),
    health_state        TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (health_state IN ('healthy', 'warning', 'critical', 'offline', 'maintenance', 'unknown')),
    docker_available    BOOLEAN NOT NULL DEFAULT false,
    last_seen_at        TIMESTAMPTZ,
    last_metrics_at     TIMESTAMPTZ,
    maintenance         BOOLEAN NOT NULL DEFAULT false,
    labels              JSONB NOT NULL DEFAULT '{}'::jsonb,
    notes               TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_servers_status ON servers(status);
CREATE INDEX idx_servers_last_seen ON servers(last_seen_at);

CREATE TABLE agents (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id               UUID NOT NULL UNIQUE REFERENCES servers(id) ON DELETE CASCADE,
    agent_version           TEXT NOT NULL DEFAULT '',
    os                      TEXT NOT NULL DEFAULT '',
    arch                    TEXT NOT NULL DEFAULT '',
    status                  TEXT NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'online', 'offline', 'unhealthy')),
    enrolled_at             TIMESTAMPTZ,
    last_heartbeat_at       TIMESTAMPTZ,
    last_latency_ms         INTEGER,
    resource_cpu_pct        REAL,
    resource_rss_bytes      BIGINT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE enrollment_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash      TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL DEFAULT '',
    expires_at      TIMESTAMPTZ NOT NULL,
    max_uses        INTEGER NOT NULL DEFAULT 1,
    uses            INTEGER NOT NULL DEFAULT 0,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agent_credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    public_id       TEXT NOT NULL UNIQUE,
    secret_hash     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_at      TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ
);

CREATE INDEX idx_agent_credentials_agent ON agent_credentials(agent_id);

CREATE TABLE docker_hosts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL UNIQUE REFERENCES servers(id) ON DELETE CASCADE,
    docker_version  TEXT NOT NULL DEFAULT '',
    api_version     TEXT NOT NULL DEFAULT '',
    swarm_mode      BOOLEAN NOT NULL DEFAULT false,
    daemon_healthy  BOOLEAN NOT NULL DEFAULT false,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE containers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id           UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    container_id        TEXT NOT NULL,
    name                TEXT NOT NULL DEFAULT '',
    image_ref           TEXT NOT NULL DEFAULT '',
    image_id            TEXT NOT NULL DEFAULT '',
    state               TEXT NOT NULL DEFAULT 'unknown',
    health              TEXT NOT NULL DEFAULT 'none',
    started_at          TIMESTAMPTZ,
    container_created_at TIMESTAMPTZ,
    restart_count       INTEGER NOT NULL DEFAULT 0,
    compose_project     TEXT NOT NULL DEFAULT '',
    compose_service     TEXT NOT NULL DEFAULT '',
    labels              JSONB NOT NULL DEFAULT '{}'::jsonb,
    ports               JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, container_id)
);

CREATE INDEX idx_containers_server_state ON containers(server_id, state);
CREATE INDEX idx_containers_compose ON containers(compose_project);

CREATE TABLE images (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    image_id        TEXT NOT NULL,
    repository      TEXT NOT NULL DEFAULT '',
    tag             TEXT NOT NULL DEFAULT '',
    size_bytes      BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ,
    dangling        BOOLEAN NOT NULL DEFAULT false,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, image_id)
);

CREATE TABLE volumes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    driver          TEXT NOT NULL DEFAULT '',
    mountpoint      TEXT NOT NULL DEFAULT '',
    size_bytes      BIGINT,
    unused          BOOLEAN NOT NULL DEFAULT false,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, name)
);

CREATE TABLE networks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    network_id      TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    driver          TEXT NOT NULL DEFAULT '',
    scope           TEXT NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, network_id)
);

CREATE TABLE compose_projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    project_name    TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown',
    config_files    JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (server_id, project_name)
);

CREATE TABLE alert_rules (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    severity            TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    scope_type          TEXT NOT NULL DEFAULT 'all',
    scope_ids           JSONB NOT NULL DEFAULT '[]'::jsonb,
    metric              TEXT NOT NULL,
    operator            TEXT NOT NULL,
    threshold           DOUBLE PRECISION NOT NULL,
    duration_seconds    INTEGER NOT NULL DEFAULT 60,
    cooldown_seconds    INTEGER NOT NULL DEFAULT 300,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_instances (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         UUID REFERENCES alert_rules(id) ON DELETE SET NULL,
    severity        TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'acknowledged', 'resolved', 'silenced')),
    server_id       UUID REFERENCES servers(id) ON DELETE SET NULL,
    container_id    UUID REFERENCES containers(id) ON DELETE SET NULL,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at     TIMESTAMPTZ,
    message         TEXT NOT NULL,
    context         JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_alert_instances_status ON alert_instances(status, severity, last_seen_at DESC);

CREATE TABLE infrastructure_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts              TIMESTAMPTZ NOT NULL DEFAULT now(),
    kind            TEXT NOT NULL,
    severity        TEXT NOT NULL DEFAULT 'info',
    server_id       UUID REFERENCES servers(id) ON DELETE SET NULL,
    container_id    UUID REFERENCES containers(id) ON DELETE SET NULL,
    message         TEXT NOT NULL,
    context         JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_events_ts ON infrastructure_events(ts DESC);

CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ts              TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    action          TEXT NOT NULL,
    target_type     TEXT NOT NULL DEFAULT '',
    target_id       TEXT NOT NULL DEFAULT '',
    result          TEXT NOT NULL DEFAULT 'ok',
    ip              TEXT NOT NULL DEFAULT '',
    context         JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_audit_ts ON audit_logs(ts DESC);

CREATE TABLE settings (
    key             TEXT PRIMARY KEY,
    value           JSONB NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dashboard_layouts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL DEFAULT 'Default',
    layout          JSONB NOT NULL DEFAULT '[]'::jsonb,
    is_default      BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (user_id, name)
);

CREATE TABLE secrets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            TEXT NOT NULL,
    name            TEXT NOT NULL,
    ciphertext      BYTEA NOT NULL,
    nonce           BYTEA NOT NULL,
    key_version     INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_at      TIMESTAMPTZ,
    UNIQUE (kind, name)
);

-- Raw metrics (partitioned by month; initial partition + default)
CREATE TABLE server_metrics_raw (
    ts                      TIMESTAMPTZ NOT NULL,
    server_id               UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    cpu_pct                 REAL,
    load1                   REAL,
    load5                   REAL,
    load15                  REAL,
    mem_used_bytes          BIGINT,
    mem_available_bytes     BIGINT,
    mem_cached_bytes        BIGINT,
    swap_used_bytes         BIGINT,
    disk_used_bytes         BIGINT,
    disk_total_bytes        BIGINT,
    net_rx_bps              BIGINT,
    net_tx_bps              BIGINT,
    net_rx_errs             BIGINT,
    net_tx_errs             BIGINT,
    uptime_seconds          BIGINT,
    PRIMARY KEY (server_id, ts)
) PARTITION BY RANGE (ts);

CREATE TABLE server_metrics_raw_default PARTITION OF server_metrics_raw DEFAULT;

CREATE TABLE container_metrics_raw (
    ts                      TIMESTAMPTZ NOT NULL,
    server_id               UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    container_id            UUID NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    cpu_pct                 REAL,
    mem_used_bytes          BIGINT,
    mem_limit_bytes         BIGINT,
    net_rx_bps              BIGINT,
    net_tx_bps              BIGINT,
    blk_read_bps            BIGINT,
    blk_write_bps           BIGINT,
    pids                    INTEGER,
    PRIMARY KEY (container_id, ts)
) PARTITION BY RANGE (ts);

CREATE TABLE container_metrics_raw_default PARTITION OF container_metrics_raw DEFAULT;

CREATE TABLE server_metrics_5m (
    bucket                  TIMESTAMPTZ NOT NULL,
    server_id               UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    cpu_pct_avg             REAL,
    cpu_pct_max             REAL,
    mem_used_bytes_avg      BIGINT,
    mem_used_bytes_max      BIGINT,
    disk_used_bytes_avg     BIGINT,
    net_rx_bps_avg          BIGINT,
    net_tx_bps_avg          BIGINT,
    samples                 INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (server_id, bucket)
);

CREATE TABLE server_metrics_1h (
    bucket                  TIMESTAMPTZ NOT NULL,
    server_id               UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    cpu_pct_avg             REAL,
    cpu_pct_max             REAL,
    mem_used_bytes_avg      BIGINT,
    mem_used_bytes_max      BIGINT,
    disk_used_bytes_avg     BIGINT,
    net_rx_bps_avg          BIGINT,
    net_tx_bps_avg          BIGINT,
    samples                 INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (server_id, bucket)
);

INSERT INTO settings (key, value) VALUES
    ('general', '{"app_name":"FleetDeck","theme":"dark","timezone":"UTC"}'::jsonb),
    ('metrics', '{"interval_seconds":10,"raw_retention_days":7,"agg_5m_retention_days":30,"agg_1h_retention_days":365}'::jsonb),
    ('alerts', '{"defaults_enabled":true}'::jsonb);
