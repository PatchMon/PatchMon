-- Revert multi-select back to single-select columns.

ALTER TABLE notification_routes ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT '*';
UPDATE notification_routes SET event_type = event_types->>0 WHERE jsonb_array_length(event_types) > 0;
ALTER TABLE notification_routes DROP COLUMN IF EXISTS event_types;

ALTER TABLE notification_routes ADD COLUMN IF NOT EXISTS host_group_id TEXT REFERENCES host_groups(id) ON DELETE CASCADE;
UPDATE notification_routes SET host_group_id = host_group_ids->>0 WHERE jsonb_array_length(host_group_ids) > 0;
ALTER TABLE notification_routes DROP COLUMN IF EXISTS host_group_ids;

ALTER TABLE notification_routes DROP COLUMN IF EXISTS host_ids;

DROP INDEX IF EXISTS idx_notification_routes_enabled;
CREATE INDEX IF NOT EXISTS idx_notification_routes_event ON notification_routes(event_type) WHERE enabled = true;
