-- Remove the seeded threshold metadata from the host_down alert_config row.
-- We can't safely tell whether the operator changed the value after the up
-- migration, so this strips both keys unconditionally; re-running the up
-- migration restores the 30-second default.

UPDATE alert_config
SET metadata = NULLIF(metadata - 'threshold' - 'threshold_unit', '{}'::jsonb),
    updated_at = NOW()
WHERE alert_type = 'host_down'
  AND metadata IS NOT NULL
  AND (metadata ? 'threshold' OR metadata ? 'threshold_unit');
