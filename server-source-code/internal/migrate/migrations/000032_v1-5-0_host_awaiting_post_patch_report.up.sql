ALTER TABLE hosts ADD COLUMN IF NOT EXISTS awaiting_post_patch_report_run_id TEXT REFERENCES patch_runs(id) ON DELETE SET NULL;
