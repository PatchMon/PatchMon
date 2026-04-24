ALTER TABLE host_packages ADD COLUMN IF NOT EXISTS source_repository_id TEXT REFERENCES repositories(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_host_packages_source_repo ON host_packages(source_repository_id) WHERE source_repository_id IS NOT NULL;
