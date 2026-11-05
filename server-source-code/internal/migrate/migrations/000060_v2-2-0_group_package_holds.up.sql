CREATE TABLE IF NOT EXISTS host_group_package_holds (
    id TEXT PRIMARY KEY,
    host_group_id TEXT NOT NULL REFERENCES host_groups(id) ON DELETE CASCADE,
    package_name TEXT NOT NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (host_group_id, package_name)
);

CREATE TABLE IF NOT EXISTS patch_run_excludes (
    patch_run_id TEXT NOT NULL REFERENCES patch_runs(id) ON DELETE CASCADE,
    package_name TEXT NOT NULL,
    PRIMARY KEY (patch_run_id, package_name)
);
