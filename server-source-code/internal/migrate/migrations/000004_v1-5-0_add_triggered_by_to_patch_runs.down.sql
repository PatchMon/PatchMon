-- Remove triggered_by_user_id from patch_runs
ALTER TABLE patch_runs DROP COLUMN IF EXISTS triggered_by_user_id;
