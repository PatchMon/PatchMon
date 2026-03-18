-- Extend host_packages with optional WUA (Windows Update Agent) metadata.
-- These columns are NULL for Linux/BSD packages and populated for Windows Update entries
-- (Category = 'Windows Update') reported by the Windows agent.
ALTER TABLE host_packages
    ADD COLUMN IF NOT EXISTS wua_guid            TEXT,
    ADD COLUMN IF NOT EXISTS wua_kb              TEXT,
    ADD COLUMN IF NOT EXISTS wua_severity        TEXT,
    ADD COLUMN IF NOT EXISTS wua_categories      JSONB,
    ADD COLUMN IF NOT EXISTS wua_description     TEXT,
    ADD COLUMN IF NOT EXISTS wua_support_url     TEXT,
    ADD COLUMN IF NOT EXISTS wua_revision_number INTEGER,
    ADD COLUMN IF NOT EXISTS wua_date_installed  TIMESTAMP(3),
    ADD COLUMN IF NOT EXISTS wua_install_result  TEXT;

-- Index for quick lookups by GUID (used for per-update install results and superseded cleanup)
CREATE INDEX IF NOT EXISTS idx_host_packages_wua_guid ON host_packages (host_id, wua_guid) WHERE wua_guid IS NOT NULL;
