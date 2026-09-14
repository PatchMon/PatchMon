-- Index for the stalled compliance scan sweep (compliance-scan-cleanup task)
-- and the two read paths that share its predicate.
--
-- compliance_scans carries only one non-primary-key index:
-- idx_compliance_scans_host_profile_completed, a partial unique index
-- WHERE status = 'completed'. The sweep's predicate is the opposite case
-- (non-terminal scans), so that index is never a candidate for it and every
-- sweep tick is a sequential scan. This was cheap while the sweep ran daily;
-- migration cf895d4c (fixing #996) raised it to hourly, a 24x increase in
-- scan frequency against a table that also never prunes 'failed' rows.
--
-- The partial predicate below is written to match the sweep's WHERE clause
-- exactly (same OR expression), not simplified to `status = 'running'`, so
-- Postgres's predicate implication check has a literal match to reason from
-- and the index is guaranteed usable rather than merely likely usable. It
-- also covers ListActiveComplianceScans and ListStalledComplianceScansWithDetails,
-- which share the same non-terminal predicate. 'failed' rows are deliberately
-- excluded from the index (not just filtered by the query) so index growth
-- stays bounded to the small number of scans genuinely in flight, even as
-- unpruned failed rows accumulate in the base table.
--
-- Plain CREATE INDEX, not CONCURRENTLY: golang-migrate's postgres driver
-- (v4.19.1, internal/migrate/migrate.go) executes each migration's SQL body
-- as a single statement with no explicit BEGIN/COMMIT wrapping in Go, but
-- confirming from first principles whether that guarantees CONCURRENTLY is
-- safe here depends on connection/protocol details this repo doesn't pin
-- down anywhere, and getting it wrong leaves a dirty schema_migrations row
-- needing manual recovery. compliance_scans is low-volume (per-host,
-- per-profile scan records, not a hot ingest path), matching the same
-- lock-vs-risk call already made for a larger table in migration 000041.
-- Idempotent: safe to rerun.

CREATE INDEX IF NOT EXISTS idx_compliance_scans_stalled
    ON compliance_scans (started_at)
    WHERE status = 'running' OR (completed_at IS NULL AND status != 'failed');
