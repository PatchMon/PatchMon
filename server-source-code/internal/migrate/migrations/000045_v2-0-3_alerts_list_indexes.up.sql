-- Indexes for the paginated Alerts list (Reporting > Alerts).
--
-- Before this migration the `alerts` and `alert_history` tables carried no
-- indexes at all beyond their primary keys, and the list endpoint returned
-- every row. Both are now addressed: the endpoint paginates and filters in
-- SQL, and these indexes let the planner serve it without a seq scan.
--
-- idx_alert_history_alert_id_created_at is the important one: resolving a
-- current_state for each alert is a LATERAL "latest history row per alert"
-- lookup, which without it degrades to a full scan of alert_history per
-- request.
--
-- Trigram indexes cover the search box, which is `title ILIKE '%x%' OR
-- message ILIKE '%x%'`. Same reasoning as idx_packages_description_trgm
-- (migration 000043): a leading-wildcard ILIKE can only be index-served by
-- gin_trgm_ops. pg_trgm is created by the v1.5.0 init.
--
-- No btree on alerts(severity) or alerts(type): both are low-cardinality
-- buckets (four severities, a handful of types) where the planner correctly
-- prefers a scan, so an index would be write overhead for nothing.
-- Idempotent: safe to rerun.

CREATE INDEX IF NOT EXISTS idx_alerts_created_at
    ON alerts (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_assigned_to_user_id
    ON alerts (assigned_to_user_id)
    WHERE assigned_to_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_alert_history_alert_id_created_at
    ON alert_history (alert_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_alerts_title_trgm
    ON alerts USING gin (title gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_alerts_message_trgm
    ON alerts USING gin (message gin_trgm_ops);
