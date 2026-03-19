ALTER TABLE host_packages
    DROP COLUMN IF EXISTS wua_guid,
    DROP COLUMN IF EXISTS wua_kb,
    DROP COLUMN IF EXISTS wua_severity,
    DROP COLUMN IF EXISTS wua_categories,
    DROP COLUMN IF EXISTS wua_description,
    DROP COLUMN IF EXISTS wua_support_url,
    DROP COLUMN IF EXISTS wua_revision_number,
    DROP COLUMN IF EXISTS wua_date_installed,
    DROP COLUMN IF EXISTS wua_install_result;

DROP INDEX IF EXISTS idx_host_packages_wua_guid;
