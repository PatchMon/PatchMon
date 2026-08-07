-- Multi-channel notifications, delivery log, scheduled reports, permission flags

ALTER TABLE role_permissions
    ADD COLUMN IF NOT EXISTS can_manage_notifications BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS can_view_notification_logs BOOLEAN NOT NULL DEFAULT false;

UPDATE role_permissions SET can_manage_notifications = true, can_view_notification_logs = true
WHERE role IN ('superadmin', 'admin');

UPDATE role_permissions SET can_manage_notifications = true, can_view_notification_logs = false
WHERE role = 'host_manager';

CREATE TABLE IF NOT EXISTS notification_destinations (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    channel_type TEXT NOT NULL,
    display_name TEXT NOT NULL,
    config_encrypted TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS notification_routes (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    destination_id TEXT NOT NULL REFERENCES notification_destinations(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    min_severity TEXT NOT NULL DEFAULT 'informational',
    host_group_id TEXT REFERENCES host_groups(id) ON DELETE CASCADE,
    match_rules JSONB,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notification_routes_destination ON notification_routes(destination_id);

-- Conditional index: event_type exists when running fresh, but may have been
-- renamed to event_types by migration 020 if the DB was previously migrated.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'notification_routes' AND column_name = 'event_type'
    ) THEN
        CREATE INDEX IF NOT EXISTS idx_notification_routes_event ON notification_routes(event_type) WHERE enabled = true;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS notification_delivery_log (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    event_fingerprint TEXT NOT NULL,
    reference_type TEXT NOT NULL DEFAULT '',
    reference_id TEXT NOT NULL DEFAULT '',
    destination_id TEXT NOT NULL REFERENCES notification_destinations(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 1,
    provider_message_id TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notification_delivery_log_created ON notification_delivery_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_delivery_log_dest ON notification_delivery_log(destination_id);

CREATE TABLE IF NOT EXISTS scheduled_reports (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name TEXT NOT NULL,
    cron_expr TEXT NOT NULL DEFAULT '0 8 * * *',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    enabled BOOLEAN NOT NULL DEFAULT true,
    definition JSONB NOT NULL DEFAULT '{}'::jsonb,
    destination_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    next_run_at TIMESTAMP(3),
    last_run_at TIMESTAMP(3),
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scheduled_reports_due ON scheduled_reports(next_run_at) WHERE enabled = true;

CREATE TABLE IF NOT EXISTS scheduled_report_runs (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    scheduled_report_id TEXT NOT NULL REFERENCES scheduled_reports(id) ON DELETE CASCADE,
    run_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL,
    error_message TEXT,
    summary_hash TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_scheduled_report_runs_report ON scheduled_report_runs(scheduled_report_id);
