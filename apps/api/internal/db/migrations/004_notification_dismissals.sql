-- Per-user dismissals for the derived in-app notification center
-- (alerts + notable events). Clearing hides items from the bell without
-- resolving underlying alert_instances or deleting infrastructure_events.
CREATE TABLE notification_dismissals (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_id TEXT NOT NULL,
    dismissed_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, notification_id)
);

CREATE INDEX idx_notification_dismissals_user ON notification_dismissals(user_id);
