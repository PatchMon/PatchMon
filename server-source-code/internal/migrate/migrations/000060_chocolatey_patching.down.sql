-- Preserve history: downgrade requires removal of Chocolatey run items first.
ALTER TABLE patch_run_items DROP CONSTRAINT patch_run_items_kind_check;
ALTER TABLE patch_run_items ADD CONSTRAINT patch_run_items_kind_check CHECK (kind IN ('wua', 'winget'));
ALTER TABLE host_packages DROP COLUMN chocolatey_pinned;
ALTER TABLE host_packages DROP COLUMN chocolatey_id;
