-- Lightweight control-plane uptime probes (HTTP/TCP). No agent/tunnel involvement.
CREATE TABLE IF NOT EXISTS uptime_probes (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL,
    enabled             BOOLEAN NOT NULL DEFAULT true,
    kind                TEXT NOT NULL CHECK (kind IN ('http', 'tcp')),
    target              TEXT NOT NULL,
    method              TEXT NOT NULL DEFAULT 'GET',
    expected_status     INT NOT NULL DEFAULT 200,
    interval_seconds    INT NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 30 AND 600),
    timeout_ms          INT NOT NULL DEFAULT 3000 CHECK (timeout_ms BETWEEN 500 AND 10000),
    fail_threshold      INT NOT NULL DEFAULT 3 CHECK (fail_threshold BETWEEN 1 AND 10),
    severity            TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('info', 'warning', 'critical')),
    consecutive_fails   INT NOT NULL DEFAULT 0,
    consecutive_oks     INT NOT NULL DEFAULT 0,
    last_status         TEXT NOT NULL DEFAULT 'unknown',
    last_latency_ms     INT,
    last_error          TEXT NOT NULL DEFAULT '',
    last_checked_at     TIMESTAMPTZ,
    last_changed_at     TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_uptime_probes_enabled ON uptime_probes (enabled) WHERE enabled = true;

ALTER TABLE alert_instances
    ADD COLUMN IF NOT EXISTS probe_id UUID REFERENCES uptime_probes(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_alert_instances_probe ON alert_instances (probe_id) WHERE probe_id IS NOT NULL;
