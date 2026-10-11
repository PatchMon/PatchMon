-- 000043: automated patching schedule on patch_policies.
--
-- A policy with auto_patch_enabled=true fires patch_all runs for every host
-- it resolves to (direct assignment + group membership minus exclusions, with
-- the usual direct-assignment precedence) on the configured days at the
-- configured time of day, with no operator interaction.
--
--   auto_patch_days   csv of weekday numbers, 0=Sunday .. 6=Saturday ("0,3")
--   auto_patch_time   HH:MM, interpreted in the org-resolved timezone
--   auto_patch_last_run_at  UTC instant of the last automatic fire; used by
--                           the dispatcher to fire at most once per scheduled
--                           slot even though it polls every minute.
--
-- auto_reboot (added in 000042) is consumed by automatic runs only: after a
-- successful scheduled patch the agent reboots iff the host reports a pending
-- reboot. Manual wizard runs are unaffected.

ALTER TABLE patch_policies ADD COLUMN IF NOT EXISTS auto_patch_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE patch_policies ADD COLUMN IF NOT EXISTS auto_patch_days TEXT;
ALTER TABLE patch_policies ADD COLUMN IF NOT EXISTS auto_patch_time TEXT;
ALTER TABLE patch_policies ADD COLUMN IF NOT EXISTS auto_patch_last_run_at TIMESTAMP(3);
