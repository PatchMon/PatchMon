-- Add validation_run_id: links a real patch run back to its validation (dry-run) run.
-- When a user approves a validated run, a NEW patch run is created with this field
-- pointing at the original validation run, preserving both runs as separate records.
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS validation_run_id TEXT REFERENCES patch_runs(id) ON DELETE SET NULL;
