-- Add scheduled_at to patch_runs for displaying when a queued patch will run
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMP(3);
