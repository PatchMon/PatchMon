CREATE INDEX IF NOT EXISTS idx_patch_runs_packages_affected_gin ON patch_runs USING GIN (packages_affected);
CREATE INDEX IF NOT EXISTS idx_patch_runs_package_names_gin ON patch_runs USING GIN (package_names);
