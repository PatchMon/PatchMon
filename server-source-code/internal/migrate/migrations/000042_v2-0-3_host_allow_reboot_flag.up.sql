-- Adds the per-host allow_reboot flag (reboot allowlist). Remote reboot is
-- fail-closed: only hosts where an operator explicitly set allow_reboot = true
-- can be rebooted. Defaults to false so newly enrolled and existing hosts
-- (including the PatchMon server's own host) are protected until opted in.

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS allow_reboot BOOLEAN NOT NULL DEFAULT false;
