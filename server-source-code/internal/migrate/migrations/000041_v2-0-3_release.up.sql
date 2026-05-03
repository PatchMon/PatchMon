-- v2.0.3 release migration (consolidated).
--
-- Combines what was previously a chain of 7 separate migrations
-- (host_section_hashes, patch_run_stall_timeout_minutes, agent_reports,
-- agent_reports_retention_days, host_packages_security_partial_index,
-- host_package_count_indexes, package_stats_mat_view) into a single
-- atomic migration. v2.0.3 has not been published; no production
-- database is on any intermediate state, so squashing the in-development
-- migrations into one is safe and keeps the upgrade path 2.0.2 → 2.0.3
-- a single transaction for operators.
--
-- Sections, in order:
--   1. Hash-gated agent check-in: per-section content hashes on `hosts`.
--   2. New tunables on `settings`: patch run stall timeout + agent
--      reports retention.
--   3. Agent Activity feed: extend `update_history` with report-type
--      discriminator + per-section sent/skipped lists + agent execution
--      timing, plus the indexes the feed UI and retention sweep need.
--   4. host_packages perf indexes for the dashboard / hosts list /
--      packages list queries.
--   5. mv_package_stats materialised view that powers the Packages
--      list page's per-package counters in <5 ms instead of ~10 s.

-- ---------------------------------------------------------------------------
-- 1. Hash-gated agent check-in (was: 000041_host_section_hashes)
-- ---------------------------------------------------------------------------
-- The agent ships SHA-256 hashes of canonical packages/repos/interfaces/
-- hostname/docker/compliance content on every ping. Server compares each
-- against the stored value and asks the agent for full content only when
-- a hash differs (or is NULL). This collapses the steady-state hourly
-- check-in from a ~2 MB POST to a ~1 KB ping + ~200 B response.
--
-- Hash columns are 64-char lowercase hex (sha256). NULL means "no stored
-- hash, force the agent to send a full report so we can populate it" —
-- this is also the natural state for hosts that existed before this
-- migration ran. last_full_report_at is informational, used by the
-- operator UI / staleness alerts.
ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS packages_hash       TEXT,
    ADD COLUMN IF NOT EXISTS repos_hash          TEXT,
    ADD COLUMN IF NOT EXISTS interfaces_hash     TEXT,
    ADD COLUMN IF NOT EXISTS hostname_hash       TEXT,
    ADD COLUMN IF NOT EXISTS docker_hash         TEXT,
    ADD COLUMN IF NOT EXISTS compliance_hash     TEXT,
    ADD COLUMN IF NOT EXISTS last_full_report_at TIMESTAMP(3);

