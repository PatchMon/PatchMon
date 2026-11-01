ALTER TABLE host_packages ADD COLUMN chocolatey_id TEXT;
ALTER TABLE host_packages ADD COLUMN chocolatey_pinned BOOLEAN;
ALTER TABLE patch_run_items DROP CONSTRAINT patch_run_items_kind_check;
ALTER TABLE patch_run_items ADD CONSTRAINT patch_run_items_kind_check CHECK (kind IN ('wua', 'winget', 'chocolatey'));
