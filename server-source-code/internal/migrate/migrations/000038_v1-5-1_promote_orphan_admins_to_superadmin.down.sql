-- Intentionally a no-op.
--
-- We cannot safely reverse this migration: once promoted, the `superadmin`
-- users are indistinguishable from superadmins that were created manually
-- afterwards. Blindly demoting all superadmins back to admin would destroy
-- legitimate role assignments made after the upgrade.
--
-- Operators who need to roll back must reassign roles manually via the UI
-- or SQL after running `migrate down`.
SELECT 1;
