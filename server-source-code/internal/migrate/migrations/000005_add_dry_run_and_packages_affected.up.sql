-- Add dry_run and packages_affected to patch_runs for dry-run validation feature
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS dry_run BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS packages_affected JSONB;
