-- Rate-limit buckets shared across API processes (and surviving restarts).
-- Used for login/enroll; in-memory limiter remains a fast path fallback if DB is unavailable.
CREATE TABLE IF NOT EXISTS rate_limit_buckets (
    bucket_key  TEXT PRIMARY KEY,
    count       INTEGER NOT NULL DEFAULT 0,
    reset_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS rate_limit_buckets_reset_at_idx ON rate_limit_buckets (reset_at);
