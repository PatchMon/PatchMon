-- Reverse of 000041_v2-0-3_release. Drops in the opposite order to up
-- (matview before its dependent indexes; columns last) so a partial
-- rollback never trips on a still-extant dependency.

-- 5. mv_package_stats (drops the unique index too).
DROP MATERIALIZED VIEW IF EXISTS mv_package_stats;

-- 4. host_packages perf indexes.
DROP INDEX IF EXISTS idx_host_packages_needs_update_security_package;
DROP INDEX IF EXISTS idx_host_packages_needs_update_package;
DROP INDEX IF EXISTS idx_host_packages_needs_update_host_cover;
DROP INDEX IF EXISTS idx_host_packages_needs_update_security;

-- 3. Agent Activity feed.
DROP INDEX IF EXISTS idx_update_history_timestamp_for_retention;
DROP INDEX IF EXISTS idx_update_history_host_id_timestamp;
ALTER TABLE update_history
    DROP COLUMN IF EXISTS agent_execution_ms,
    DROP COLUMN IF EXISTS sections_unchanged,
    DROP COLUMN IF EXISTS sections_sent,
    DROP COLUMN IF EXISTS report_type;

-- 2. Settings tunables.
ALTER TABLE settings
    DROP COLUMN IF EXISTS agent_reports_retention_days,
    DROP COLUMN IF EXISTS patch_run_stall_timeout_minutes;

-- 1. Hash-gated agent check-in: drop constraints first, then columns.
ALTER TABLE hosts
    DROP CONSTRAINT IF EXISTS hosts_compliance_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_docker_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_hostname_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_interfaces_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_repos_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_packages_hash_len;

ALTER TABLE hosts
    DROP COLUMN IF EXISTS last_full_report_at,
    DROP COLUMN IF EXISTS compliance_hash,
    DROP COLUMN IF EXISTS docker_hash,
    DROP COLUMN IF EXISTS hostname_hash,
    DROP COLUMN IF EXISTS interfaces_hash,
    DROP COLUMN IF EXISTS repos_hash,
    DROP COLUMN IF EXISTS packages_hash;
