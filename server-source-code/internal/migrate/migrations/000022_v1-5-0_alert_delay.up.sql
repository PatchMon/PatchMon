-- Add configurable alert delay (seconds) to suppress transient notifications.
ALTER TABLE alert_config ADD COLUMN IF NOT EXISTS alert_delay_seconds INTEGER;
