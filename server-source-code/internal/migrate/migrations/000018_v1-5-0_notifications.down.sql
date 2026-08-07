DROP TABLE IF EXISTS scheduled_report_runs;
DROP TABLE IF EXISTS scheduled_reports;
DROP TABLE IF EXISTS notification_delivery_log;
DROP TABLE IF EXISTS notification_routes;
DROP TABLE IF EXISTS notification_destinations;

ALTER TABLE role_permissions DROP COLUMN IF EXISTS can_view_notification_logs;
ALTER TABLE role_permissions DROP COLUMN IF EXISTS can_manage_notifications;
