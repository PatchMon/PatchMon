-- Agent Activity feature: extend update_history with report-type discriminator,
-- per-section "updated/skipped" lists, and agent-side execution timing. Used by
-- the new Agent Activity tab to render every agent comm cycle (ping, full,
-- partial, docker, compliance) on a single timeline.
ALTER TABLE update_history
    ADD COLUMN IF NOT EXISTS report_type TEXT NOT NULL DEFAULT 'full',
    ADD COLUMN IF NOT EXISTS sections_sent TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS sections_unchanged TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS agent_execution_ms INTEGER;

-- Index for the per-host activity feed (reverse-chronological pagination).
CREATE INDEX IF NOT EXISTS idx_update_history_host_id_timestamp
    ON update_history (host_id, timestamp DESC);

-- Index for the retention sweep (DELETE WHERE timestamp < threshold).
CREATE INDEX IF NOT EXISTS idx_update_history_timestamp_for_retention
    ON update_history (timestamp);
