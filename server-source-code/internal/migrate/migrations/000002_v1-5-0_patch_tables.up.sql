-- Patch tables for patching feature
-- Uses CREATE TABLE IF NOT EXISTS for idempotency: works for fresh installs and existing DBs (Prisma-migrated)

CREATE TABLE IF NOT EXISTS patch_policies (
    id TEXT NOT NULL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    patch_delay_type TEXT NOT NULL,
    delay_minutes INTEGER,
    fixed_time_utc TEXT,
    timezone TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS patch_policy_assignments (
    id TEXT NOT NULL PRIMARY KEY,
    patch_policy_id TEXT NOT NULL REFERENCES patch_policies(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS patch_policy_exclusions (
    id TEXT NOT NULL PRIMARY KEY,
    patch_policy_id TEXT NOT NULL REFERENCES patch_policies(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS patch_runs (
    id TEXT NOT NULL PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL,
    patch_type TEXT NOT NULL,
    package_name TEXT,
    status TEXT NOT NULL,
    shell_output TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    started_at TIMESTAMP(3),
    completed_at TIMESTAMP(3),
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Indexes (IF NOT EXISTS for idempotency)
CREATE INDEX IF NOT EXISTS patch_policies_patch_delay_type_idx ON patch_policies(patch_delay_type);

CREATE UNIQUE INDEX IF NOT EXISTS patch_policy_assignments_target_type_target_id_key ON patch_policy_assignments(target_type, target_id);
CREATE INDEX IF NOT EXISTS patch_policy_assignments_patch_policy_id_idx ON patch_policy_assignments(patch_policy_id);
CREATE INDEX IF NOT EXISTS patch_policy_assignments_target_type_idx ON patch_policy_assignments(target_type);
CREATE INDEX IF NOT EXISTS patch_policy_assignments_target_id_idx ON patch_policy_assignments(target_id);

CREATE UNIQUE INDEX IF NOT EXISTS patch_policy_exclusions_patch_policy_id_host_id_key ON patch_policy_exclusions(patch_policy_id, host_id);
CREATE INDEX IF NOT EXISTS patch_policy_exclusions_patch_policy_id_idx ON patch_policy_exclusions(patch_policy_id);
CREATE INDEX IF NOT EXISTS patch_policy_exclusions_host_id_idx ON patch_policy_exclusions(host_id);

CREATE INDEX IF NOT EXISTS patch_runs_host_id_idx ON patch_runs(host_id);
CREATE INDEX IF NOT EXISTS patch_runs_job_id_idx ON patch_runs(job_id);
CREATE INDEX IF NOT EXISTS patch_runs_status_idx ON patch_runs(status);
CREATE INDEX IF NOT EXISTS patch_runs_created_at_idx ON patch_runs(created_at);
CREATE INDEX IF NOT EXISTS patch_runs_host_id_created_at_idx ON patch_runs(host_id, created_at);

-- package_names column (for grouped installs)
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS package_names JSONB;

-- triggered_by_user_id: user who initiated the patch run
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS triggered_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL;
