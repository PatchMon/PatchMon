DROP INDEX IF EXISTS idx_host_packages_source_repo;

ALTER TABLE host_packages DROP COLUMN IF EXISTS source_repository_id;
