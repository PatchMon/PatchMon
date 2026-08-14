-- Remove the seeded threshold metadata from the host_down alert_config row.
-- We can't safely tell whether the operator changed the value after the up
-- migration, so this strips both keys unconditionally; re-running the up
-- migration restores the 30-second default.
--
-- Guarded on jsonb_typeof rather than IS NOT NULL for the same reason as the up
-- migration: `?` is true for the string scalar '"threshold"' and for an array
-- containing it, and the `-` operator then fails on a non-object with 22023.

UPDATE alert_config
SET metadata = NULLIF(metadata - 'threshold' - 'threshold_unit', '{}'::jsonb),
    updated_at = NOW()
WHERE alert_type = 'host_down'
  AND jsonb_typeof(metadata) = 'object'
  AND (metadata ? 'threshold' OR metadata ? 'threshold_unit');
