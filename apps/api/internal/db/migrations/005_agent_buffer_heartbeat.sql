-- Agent heartbeat fields for durable metrics buffer visibility
ALTER TABLE agents
    ADD COLUMN IF NOT EXISTS buffered_samples INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS oldest_buffer_age_sec INTEGER NOT NULL DEFAULT 0;
