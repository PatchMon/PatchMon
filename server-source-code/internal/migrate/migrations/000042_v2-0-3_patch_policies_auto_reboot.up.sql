-- 000042: reserved column for future policy-level auto_reboot toggle.
--
-- The column is added now so the schema is forward-compatible with the
-- planned policy-driven reboot flow, even though this release ships the
-- simpler on-demand reboot path (POST /hosts/:id/reboot,
-- POST /hosts/bulk/reboot) that doesn't read this flag yet.
--
-- Keeping the migration in tree means a deploy that already applied this
-- column (during the on-demand reboot feature development) doesn't end up
-- with a schema version ahead of the migration set on subsequent rebuilds.

ALTER TABLE patch_policies ADD COLUMN IF NOT EXISTS auto_reboot BOOLEAN NOT NULL DEFAULT false;
