DROP INDEX IF EXISTS idx_update_history_timestamp_for_retention;
DROP INDEX IF EXISTS idx_update_history_host_id_timestamp;

ALTER TABLE update_history
    DROP COLUMN IF EXISTS agent_execution_ms,
    DROP COLUMN IF EXISTS sections_unchanged,
    DROP COLUMN IF EXISTS sections_sent,
    DROP COLUMN IF EXISTS report_type;
