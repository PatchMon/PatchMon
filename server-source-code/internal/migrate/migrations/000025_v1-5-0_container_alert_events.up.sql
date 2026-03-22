-- Seed alert_config rows for Docker container status change events.
INSERT INTO alert_config (id, alert_type, is_enabled, default_severity, auto_assign_enabled, notification_enabled, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'container_stopped', true, 'warning', false, true, NOW()),
    (gen_random_uuid()::TEXT, 'container_started', true, 'informational', false, true, NOW())
ON CONFLICT (alert_type) DO NOTHING;
