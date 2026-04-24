-- Re-add per-report timezone column.
ALTER TABLE scheduled_reports ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC';
