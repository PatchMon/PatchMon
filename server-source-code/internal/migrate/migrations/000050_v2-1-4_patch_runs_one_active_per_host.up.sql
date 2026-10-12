WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY host_id
               ORDER BY (status = 'running') DESC, updated_at DESC, created_at DESC, id
           ) AS rn
    FROM patch_runs
    WHERE status IN ('queued', 'pending_validation', 'running')
)
UPDATE patch_runs pr
SET status = 'timed_out',
    error_message = 'Stopped tracking on upgrade: another patch run was already active on this host',
    completed_at = NOW(),
    updated_at = NOW()
FROM ranked
WHERE pr.id = ranked.id
  AND ranked.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS patch_runs_one_active_per_host
    ON patch_runs (host_id)
    WHERE status IN ('queued', 'pending_validation', 'running');
