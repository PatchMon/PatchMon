-- Seed default host_down threshold metadata (30 seconds) on the alert_config row
-- so the alerts/host_down logic can read a configurable threshold instead of
-- hardcoding update_interval x 3 minutes. Idempotent: only fills in the
-- threshold/threshold_unit keys when they are missing, so an operator's
-- previously-saved value is preserved on re-runs.
--
-- metadata may hold the JSON scalar null rather than SQL NULL on older installs.
-- COALESCE does not catch that, `? 'threshold'` returns false for it so the row
-- still matches, and jsonb_set then fails with 22023 "cannot set path in scalar",
-- aborting the migration. Normalise anything that is not a JSON object to '{}'
-- and match on jsonb_typeof rather than IS NULL.

UPDATE alert_config
SET metadata = jsonb_set(
        jsonb_set(
            CASE
                WHEN jsonb_typeof(metadata) = 'object' THEN metadata
                ELSE '{}'::jsonb
            END,
            '{threshold}',
            '30'::jsonb,
            true
        ),
        '{threshold_unit}',
        '"seconds"'::jsonb,
        true
    ),
    updated_at = NOW()
WHERE alert_type = 'host_down'
  AND (jsonb_typeof(metadata) IS DISTINCT FROM 'object' OR NOT (metadata ? 'threshold'));
