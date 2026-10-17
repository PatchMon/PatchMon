CREATE INDEX IF NOT EXISTS idx_job_history_host_id_created_at
    ON job_history (host_id, created_at DESC);

-- Not unique: deterministic asynq task IDs (run-scan, ssg-upgrade) recur across runs.
CREATE INDEX IF NOT EXISTS idx_job_history_job_id
    ON job_history (job_id);
