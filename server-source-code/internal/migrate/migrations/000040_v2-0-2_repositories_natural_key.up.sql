-- 000040: enforce natural key on repositories so SELECT-then-INSERT can
-- collapse to UPSERT. Removes a TOCTOU race where two concurrent host reports
-- could both see "no row" and both insert duplicates of
-- (url, distribution, components).

-- Step 1: dedupe existing rows. For each (url, distribution, components)
-- group keep the OLDEST row (smallest created_at, ties broken by id) and
-- repoint child FKs to it. host_packages.source_repository_id has
-- ON DELETE SET NULL — deleting duplicate parents would silently lose source
-- attribution on their child rows, so we repoint BEFORE deleting.

WITH ranked AS (
    SELECT
        id,
        url, distribution, components,
        FIRST_VALUE(id) OVER (
            PARTITION BY url, distribution, components
            ORDER BY created_at, id
        ) AS keep_id
    FROM repositories
)
UPDATE host_packages hp
SET source_repository_id = r.keep_id
FROM ranked r
WHERE hp.source_repository_id = r.id
  AND r.id <> r.keep_id;

-- Drop host_repositories rows that point at duplicate parents. We do not
-- repoint these because (host_id, repository_id) is UNIQUE; if the same
-- host has rows pointing at both a duplicate AND its surviving canonical
-- repository, repointing would 23505. The next agent report rebuilds
-- host_repositories from scratch (DeleteHostRepositoriesByHostID +
-- InsertHostRepository), so dropping them now is safe.
WITH ranked AS (
    SELECT
        id,
        url, distribution, components,
        FIRST_VALUE(id) OVER (
            PARTITION BY url, distribution, components
            ORDER BY created_at, id
        ) AS keep_id
    FROM repositories
)
DELETE FROM host_repositories hr
WHERE hr.repository_id IN (
    SELECT r.id FROM ranked r WHERE r.id <> r.keep_id
);

-- Now the duplicate parents have no remaining incoming FKs (host_packages
-- repointed, host_repositories deleted) and can be removed.
WITH ranked AS (
    SELECT
        id,
        url, distribution, components,
        FIRST_VALUE(id) OVER (
            PARTITION BY url, distribution, components
            ORDER BY created_at, id
        ) AS keep_id
    FROM repositories
)
DELETE FROM repositories r
USING ranked rk
WHERE r.id = rk.id AND rk.id <> rk.keep_id;

-- Step 2: enforce uniqueness going forward. Idempotent guard so reruns
-- don't fail if an earlier attempt added the constraint.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'repositories_url_distribution_components_key'
           OR conname = 'repositories_url_dist_comp_unique'
    ) THEN
        ALTER TABLE repositories
            ADD CONSTRAINT repositories_url_dist_comp_unique
            UNIQUE (url, distribution, components);
    END IF;
END $$;
