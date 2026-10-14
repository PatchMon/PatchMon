DROP INDEX IF EXISTS patch_runs_one_active_per_host;

UPDATE patch_runs SET status = 'queued', updated_at = (NOW() AT TIME ZONE 'UTC') WHERE status = 'scheduled';
