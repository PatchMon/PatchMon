-- Drop the UNIQUE constraint added in 000040.up.sql.
--
-- Note: the deduplication step in the UP migration is destructive — duplicate
-- repositories rows were merged into the oldest survivor and their child
-- host_repositories rows deleted. This DOWN migration drops the constraint
-- only; it cannot reconstruct the discarded rows. That is acceptable because
-- the next agent report rebuilds host_repositories anyway, and duplicate
-- repository rows were a bug, not a feature.
ALTER TABLE repositories
    DROP CONSTRAINT IF EXISTS repositories_url_dist_comp_unique;
