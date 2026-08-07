-- Add triggered_by_user_id to patch_runs (user who initiated the patch run)
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS triggered_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL;
