-- Adds boot_time to hosts so the frontend can compute live uptime as
-- (now() - boot_time), replacing the stale pre-formatted system_uptime
-- string. Existing rows stay NULL until each agent next reports; old
-- agents (pre-this-change) leave the column NULL indefinitely and the
-- UI falls back to system_uptime.
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS boot_time TIMESTAMPTZ;
