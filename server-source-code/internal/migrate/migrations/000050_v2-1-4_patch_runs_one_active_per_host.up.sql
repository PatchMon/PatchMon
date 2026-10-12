LOCK TABLE patch_runs IN SHARE ROW EXCLUSIVE MODE;

UPDATE patch_runs
SET status = 'scheduled', updated_at = (NOW() AT TIME ZONE 'UTC')
WHERE status = 'queued'
  AND scheduled_at > (NOW() AT TIME ZONE 'UTC');

UPDATE patch_runs
SET status = 'timed_out',
    error_message = 'Marked as timed_out: the run was not dispatched to the agent within 24 hours of becoming due',
    completed_at = (NOW() AT TIME ZONE 'UTC'),
    updated_at = (NOW() AT TIME ZONE 'UTC')
WHERE status IN ('scheduled', 'queued', 'pending_validation')
  AND COALESCE(scheduled_at, created_at) < (NOW() AT TIME ZONE 'UTC') - INTERVAL '24 hours';

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY host_id
               ORDER BY (status = 'running') DESC, (status = 'queued') DESC, updated_at DESC, created_at DESC, id
           ) AS rn
    FROM patch_runs
    WHERE status IN ('queued', 'pending_validation', 'running')
)
UPDATE patch_runs pr
SET status = 'timed_out',
    error_message = 'Stopped tracking on upgrade: another patch run was already active on this host',
    completed_at = (NOW() AT TIME ZONE 'UTC'),
    updated_at = (NOW() AT TIME ZONE 'UTC')
FROM ranked
WHERE pr.id = ranked.id
  AND ranked.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS patch_runs_one_active_per_host
    ON patch_runs (host_id)
    WHERE status IN ('queued', 'pending_validation', 'running');
