-- Seed default host_down threshold metadata (30 seconds) on the alert_config row
-- so the alerts/host_down logic can read a configurable threshold instead of
-- hardcoding update_interval x 3 minutes. Idempotent: only fills in the
-- threshold/threshold_unit keys when they are missing, so an operator's
-- previously-saved value is preserved on re-runs.

UPDATE alert_config
SET metadata = jsonb_set(
        jsonb_set(
            COALESCE(metadata, '{}'::jsonb),
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
  AND (metadata IS NULL OR NOT (metadata ? 'threshold'));
