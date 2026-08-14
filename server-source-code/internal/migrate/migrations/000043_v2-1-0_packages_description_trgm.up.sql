-- Trigram index on packages.description so the Packages list page's
-- search predicate (`p.name ILIKE '%foo%' OR p.description ILIKE '%foo%'`)
-- can be served by indexes for both columns. Without it, Postgres falls
-- back to a parallel sequential scan on `packages` whenever the search
-- box is used, which in Docker hits the default 64 MB /dev/shm shared
-- memory ceiling and 500s with `could not resize shared memory segment`.
--
-- A matching trigram index already exists on packages.name from the
-- v1.5.0 init (idx_packages_name_trgm). This migration completes the
-- coverage for the description leg of the same OR predicate.
--
-- A btree index on packages(category) was considered and skipped:
-- `category` is a low-cardinality bucket (typically <50 distinct values
-- like "main", "universe", "extras", ...). The planner picks an index
-- scan only when the predicate is selective; on a low-cardinality
-- column it correctly prefers a sequential scan over a btree index, so
-- the index would just be dead weight in storage and write overhead.
-- Idempotent: safe to rerun.
CREATE INDEX IF NOT EXISTS idx_packages_description_trgm
    ON packages USING gin (description gin_trgm_ops);
