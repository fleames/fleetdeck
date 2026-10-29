-- Separate Windows RMM agent registry and RDP sessions (not Docker agent tables).
CREATE TABLE IF NOT EXISTS rmm_hosts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    hostname            TEXT NOT NULL DEFAULT '',
    os_name             TEXT NOT NULL DEFAULT 'windows',
    os_version          TEXT NOT NULL DEFAULT '',
    arch                TEXT NOT NULL DEFAULT 'amd64',
    agent_version       TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'online', 'offline')),
    rdp_enabled         BOOLEAN NOT NULL DEFAULT false,
    rdp_port            INT NOT NULL DEFAULT 3389,
    last_heartbeat_at   TIMESTAMPTZ,
    last_seen_at        TIMESTAMPTZ,
    server_id           UUID REFERENCES servers(id) ON DELETE SET NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rmm_credentials (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id         UUID NOT NULL REFERENCES rmm_hosts(id) ON DELETE CASCADE,
    public_id       TEXT NOT NULL UNIQUE,
    secret_hash     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS rmm_enrollment_tokens (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash      TEXT NOT NULL UNIQUE,
    label           TEXT NOT NULL DEFAULT '',
    name_hint       TEXT NOT NULL DEFAULT '',
    expires_at      TIMESTAMPTZ NOT NULL,
    max_uses        INT NOT NULL DEFAULT 1,
    uses            INT NOT NULL DEFAULT 0,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS rmm_commands (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id         UUID NOT NULL REFERENCES rmm_hosts(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'expired')),
    result_text     TEXT,
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    completed_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rmm_commands_host_pending
    ON rmm_commands (host_id, created_at)
    WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS rmm_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id             UUID NOT NULL REFERENCES rmm_hosts(id) ON DELETE CASCADE,
    mode                TEXT NOT NULL CHECK (mode IN ('browser', 'native')),
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'connecting', 'active', 'closed', 'failed')),
    created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    gateway_listen_port INT,
    tunnel_token_hash   TEXT,
    native_token_hash   TEXT,
    native_token_expires_at TIMESTAMPTZ,
    error               TEXT NOT NULL DEFAULT '',
    started_at          TIMESTAMPTZ,
    ended_at            TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rmm_sessions_host_active
    ON rmm_sessions (host_id)
    WHERE status IN ('pending', 'connecting', 'active');

CREATE TABLE IF NOT EXISTS rmm_session_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES rmm_sessions(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    message     TEXT NOT NULL DEFAULT '',
    context     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
