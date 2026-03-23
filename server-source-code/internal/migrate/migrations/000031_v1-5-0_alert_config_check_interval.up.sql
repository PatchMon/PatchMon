-- Add configurable check interval for periodic/cron-based alert monitors.
-- NULL means "not applicable" (event-driven alerts).
ALTER TABLE alert_config ADD COLUMN IF NOT EXISTS check_interval_minutes INTEGER;

-- Set defaults for periodic alert types.
UPDATE alert_config SET check_interval_minutes = 5  WHERE alert_type = 'host_down';
UPDATE alert_config SET check_interval_minutes = 30 WHERE alert_type = 'host_security_updates_exceeded';
UPDATE alert_config SET check_interval_minutes = 30 WHERE alert_type = 'host_pending_updates_exceeded';
