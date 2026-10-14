CREATE TABLE IF NOT EXISTS patch_run_items (
    id TEXT PRIMARY KEY,
    patch_run_id TEXT NOT NULL REFERENCES patch_runs(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('wua', 'winget')),
    identifier TEXT NOT NULL,
    revision_number INTEGER,
    title TEXT NOT NULL,
    kb TEXT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'would_install', 'not_applicable', 'installed',
        'installed_with_errors', 'installed_pending_reboot', 'already_installed', 'failed', 'aborted', 'skipped')),
    result_code INTEGER,
    hresult TEXT,
    exit_code BIGINT,
    reboot_required BOOLEAN NOT NULL DEFAULT false,
    download_bytes BIGINT,
    message TEXT,
    started_at TIMESTAMP(3),
    completed_at TIMESTAMP(3),
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (patch_run_id, kind, identifier)
);
CREATE INDEX IF NOT EXISTS idx_patch_run_items_host_identifier ON patch_run_items (host_id, identifier);

ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS reboot_required BOOLEAN;
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS windows_include_updates BOOLEAN;
ALTER TABLE patch_runs ADD COLUMN IF NOT EXISTS windows_include_apps BOOLEAN;
ALTER TABLE host_packages ADD COLUMN IF NOT EXISTS winget_id TEXT;
ALTER TABLE host_packages ADD COLUMN IF NOT EXISTS winget_pinned BOOLEAN;
ALTER TABLE host_packages ADD COLUMN IF NOT EXISTS winget_store_app BOOLEAN;
ALTER TABLE host_packages ADD COLUMN IF NOT EXISTS wua_browse_only BOOLEAN;
ALTER TABLE host_packages ADD COLUMN IF NOT EXISTS wua_update_type TEXT;
ALTER TABLE host_packages DROP COLUMN IF EXISTS wua_install_result;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS patch_reboot_pending_boot_time TIMESTAMPTZ;
