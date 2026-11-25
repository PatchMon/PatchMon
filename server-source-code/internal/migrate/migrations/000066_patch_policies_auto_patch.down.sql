ALTER TABLE patch_policies DROP COLUMN IF EXISTS auto_patch_enabled;
ALTER TABLE patch_policies DROP COLUMN IF EXISTS auto_patch_days;
ALTER TABLE patch_policies DROP COLUMN IF EXISTS auto_patch_time;
ALTER TABLE patch_policies DROP COLUMN IF EXISTS auto_patch_last_run_at;
