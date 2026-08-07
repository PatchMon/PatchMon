-- Create the built-in "Internal Alerts" destination.
-- This destination creates alert records in the alerts table when notifications are routed to it.
-- It cannot be deleted, only enabled/disabled.

INSERT INTO notification_destinations (id, channel_type, display_name, config_encrypted, enabled, created_at, updated_at)
VALUES ('internal-alerts', 'internal', 'Internal Alerts', '{}', true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;

-- Create default event rules routing host_down, server_update, and agent_update to the internal destination.
INSERT INTO notification_routes (id, destination_id, event_types, min_severity, host_group_ids, host_ids, match_rules, enabled, created_at, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'internal-alerts', '["host_down"]'::jsonb, 'informational', '[]'::jsonb, '[]'::jsonb, NULL, true, NOW(), NOW()),
    (gen_random_uuid()::TEXT, 'internal-alerts', '["server_update"]'::jsonb, 'informational', '[]'::jsonb, '[]'::jsonb, NULL, true, NOW(), NOW()),
    (gen_random_uuid()::TEXT, 'internal-alerts', '["agent_update"]'::jsonb, 'informational', '[]'::jsonb, '[]'::jsonb, NULL, true, NOW(), NOW())
ON CONFLICT (id) DO NOTHING;
