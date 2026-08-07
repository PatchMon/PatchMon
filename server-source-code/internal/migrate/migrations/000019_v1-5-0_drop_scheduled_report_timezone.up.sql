-- Remove per-report timezone; scheduled reports now use the app-level TZ setting.
ALTER TABLE scheduled_reports DROP COLUMN IF EXISTS timezone;