-- Length checks. Idempotent guard so the migration is safe to rerun on a
-- partially-applied database.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_packages_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_packages_hash_len
            CHECK (packages_hash IS NULL OR length(packages_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_repos_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_repos_hash_len
            CHECK (repos_hash IS NULL OR length(repos_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_interfaces_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_interfaces_hash_len
            CHECK (interfaces_hash IS NULL OR length(interfaces_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_hostname_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_hostname_hash_len
            CHECK (hostname_hash IS NULL OR length(hostname_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_docker_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_docker_hash_len
            CHECK (docker_hash IS NULL OR length(docker_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_compliance_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_compliance_hash_len
            CHECK (compliance_hash IS NULL OR length(compliance_hash) = 64);
    END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 2. New settings tunables (was: 000042 + 000044)
-- ---------------------------------------------------------------------------
-- Both NULL means "use env or built-in default" (env -> DB -> default).
ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS patch_run_stall_timeout_minutes INTEGER,
    ADD COLUMN IF NOT EXISTS agent_reports_retention_days    INTEGER;

-- ---------------------------------------------------------------------------
-- 3. Agent Activity feed (was: 000043_agent_reports)
-- ---------------------------------------------------------------------------
-- update_history extended with report-type discriminator, per-section
-- "updated/skipped" lists, and agent-side execution timing. Used by the
-- Agent Activity tab to render every agent comm cycle (ping, full,
-- partial, docker, compliance) on a single timeline.
ALTER TABLE update_history
    ADD COLUMN IF NOT EXISTS report_type        TEXT      NOT NULL DEFAULT 'full',
    ADD COLUMN IF NOT EXISTS sections_sent      TEXT[]    NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS sections_unchanged TEXT[]    NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS agent_execution_ms INTEGER;

-- Index for the per-host activity feed (reverse-chronological pagination).
CREATE INDEX IF NOT EXISTS idx_update_history_host_id_timestamp
    ON update_history (host_id, timestamp DESC);

-- Index for the retention sweep (DELETE WHERE timestamp < threshold).
CREATE INDEX IF NOT EXISTS idx_update_history_timestamp_for_retention
    ON update_history (timestamp);

-- ---------------------------------------------------------------------------
-- 4. host_packages perf indexes (was: 000045 + 000046)
-- ---------------------------------------------------------------------------
-- These four partial / covering indexes target distinct query shapes:
--   * security count per host: idx_host_packages_needs_update_security
--     — narrowest predicate, smallest index, ideal for the security-only
--     count subquery in GetHostsWithCounts.
--   * needs-update count per host with is_security_update on hand:
--     idx_host_packages_needs_update_host_cover — covering index for
--     the broader needs-update + per-row security check pattern.
--   * outdated package counts globally:
--     idx_host_packages_needs_update_package — the package_id-keyed
--     companion to the host_id partial index from v1.5.0 init.
--   * security-only outdated package counts globally:
--     idx_host_packages_needs_update_security_package — the
--     package_id-keyed companion to the security partial index.
--
-- CREATE INDEX (without CONCURRENTLY) holds an ACCESS EXCLUSIVE lock for
-- the duration of the build. At ~40k matching rows on a 1.3M-row table
-- this completes in a few hundred ms — acceptable since the migration
-- runs once at startup. If a future deployment has tens of millions of
-- security-flagged rows, a separate ops procedure can drop and recreate
-- with CONCURRENTLY out-of-band.

CREATE INDEX IF NOT EXISTS idx_host_packages_needs_update_security
    ON host_packages (host_id)
    WHERE needs_update = true AND is_security_update = true;

CREATE INDEX IF NOT EXISTS idx_host_packages_needs_update_host_cover
    ON host_packages (host_id)
    INCLUDE (is_security_update)
    WHERE needs_update = true;

CREATE INDEX IF NOT EXISTS idx_host_packages_needs_update_package
    ON host_packages (package_id)
    WHERE needs_update = true;

CREATE INDEX IF NOT EXISTS idx_host_packages_needs_update_security_package
    ON host_packages (package_id)
    WHERE needs_update = true AND is_security_update = true;

-- ---------------------------------------------------------------------------
-- 5. mv_package_stats materialised view (was: 000047_package_stats_mat_view)
-- ---------------------------------------------------------------------------
-- Per-package install / update / security counters used by the Packages
-- list page. Without this, ListPackages has to compute COUNT(*) +
-- COUNT(*) FILTER (...) across host_packages (~1.3 M rows at 1k-host
-- scale) for every request just to render 25 rows — which costs ~10 s
-- (HashAggregate, work_mem-spilled) or ~30 s (LATERAL fan-out) before
-- LIMIT can fire. The matview is joined as a single indexed hash join,
-- sub-millisecond per request.
--
-- Refresh is driven by an asynq scheduler entry (TypePackageStatsRefresh,
-- every 2 minutes). REFRESH CONCURRENTLY needs the unique index below
-- and produces brief row-level locks rather than a global table lock.
-- Worst-case staleness on the Packages page = the refresh interval,
-- which is fine for an admin counters page.
--
-- The initial CREATE MATERIALIZED VIEW populates the view inline using
-- the same query the matview itself stores; subsequent refreshes use
-- REFRESH MATERIALIZED VIEW CONCURRENTLY mv_package_stats.
CREATE MATERIALIZED VIEW IF NOT EXISTS mv_package_stats AS
SELECT
    hp.package_id,
    COUNT(*)::int                                                                  AS total_installs,
    COUNT(*) FILTER (WHERE hp.needs_update)::int                                   AS updates_needed,
    COUNT(*) FILTER (WHERE hp.needs_update AND hp.is_security_update)::int         AS security_updates
FROM host_packages hp
GROUP BY hp.package_id;

-- Unique index is required for REFRESH ... CONCURRENTLY.
CREATE UNIQUE INDEX IF NOT EXISTS mv_package_stats_pkey
    ON mv_package_stats (package_id);
